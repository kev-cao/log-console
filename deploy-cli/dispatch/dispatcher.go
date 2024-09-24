package dispatch

import (
	"context"
	"io"
	"os"
	"time"
)

type Command struct {
	Cmd     string
	Env     map[string]string
	Stdout  io.Writer
	Stderr  io.Writer
	Timeout time.Duration
}

// NewCommand creates a command object that default outputs to stdout and stderr
// with node information.
func NewCommand(node Node, cmd string, env map[string]string, timeout time.Duration) Command {
	return Command{
		Cmd:     cmd,
		Env:     env,
		Stdout:  NewPrefixWriter(node.Name, os.Stdout),
		Stderr:  NewPrefixWriter(node.Name, os.Stderr),
		Timeout: 0,
	}
}

// NewCommands creates a list of command objects that default outputs to stdout and stderr
func NewCommands(node Node, timeout time.Duration, cmds ...string) []Command {
	var commands []Command
	for _, cmd := range cmds {
		commands = append(commands, NewCommand(node, cmd, nil, timeout))
	}
	return commands
}

type Node struct {
	Name string
	IPv4 string
}

type ClusterDispatcher interface {
	// GetNodes returns all nodes in the cluster.
	GetNodes() []Node
	// GetMasterNode returns the master node in the cluster.
	GetMasterNode() Node
	// GetWorkerNodes returns all worker nodes in the cluster.
	GetWorkerNodes() []Node
	// Ready checks if the cluster is ready to accept commands.
	Ready() bool
	// SendCommand sends a command to a node in the cluster.
	SendCommand(node Node, cmds ...Command) error
	// SendCommandContext sends a command to a node in the cluster with a custom context.
	SendCommandContext(ctx context.Context, node Node, cmds ...Command) error
	// SendCommandEnv sends a command to a node in the cluster with a custom environment.
	SendCommandEnv(node Node, env map[string]string, cmds ...Command) error
	// SendCommandEnvContext sends a command to a node in the cluster with a custom context and environment.
	SendCommandEnvContext(ctx context.Context, node Node, env map[string]string, cmds ...Command) error
	// SendFile sends a file to a node in the cluster.
	SendFile(node Node, src, dst string) error
	// DownloadProject sets up the log-console project into the given node. If the source begins with
	// local://, it'll mount the local directory into the node. For all other sources, it'll git
	// clone the URL.
	DownloadProject(node Node, source string) error
	// TearDown tears down the cluster.
	TearDown() error
}

type PrefixWriter struct {
	prefix string
	writer io.Writer
}

func NewPrefixWriter(prefix string, writer io.Writer) io.Writer {
	if writer == nil {
		return nil
	}
	return &PrefixWriter{prefix: prefix, writer: writer}
}

// Write writes the prefix before the actual bytes to the underlying writer.
func (p PrefixWriter) Write(b []byte) (n int, err error) {
	if p.writer == nil {
		return len(b), nil
	}
	if _, err := p.writer.Write(append([]byte("["+p.prefix+"] "), b...)); err != nil {
		return 0, err
	}
	return len(b), nil
}
