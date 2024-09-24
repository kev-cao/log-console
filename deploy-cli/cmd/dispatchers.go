package cmd

import (
	"fmt"

	"github.com/kev-cao/log-console/deploy-cli/dispatch"
	"github.com/kev-cao/log-console/deploy-cli/dispatch/multipass"
)

var multipassDispatcher = multipass.MultipassDispatcher{
	NumNodes:   3,
	MasterName: "master",
	WorkerName: "worker",
}

func getDispatcher(method string) (dispatch.ClusterDispatcher, error) {
	switch method {
	case "multipass":
		return multipassDispatcher, nil
	case "ssh":
		fallthrough
	case "local":
		return nil, fmt.Errorf("Deployment method %s not yet implemented", method)
	}
	return nil, fmt.Errorf("Unsupported deployment method: %s", method)
}
