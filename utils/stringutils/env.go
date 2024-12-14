package stringutils

import "strings"

// BuildEnvBindings converts a map of environment variables to a space-separate list of bindings
// in the form "key=value".
func BuildEnvBindings(env map[string]string) string {
	var envBindings []string
	for k, v := range env {
		envBindings = append(envBindings, k+"="+v)
	}
	return strings.Join(envBindings, " ")
}
