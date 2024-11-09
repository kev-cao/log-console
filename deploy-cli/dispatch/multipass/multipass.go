package multipass

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/kev-cao/log-console/deploy-cli/dispatch"
	"github.com/kev-cao/log-console/utils/pathutils"
	"github.com/kev-cao/log-console/utils/sliceutils"
	"golang.org/x/sync/errgroup"
)

type MultipassDispatcher struct {
	NumNodes   int
	MasterName string
	// Workers will be named $WorkerName-1, $WorkerName-2, ...
	WorkerName  string
	MasterNode  dispatch.Node
	WorkerNodes []dispatch.Node
}

var _ dispatch.ClusterDispatcher = &MultipassDispatcher{}

func NewMultipassDispatcher(numNodes int, masterName, workerName string) *MultipassDispatcher {
	dispatcher := &MultipassDispatcher{
		NumNodes:   numNodes,
		MasterName: masterName,
		WorkerName: workerName,
	}
	dispatcher.MasterNode = dispatch.Node{Name: masterName}
	for i := 1; i < numNodes; i++ {
		dispatcher.WorkerNodes = append(
			dispatcher.WorkerNodes,
			dispatch.Node{
				Name: fmt.Sprintf("%s-%d", workerName, i),
			},
		)
	}
	dispatcher.maybeGenerateNodes()
	return dispatcher
}

func (m *MultipassDispatcher) maybeGenerateNodes() error {
	var wg errgroup.Group
	for idx, node := range m.GetNodes() {
		wg.Go(func() error {
			// Check if node exists (could be not launched yet)
			cmd := exec.Command("multipass", "info", node.Name, "--format", "json")
			stdout := bytes.NewBuffer([]byte{})
			cmd.Stdout = stdout
			if err := cmd.Run(); err != nil {
				// If node does not exist, we cannot generate its node
				return nil
			}
			// Get IP address of the node
			cmd = exec.Command(
				"bash", "-c",
				fmt.Sprintf(
					`multipass info %s --format json | jq -r '.info.["%s"].ipv4[0]'`,
					node.Name, node.Name,
				),
			)
			cmd.Stderr = dispatch.NewPrefixWriter(node.Name, os.Stderr)
			cmd.WaitDelay = 5 * time.Second
			ipBytes, err := cmd.Output()
			if err != nil {
				return err
			}
			node.Remote.User = "ubuntu" // multipass by default uses ubuntu user
			node.Remote.FQDN = strings.TrimSpace(string(ipBytes))
			if idx == 0 {
				m.MasterNode = node
			} else {
				m.WorkerNodes = append(m.WorkerNodes, node)
			}
			return nil
		})
	}
	return wg.Wait()
}

func (m *MultipassDispatcher) LaunchNodes() error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	nodeNames := m.generateNodeNames()
	var wg errgroup.Group
	stdoutWriter := newLaunchWriter(os.Stdout)
	for _, name := range nodeNames {
		func(name string) {
			node := dispatch.Node{Name: name}
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
		}(name)
	}
	if err := wg.Wait(); err != nil {
		return err
	}
	if err := m.maybeGenerateNodes(); err != nil {
		return err
	}
	fmt.Println("Nodes are ready!")
	return nil
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
	return m.MasterNode
}

func (m MultipassDispatcher) GetWorkerNodes() []dispatch.Node {
	return m.WorkerNodes
}

func (m MultipassDispatcher) SendCommand(node dispatch.Node, cmds ...dispatch.Command) error {
	return m.SendCommandContext(context.Background(), node, cmds...)
}

func (m MultipassDispatcher) SendCommandContext(
	ctx context.Context,
	node dispatch.Node,
	cmds ...dispatch.Command,
) error {
	for _, cmd := range cmds {
		cmdCtx := ctx
		var cancel context.CancelFunc
		if cmd.Timeout > 0 {
			cmdCtx, cancel = context.WithTimeout(ctx, cmd.Timeout)
			defer cancel()
		}
		command := exec.CommandContext(
			cmdCtx, "multipass", "exec", node.Name, "--", "/bin/bash", "-c",
			fmt.Sprintf("%s %s", buildEnvBindings(cmd.Env), cmd.Cmd),
		)
		if cmd.Stdout == nil {
			command.Stdout = io.Discard
		} else {
			command.Stdout = cmd.Stdout
		}
		if cmd.Stderr == nil {
			command.Stderr = io.Discard
		} else {
			command.Stderr = cmd.Stderr
		}

		if err := command.Run(); err != nil {
			return err
		}
	}
	return nil
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
		path, err := pathutils.AbsolutePath(src)
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
				[]string{
					fmt.Sprintf("rm -rf /home/ubuntu/$(basename %s .git)", source),
					fmt.Sprintf("(cd /home/ubuntu && git clone %s)", source),
				},
				dispatch.WithOsPipe(),
				dispatch.WithPrefixWriter(node),
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
	nodes := sliceutils.Map(m.GetNodes(), func(node dispatch.Node, _ int) string {
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

// generateNodeNames generates node names based on the dispatcher's configuration.
// The first name is always the master node, and the rest are worker nodes.
func (m MultipassDispatcher) generateNodeNames() []string {
	var names []string
	names = append(names, m.MasterName)
	for i := 1; i < m.NumNodes; i++ {
		names = append(names, fmt.Sprintf("%s-%d", m.WorkerName, i))
	}
	return names
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
