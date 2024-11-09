package cmd

import (
	"fmt"

	"github.com/kev-cao/log-console/deploy-cli/dispatch"
	"github.com/kev-cao/log-console/deploy-cli/dispatch/multipass"
	"github.com/kev-cao/log-console/deploy-cli/dispatch/ssh"
	"github.com/kev-cao/log-console/utils/sliceutils"
)

var multipassDispatcher = multipass.MultipassDispatcher{
	NumNodes:   3,
	MasterName: "master",
	WorkerName: "worker",
}

var sshDispatcher = ssh.SshDispatcher{}

type dispatchMethod string

const (
	// Multipass deployment methods
	Multipass dispatchMethod = "multipass"
	SSH                      = "ssh"
)

type dispatcherFactory struct {
	// Cached dispatchers
	mp  *multipass.MultipassDispatcher
	ssh *ssh.SshDispatcher
}

var dispatchers dispatcherFactory = dispatcherFactory{}

func (f *dispatcherFactory) GetDispatcher(
	flags map[string]interface{}, method dispatchMethod,
) (dispatch.ClusterDispatcher, error) {
	switch method {
	case Multipass:
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
				return nil, nil
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
