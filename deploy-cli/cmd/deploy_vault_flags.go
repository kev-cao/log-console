package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/kev-cao/log-console/utils/pathutils"
)

type vaultFlags struct {
	deployFlags
	Creds          string
	KeysOutputFile string
	Auth           vaultAuth
}

func (f *vaultFlags) validate() error {
	if f.Creds == "" {
		return errors.New("Cloud credentials file must be provided.")
	}
	creds, _ := pathutils.AbsolutePath(f.Creds)
	if _, err := os.Stat(creds); os.IsNotExist(err) {
		return errors.New("Cloud credentials file does not exist.")
	}
	return nil
}

type vaultAuth string

const (
	vaultAuthGithub   vaultAuth = "github"
	vaultAuthUserpass vaultAuth = "userpass"
)

var vaultAuthOptions []vaultAuth = []vaultAuth{vaultAuthGithub, vaultAuthUserpass}

func (a *vaultAuth) String() string {
	return string(*a)
}

func (a *vaultAuth) Set(s string) error {
	switch s {
	case "github", "userpass":
		*a = vaultAuth(s)
		return nil
	default:
		return errors.New(fmt.Sprintf("must be one of %v", vaultAuthOptions))
	}
}

func (e *vaultAuth) Type() string {
	return "auth"
}
