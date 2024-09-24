package multipass

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kev-cao/log-console/deploy-cli/dispatch"
	"github.com/kev-cao/log-console/utils/path"
	"github.com/kev-cao/log-console/utils/slice"
	"golang.org/x/sync/errgroup"
)

type MultipassDispatcher struct {
	NumNodes   int
	MasterName string
	// Workers will be named $WorkerName-1, $WorkerName-2, ...
	WorkerName string
}

var _ dispatch.ClusterDispatcher = &MultipassDispatcher{}

func (m MultipassDispatcher) Init() error {
	if err := m.launchNodes(); err != nil {
		return err
	}
	fmt.Println("Setting up K3S on nodes...")
	if err := m.setupK3S(); err != nil {
		return err
	}
	fmt.Println("Nodes are ready!")
	return nil
}

func (m MultipassDispatcher) launchNodes() error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	nodes := m.GetNodes()
	var wg errgroup.Group
	stdoutWriter := newLaunchWriter(os.Stdout)
	for _, node := range nodes {
		func(node dispatch.Node) {
			nodeWriter := stdoutWriter.newNodeWriter(&node)
			wg.Go(func() error {
				stdErr := bytes.NewBuffer([]byte{})
				cmd := exec.CommandContext(
					ctx,
					"multipass", "launch", "--name", node.Name, "--cpus", "2", "--memory", "2G", "--disk", "20G",
				)
				cmd.Stdout = nodeWriter
				cmd.Stderr = stdErr
				e := cmd.Run()
				if e != nil {
					stdoutWriter.setLine(
						&node,
						fmt.Sprintf("Error launching node: %s", strings.TrimSpace(stdErr.String())),
					)
					return e
				}
				stdoutWriter.setLine(&node, "Node launched successfully!")
				return nil
			})
		}(node)
	}
	if err := wg.Wait(); err != nil {
		return err
	}
	return nil
}

func (m MultipassDispatcher) setupK3S() error {
	// Start K3S daemon on master node
	masterNode := m.GetMasterNode()
	if err := m.SendCommand(
		masterNode,
		dispatch.NewCommand(
			masterNode,
			fmt.Sprintf(
				`curl -sfL https://get.k3s.io | K3S_NODE_NAME=%s K3S_KUBECONFIG_MODE=644 sh -`,
				masterNode.Name,
			),
			nil,
			2*time.Minute,
		),
	); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	// Get connection params for worker nodes
	url, token, err := m.getMasterNodeConnectionParams()
	if err != nil {
		return err
	}
	var wg errgroup.Group
	for _, node := range m.GetWorkerNodes() {
		func(node dispatch.Node) {
			wg.Go(func() error {
				if err := m.SendCommandContext(
					ctx,
					node,
					dispatch.NewCommand(
						node,
						fmt.Sprintf(
							`curl -sfL https://get.k3s.io | K3S_NODE_NAME="%s" K3S_URL="%s" K3S_TOKEN="%s" sh -`,
							node.Name, url, token,
						),
						nil,
						30*time.Second,
					),
				); err != nil {
					return err
				}
				return nil
			})
		}(node)
	}
	if err := wg.Wait(); err != nil {
		return err
	}
	return nil
}

func (m MultipassDispatcher) getMasterNodeConnectionParams() (
	url string,
	token string,
	err error,
) {
	master := m.GetMasterNode()
	cmd := exec.Command(
		"bash", "-c",
		fmt.Sprintf(
			`multipass info %s --format json | jq -r '.info.["%s"].ipv4[0]'`,
			master.Name, master.Name,
		),
	)
	cmd.Stderr = dispatch.NewPrefixWriter(master.Name, os.Stderr)
	cmd.WaitDelay = 5 * time.Second
	urlBytes, err := cmd.Output()
	if err != nil {
		return "", "", err
	}
	cmd = exec.Command(
		"bash", "-c",
		fmt.Sprintf(
			`multipass exec %s -- sudo cat /var/lib/rancher/k3s/server/node-token`,
			master.Name,
		),
	)
	cmd.Stderr = dispatch.NewPrefixWriter(master.Name, os.Stderr)
	cmd.WaitDelay = 5 * time.Second
	tokenBytes, err := cmd.Output()
	if err != nil {
		return "", "", err
	}
	url = fmt.Sprintf("https://%s:6443", strings.TrimSpace(string(urlBytes)))
	token = strings.TrimSpace(string(tokenBytes))
	return
}

func (m MultipassDispatcher) Ready() bool {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	ready := true
	var mu sync.Mutex
	for _, node := range m.GetNodes() {
		wg.Add(1)
		go func(node dispatch.Node) {
			defer wg.Done()
			cmd := exec.CommandContext(
				ctx,
				"bash", "-c",
				fmt.Sprintf(
					`multipass info %s --format json | jq -r '.info.["%s"].state'`,
					node.Name, node.Name,
				),
			)
			cmd.WaitDelay = 5 * time.Second
			state, err := cmd.Output()
			// If ready is already set false, we don't need to check again
			if !ready {
				return
			}
			// If there was an error or the node is not running, set ready to false
			if err != nil || !strings.Contains(string(state), "Running") {
				mu.Lock()
				defer mu.Unlock()
				ready = false
				cancel()
			}
		}(node)
	}
	wg.Wait()
	return ready
}

func (m MultipassDispatcher) GetNodes() []dispatch.Node {
	return append([]dispatch.Node{m.GetMasterNode()}, m.GetWorkerNodes()...)
}

func (m MultipassDispatcher) GetMasterNode() dispatch.Node {
	return dispatch.Node{
		Name: "master",
	}
}

func (m MultipassDispatcher) GetWorkerNodes() []dispatch.Node {
	var nodes []dispatch.Node
	for i := 1; i < m.NumNodes; i++ {
		nodes = append(nodes, dispatch.Node{
			Name: m.WorkerName + "-" + strconv.Itoa(i),
		})
	}
	return nodes
}

func (m MultipassDispatcher) SendCommand(node dispatch.Node, cmds ...dispatch.Command) error {
	for _, cmd := range cmds {
		command := exec.Command(
			"multipass", "exec", node.Name, "--", "/bin/bash", "-c",
			fmt.Sprintf("%s %s", buildEnvBindings(cmd.Env), cmd.Cmd),
		)
		command.WaitDelay = cmd.Timeout
		if cmd.Stdout != nil {
			command.Stdout = cmd.Stdout
		} else {
			command.Stdout = io.Discard
		}
		if cmd.Stderr != nil {
			command.Stderr = cmd.Stderr
		} else {
			command.Stderr = io.Discard
		}

		if err := command.Run(); err != nil {
			return err
		}
	}
	return nil
}

func (m MultipassDispatcher) SendCommandContext(
	ctx context.Context,
	node dispatch.Node,
	cmds ...dispatch.Command,
) error {
	for _, cmd := range cmds {
		command := exec.CommandContext(
			ctx, "multipass", "exec", node.Name, "--", "/bin/bash", "-c",
			fmt.Sprintf("%s %s", buildEnvBindings(cmd.Env), cmd.Cmd),
		)
		if cmd.Stdout != nil {
			command.Stdout = cmd.Stdout
		} else {
			command.Stdout = io.Discard
		}
		if cmd.Stderr != nil {
			command.Stderr = cmd.Stderr
		} else {
			command.Stderr = io.Discard
		}

		if err := command.Run(); err != nil {
			return err
		}
	}
	return nil
}

func (m MultipassDispatcher) SendCommandEnv(
	node dispatch.Node,
	env map[string]string,
	cmds ...dispatch.Command,
) error {
	cmdsWithEnvs := make([]dispatch.Command, len(cmds))
	for _, cmd := range cmds {
		if cmd.Env == nil {
			cmd.Env = env
		} else {
			for k, v := range env {
				cmd.Env[k] = v
			}
		}
		cmdsWithEnvs = append(cmdsWithEnvs, cmd)
	}
	return m.SendCommand(node, cmdsWithEnvs...)
}

func (m MultipassDispatcher) SendCommandEnvContext(
	ctx context.Context,
	node dispatch.Node,
	env map[string]string,
	cmds ...dispatch.Command,
) error {
	cmdsWithEnvs := make([]dispatch.Command, len(cmds))
	for _, cmd := range cmds {
		cmd.Env = env
		cmdsWithEnvs = append(cmdsWithEnvs, cmd)
	}
	return m.SendCommandContext(ctx, node, cmdsWithEnvs...)
}

func (m MultipassDispatcher) SendFile(node dispatch.Node, src, dst string) error {
	cmd := pipeOutputs(
		exec.Command("multipass", "transfer", "--parents", src, node.Name+":"+dst),
		node.Name,
	)
	return cmd.Run()
}

func (m MultipassDispatcher) DownloadProject(node dispatch.Node, source string) error {
	if strings.HasPrefix(source, "local://") {
		src := strings.TrimPrefix(source, "local://")
		path, err := path.AbsolutePath(src)
		if err != nil {
			return err
		}
		// unmount if it's already mounted
		exec.Command("multipass", "umount", node.Name+":/home/ubuntu/log-console").Run()
		if err := exec.Command(
			"multipass", "mount", "--type=classic", path, node.Name+":/home/ubuntu/log-console",
		).Run(); err != nil {
			return err
		}
	} else {
		if err := m.SendCommand(
			node,
			dispatch.NewCommands(
				node,
				0,
				"rm -rf /home/ubuntu/log-console",
				fmt.Sprintf("git clone %s /home/ubuntu/log-console", source),
			)...,
		); err != nil {
			return err
		}
	}
	return nil
}

func (m MultipassDispatcher) TearDown() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	nodes := slice.Map(m.GetNodes(), func(node dispatch.Node, _ int) string {
		return node.Name
	})
	cmd := exec.CommandContext(ctx, "multipass", append([]string{"stop", "--force"}, nodes...)...)
	if err := cmd.Run(); err != nil {
		return err
	}
	cmd = exec.CommandContext(ctx, "multipass", append([]string{"delete"}, nodes...)...)
	if err := cmd.Run(); err != nil {
		return err
	}
	cmd = exec.CommandContext(ctx, "multipass", "purge")
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

// buildEnvBindings converts a map of environment variables to a space-separate list of bindings
// in the form "key=value".
func buildEnvBindings(env map[string]string) string {
	var envBindings []string
	for k, v := range env {
		envBindings = append(envBindings, k+"="+v)
	}
	return strings.Join(envBindings, " ")
}

// pipeOutputs pipes the command's stdout and stderr to the current process's stdout and stderr.
// Mostly used for easily one-lining creating exec.Cmd and running.
func pipeOutputs(cmd *exec.Cmd, prefix string) *exec.Cmd {
	cmd.Stdout = dispatch.NewPrefixWriter(prefix, os.Stdout)
	cmd.Stderr = dispatch.NewPrefixWriter(prefix, os.Stderr)
	return cmd
}
