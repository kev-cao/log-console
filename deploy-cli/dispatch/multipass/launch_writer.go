package multipass

import (
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/kev-cao/log-console/deploy-cli/dispatch"
)

// Since multipass rewrites the same line when launching a node, we need to
// create a custom writer that shows the launch progress of each node.

type nodeLaunchWriter struct {
	parent *launchWriter
	node   *dispatch.Node
	bufPos int
}

// This is what multipass writes to the terminal to clear the line
var multipassClearLine []byte = []byte("\x1B[2K\x1B[0A\x1B[0E")

// Write writes the contents of a multipass launch process for a node to the
// parent launchWriter, clearing the line if the multipass clear line is detected.
func (n *nodeLaunchWriter) Write(b []byte) (int, error) {
	toWrite := make([]byte, 0, len(b))
	for _, c := range b {
		if c == multipassClearLine[n.bufPos] {
			n.bufPos++
			if n.bufPos == len(multipassClearLine) {
				n.parent.clearLine(n.node)
				n.bufPos = 0
				toWrite = nil
			}
		} else {
			toWrite = append(toWrite, c)
			n.bufPos = 0
		}
	}
	if err := n.parent.appendLine(n.node, toWrite); err != nil {
		return 0, err
	}
	return len(b), nil
}

type launchWriter struct {
	nodes      []*dispatch.Node
	nodeOutput map[string]string
	writer     io.Writer
	outputMu   sync.Mutex
	writeMu    sync.Mutex
	hasWritten bool
}

// newLaunchWriter creates a new launchWriter with the given writer.
func newLaunchWriter(w io.Writer) launchWriter {
	return launchWriter{
		nodeOutput: make(map[string]string),
		writer:     w,
	}
}

// newNodeWriter creates a new nodeLaunchWriter for a node belonging to this launchWriter.
func (l *launchWriter) newNodeWriter(n *dispatch.Node) *nodeLaunchWriter {
	nodeWriter := &nodeLaunchWriter{
		node: n,
	}
	l.register(nodeWriter)
	return nodeWriter
}

// register registers a nodeLaunchWriter to this launchWriter.
func (l *launchWriter) register(w *nodeLaunchWriter) {
	w.parent = l
	l.nodes = append(l.nodes, w.node)
	l.nodeOutput[w.node.Name] = ""
}

// clearLine removes the body text of a node's output. It does not write
// to the terminal.
func (l *launchWriter) clearLine(node *dispatch.Node) {
	l.outputMu.Lock()
	defer l.outputMu.Unlock()
	l.nodeOutput[node.Name] = fmt.Sprintf("[%s] ", node.Name)
}

// setLine sets the body text of a node's output and then writes to the terminal.
func (l *launchWriter) setLine(node *dispatch.Node, line string) error {
	l.outputMu.Lock()
	defer l.outputMu.Unlock()
	l.nodeOutput[node.Name] = fmt.Sprintf("[%s] %s", node.Name, line)
	return l.write()
}

// appendLine appends bytes to a node's output and then writes to the terminal.
func (l *launchWriter) appendLine(node *dispatch.Node, b []byte) error {
	l.outputMu.Lock()
	defer l.outputMu.Unlock()
	if l.nodeOutput[node.Name] == "" {
		l.nodeOutput[node.Name] = fmt.Sprintf("[%s] ", node.Name)
	}
	output := strings.ReplaceAll(string(b), "\n", fmt.Sprintf("\n[%s] ", node.Name))
	l.nodeOutput[node.Name] += output
	return l.write()
}

// write writes the output of all nodes to the terminal.
func (l *launchWriter) write() error {
	l.writeMu.Lock()
	defer l.writeMu.Unlock()
	// Clear the previous lines in the terminal
	if l.hasWritten {
		for i := 0; i < len(l.nodes); i++ {
			if _, err := fmt.Fprintf(l.writer, string([]byte("\x1B[1F\x1B[2K"))); err != nil {
				return err
			}
		}
	}

	// Rewrite the lines
	for _, n := range l.nodes {
		if _, err := fmt.Fprintln(l.writer, l.nodeOutput[n.Name]); err != nil {
			return err
		}
	}
	l.hasWritten = true
	return nil
}
