package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/kev-cao/log-console/utils/pathutils"
	"github.com/spf13/pflag"
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
	VAULT_AUTH_NONE     vaultAuth = ""
	VAULT_AUTH_GITHUB             = "github"
	VAULT_AUTH_USERPASS           = "userpass"
)

var _ pflag.Value = (*vaultAuth)(nil)
var vaultAuthOptions = []vaultAuth{VAULT_AUTH_GITHUB, VAULT_AUTH_USERPASS}

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

func (e *vaultAuth) SignInURI(domain string) (string, error) {
	switch *e {
	case VAULT_AUTH_NONE:
		return fmt.Sprintf("https://%s/ui/vault:8200", domain), nil
	case VAULT_AUTH_GITHUB, VAULT_AUTH_USERPASS:
		return fmt.Sprintf("https://%s:8200/ui/vault/auth?with=%s", domain, *e), nil
	default:
		return "", errors.New("Invalid auth method")
	}
}
