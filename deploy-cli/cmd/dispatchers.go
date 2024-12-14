package cmd

import (
	"errors"
	"fmt"

	"github.com/kev-cao/log-console/deploy-cli/dispatch"
	"github.com/kev-cao/log-console/deploy-cli/dispatch/multipass"
	"github.com/kev-cao/log-console/deploy-cli/dispatch/ssh"
	"github.com/kev-cao/log-console/utils/sliceutils"
	"github.com/spf13/pflag"
)

var multipassDispatcher = multipass.MultipassDispatcher{
	NumNodes:   3,
	MasterName: "master",
	WorkerName: "worker",
}

var sshDispatcher = ssh.SshDispatcher{}

type dispatchMethod string

const (
	// MULTIPASS deployment methods
	MULTIPASS dispatchMethod = "multipass"
	SSH                      = "ssh"
)

var _ pflag.Value = (*dispatchMethod)(nil)
var dispatchMethodOptions = []dispatchMethod{MULTIPASS, SSH}

func (m *dispatchMethod) String() string {
	return string(*m)
}

func (m *dispatchMethod) Set(s string) error {
	switch s {
	case "multipass", "ssh":
		*m = dispatchMethod(s)
		return nil
	default:
		return errors.New(fmt.Sprintf("must be one of %v", dispatchMethodOptions))
	}
}

func (m *dispatchMethod) Type() string {
	return "method"
}

type dispatcherFactory struct {
	// Cached dispatchers
	mp  *multipass.MultipassDispatcher
	ssh *ssh.SshDispatcher
}

var dispatchers dispatcherFactory = dispatcherFactory{}

// GetDispatcher returns a dispatcher based on the deployment method. Flags are dependent on the
// deployment method.
func (f *dispatcherFactory) GetDispatcher(
	flags map[string]interface{}, method dispatchMethod,
) (dispatch.ClusterDispatcher, error) {
	switch method {
	case MULTIPASS:
		if f.mp == nil {
			f.mp = multipass.NewMultipassDispatcher(flags["NumNodes"].(int), "master", "worker")
		}
		return f.mp, nil
	case SSH:
		if f.ssh == nil {
			remotes, err := sliceutils.MapErr(
				flags["Remotes"].([]string),
				func(s string, _ int) (dispatch.UserQualifiedHostname, error) {
					var uqhn dispatch.UserQualifiedHostname
					if _, err := uqhn.ParseString(s); err != nil {
						return dispatch.UserQualifiedHostname{}, err
					}
					return uqhn, nil
				},
			)
			if err != nil {
				return nil, err
			}
			if f.ssh, err = ssh.NewSshDispatcher(
				remotes,
				flags["IdentityFile"].(string),
			); err != nil {
				return nil, err
			}
		}
		return f.ssh, nil
	default:
		return nil, fmt.Errorf("Unknown deployment method: %s", method)
	}
}
