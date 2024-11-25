package dispatch

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/kev-cao/log-console/utils/sliceutils"
)

type Command struct {
	Cmd     string
	Env     map[string]string
	Stdout  io.Writer
	Stderr  io.Writer
	Timeout time.Duration
}

type optionLoader func(Command) Command

// NewCommand creates a command object from a command string with provided options.
func NewCommand(cmdStr string, opts ...optionLoader) Command {
	cmd := Command{Cmd: cmdStr}
	for _, opt := range opts {
		cmd = opt(cmd)
	}
	if cmd.Stdout == nil {
		cmd.Stdout = io.Discard
	}
	if cmd.Stderr == nil {
		cmd.Stderr = io.Discard
	}
	return cmd
}

// NewCommands creates a list of command objects from the provided command strings,
// and options.
func NewCommands(cmdStrs []string, opts ...optionLoader) []Command {
	return sliceutils.Map(cmdStrs, func(cmdStr string, _ int) Command {
		return NewCommand(cmdStr, opts...)
	})
}

// WithTimeout sets the timeout for the command.
func WithTimeout(timeout time.Duration) optionLoader {
	return func(c Command) Command {
		c.Timeout = timeout
		return c
	}
}

// WithOsPipe sets the stdout and stderr of the command to `os.Stdout` and `os.Stderr`.
func WithOsPipe() optionLoader {
	return func(c Command) Command {
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		return c
	}
}

// WithStdout sets the stdout writer for the command. If `nil`, it defaults to `os.Stdout`.
func WithStdout(w io.Writer) optionLoader {
	return func(c Command) Command {
		c.Stdout = w
		return c
	}
}

// WithStderr sets the stderr writer for the command. If `nil`, it defaults to `os.Stderr`.
func WithStderr(w io.Writer) optionLoader {
	return func(c Command) Command {
		c.Stderr = w
		return c
	}
}

// WithPrefixWriter sets the prefix writer for the command. Must be used after
// WithStdout or WithStderr.
func WithPrefixWriter(node Node) optionLoader {
	return func(c Command) Command {
		if c.Stdout != nil {
			c.Stdout = NewPrefixWriter(node.Name, c.Stdout)
		}
		if c.Stderr != nil {
			c.Stderr = NewPrefixWriter(node.Name, c.Stderr)
		}
		return c
	}
}

func WithEnv(env map[string]string) optionLoader {
	return func(c Command) Command {
		c.Env = env
		return c
	}
}

type UserQualifiedHostname struct {
	User string
	FQDN string
}

func (r UserQualifiedHostname) String() string {
	return fmt.Sprintf("%s@%s", r.User, r.FQDN)
}

func (r *UserQualifiedHostname) ParseString(s string) (*UserQualifiedHostname, error) {
	scanned, err := fmt.Sscanf(s, "%s@%s", &r.User, &r.FQDN)
	if scanned != 2 {
		return r, fmt.Errorf("failed to parse user qualified hostname: %s", s)
	}
	return r, err
}

type Node struct {
	Name   string
	Remote UserQualifiedHostname
}

type App string

const (
	All   App = "all"
	Vault App = "vault"
)

type ClusterDispatcher interface {
	// GetNodes returns all nodes in the cluster.
	GetNodes() []Node
	// GetMasterNode returns the master node in the cluster.
	GetMasterNode() Node
	// GetWorkerNodes returns all worker nodes in the cluster.
	GetWorkerNodes() []Node
	// Ready checks if the cluster is ready to accept commands.
	Ready() bool
	// SendCommands sends commands to a node in the cluster.
	SendCommands(node Node, cmds ...Command) error
	// SendCommandsContext sends commands to a node in the cluster with a custom context.
	SendCommandsContext(ctx context.Context, node Node, cmds ...Command) error
	// SendFile sends a file to a node in the cluster.
	SendFile(node Node, src, dst string) error
	// DownloadProject sets up the log-console project into the given node. If the source begins with
	// local://, it'll mount the local directory into the node. For all other sources, it'll git
	// clone the URL.
	DownloadProject(node Node, source string) error
	// Teardown tears down apps from the cluster
	Teardown(app App) error
	// Cleanup disposes of any held resources.
	Cleanup() error
}

type PrefixWriter struct {
	prefix string
	writer io.Writer
	// Both hasWritten and lastWroteNewline are used to determine if the prefix
	// needs to be written. If hasWritten is false, the prefix is written. If
	// lastWroteNewline is true, then on the next write, the prefix will be written.
	// lastWroteNewline means that the lsat time Write was called, a newline was written
	// as the last character.
	hasWritten       bool
	lastWroteNewline bool
}

func NewPrefixWriter(prefix string, writer io.Writer) io.Writer {
	if writer == nil {
		return nil
	}
	return &PrefixWriter{prefix: prefix, writer: writer}
}

// Write writes the prefix before the actual bytes to the underlying writer.
func (p *PrefixWriter) Write(b []byte) (n int, err error) {
	if p.writer == nil {
		return len(b), nil
	}
	s := string(b)
	toWrite := make([]byte, 0, len(b)+len(p.prefix)+3)
	if !p.hasWritten || p.lastWroteNewline {
		toWrite = append(toWrite, []byte("["+p.prefix+"] ")...)
		p.hasWritten = true
	}
	p.lastWroteNewline = false
	for i, c := range s {
		toWrite = append(toWrite, byte(c))
		if c == '\n' {
			if i != len(s)-1 {
				toWrite = append(toWrite, []byte("["+p.prefix+"] ")...)
			} else {
				p.lastWroteNewline = true
			}
		}
	}
	if _, err := p.writer.Write(toWrite); err != nil {
		return 0, err
	}
	return len(b), nil
}

func DeleteVaultResources(d ClusterDispatcher) error {
	master := d.GetMasterNode()
	if err := d.SendCommands(
		master,
		NewCommands(
			[]string{
				"helm uninstall vault -n vault --ignore-not-found",
				"helm uninstall cert-manager -n cert-manager --ignore-not-found",
				"helm uninstall cert-manager-approver-policy -n cert-manager --ignore-not-found",
				"helm uninstall trust-manager -n cert-manager --ignore-not-found",
				"kubectl delete -l app=vault --all-namespaces " +
					"$(kubectl api-resources --verbs=delete -o name | tr \"\\n\" \",\" | sed -e 's/,$//')",
			},
			WithEnv(map[string]string{
				"KUBECONFIG": "/etc/rancher/k3s/k3s.yaml",
			}),
			WithOsPipe(),
		)...,
	); err != nil {
		return fmt.Errorf("error deleting vault resources: %w", err)
	}

	for _, node := range d.GetNodes() {
		if err := d.SendCommands(
			node,
			NewCommand(
				"sudo rm -rf /srv/cluster/storage/vault",
				WithOsPipe(),
			),
		); err != nil {
			return fmt.Errorf("error cleaning up vault storage on %s: %w", node.Name, err)
		}
	}

	return nil
}
