package cmd

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCapturingPipe(t *testing.T) {
	tests := []struct {
		name          string
		bufSize       int
		capturingCond func([]byte) ([]byte, bool)
		inputs        []string
		expected      []string
	}{
		{
			name:    "matches on entire buffer matching string",
			bufSize: 5,
			capturingCond: func(b []byte) ([]byte, bool) {
				if string(b) == "world" {
					return b, true
				}
				return nil, false
			},
			inputs:   []string{"hello", "world", "za ", "world\n"},
			expected: []string{"world", "world"},
		},
		{
			name:    "matches on buffer containing string",
			bufSize: 10,
			capturingCond: func(b []byte) ([]byte, bool) {
				if strings.Contains(string(b), "world") {
					return []byte("world"), true
				}
				return nil, false
			},
			inputs:   []string{"hello", "world", "za ", "world\n"},
			expected: []string{"world", "world"},
		},
		{
			name:    "matches on buffer containing string with multiple matches",
			bufSize: 50,
			capturingCond: func(b []byte) ([]byte, bool) {
				if strings.Contains(string(b), "world") {
					return []byte("world"), true
				}
				return nil, false
			},
			inputs:   []string{"hello", "world", "za ", "world\n"},
			expected: []string{"world", "world"},
		},
		{
			name:    "regex capturing",
			bufSize: 20,
			capturingCond: func(b []byte) ([]byte, bool) {
				re := regexp.MustCompile(`\d{5}`)
				match := re.Find(b)
				if len(match) > 0 {
					return match, true
				}
				return nil, false
			},
			inputs:   []string{"hello", "wor12", "345", "za ", "67890", "world\n"},
			expected: []string{"12345", "67890"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var dest bytes.Buffer
			pipe := newCapturingPipe(&dest, test.bufSize, test.capturingCond)
			for _, input := range test.inputs {
				pipe.Write([]byte(input))
			}
			require.Len(t, pipe.Captured, len(test.expected))
			for idx, e := range test.expected {
				require.Equal(t, e, string(pipe.Captured[idx]))
			}
			require.Equal(t, strings.Join(test.inputs, ""), dest.String())
		})
	}
}
