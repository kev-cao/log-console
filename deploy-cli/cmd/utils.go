package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/kev-cao/log-console/utils/structures"
)

// header returns a string with the provided string in a header.
func header(s string) string {
	dividerN := 40 // Minimum number of characters in divider
	padding := 5   // Right padding for the string
	if len(s)+padding > dividerN {
		dividerN = len(s) + padding
	}
	divider := strings.Repeat("-", dividerN)
	return fmt.Sprintf("\x1b[32;1m%s\n%s\n%s\x1b[0m", divider, s, divider)
}

// capturingPipe is a passthrough io.Writer that captures data written to it based on a condition and also
// writes through to its underlying writer.
type capturingPipe struct {
	Captured      [][]byte
	writer        io.Writer
	buf           *structures.CircularBuffer[byte]
	capturingCond func([]byte) ([]byte, bool)
}

var _ io.Writer = &capturingPipe{}

func newCapturingPipe(writer io.Writer, bufSize int, capturingCond func([]byte) ([]byte, bool)) *capturingPipe {
	if writer == nil {
		writer = io.Discard
	}
	return &capturingPipe{
		writer:        writer,
		buf:           structures.NewCircularBuffer[byte](bufSize),
		capturingCond: capturingCond,
	}
}

func (c *capturingPipe) Write(p []byte) (n int, err error) {
	for _, b := range p {
		c.buf.Add(b)
		captured, ok := c.capturingCond(c.buf.Get())
		if ok {
			c.Captured = append(c.Captured, captured)
			c.buf.Clear()
		}
	}
	return c.writer.Write(p)
}
