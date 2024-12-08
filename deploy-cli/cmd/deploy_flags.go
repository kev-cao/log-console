package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/pflag"
)

type deployFlags struct {
	Method       dispatchMethod
	Env          env
	NumNodes     int
	Remotes      []string
	Launch       bool
	IdentityFile string
	SetupK3S     bool
}

func (f *deployFlags) validate() error {
	if f.NumNodes <= 0 {
		return errors.New("Number of nodes must be greater than 0.")
	}

	if f.Launch {
		if f.Method != MULTIPASS {
			return errors.New("Launch flag is only supported for multipass deployments.")
		}
		f.SetupK3S = true
	}

	if f.Method == SSH {
		if len(f.Remotes) == 0 {
			return errors.New("Remote addresses must be provided for SSH deployments.")
		} else if len(f.Remotes) != f.NumNodes {
			return errors.New("Number of remotes must match number of nodes.")
		} else if f.IdentityFile == "" {
			return errors.New("Private key file must be provided for SSH deployments.")
		}
	}
	return nil
}

type env string

const (
	DEV  env = "dev"
	PROD     = "prod"
)

var _ pflag.Value = (*env)(nil)
var envOptions = []env{DEV, PROD}

func (e *env) String() string {
	return string(*e)
}

func (e *env) Set(s string) error {
	switch s {
	case "dev", "prod":
		*e = env(s)
	default:
		return errors.New(fmt.Sprintf("must be one of %v", envOptions))
	}
	return nil
}

func (e *env) Type() string {
	return "env"
}
