package ssh

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/kev-cao/log-console/deploy-cli/dispatch"
	"github.com/kev-cao/log-console/utils/pathutils"
	"github.com/kev-cao/log-console/utils/sliceutils"
	"github.com/kev-cao/log-console/utils/stringutils"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

type SshDispatcher struct {
	NumNodes int
	// Remotes to SSH to. First address is the master node.
	Remotes        []dispatch.UserQualifiedHostname
	PrivateKeyFile string
	connections    map[string]*ssh.Client
	privateKeyPass string
}

type outputPipes struct {
	out io.Reader
	err io.Reader
}

var _ dispatch.ClusterDispatcher = &SshDispatcher{}

func NewSshDispatcher(remotes []dispatch.UserQualifiedHostname, privateKeyFile string) (*SshDispatcher, error) {
	dispatcher := &SshDispatcher{
		NumNodes:       len(remotes),
		Remotes:        remotes,
		PrivateKeyFile: privateKeyFile,
		connections:    make(map[string]*ssh.Client),
	}
	if err := dispatcher.init(); err != nil {
		return nil, err
	}
	return dispatcher, nil
}

// init initializes the dispatcher by connecting to all the remotes and instantiating
// a session for each.
func (s *SshDispatcher) init() error {
	signer, err := s.getPrivateKeySigner()
	if err != nil {
		return err
	}

	s.connections = make(map[string]*ssh.Client)
	for _, remote := range s.Remotes {
		// Connect to the address and add it to the connection pool
		config := &ssh.ClientConfig{
			User: remote.User,
			Auth: []ssh.AuthMethod{
				ssh.PublicKeys(signer),
			},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		}

		// Will always assume port 22 for SSH for simplicity
		client, err := ssh.Dial("tcp", remote.FQDN+":22", config)
		if err != nil {
			return fmt.Errorf("failed to connect to %s: %v", remote, err)
		}
		s.connections[remote.FQDN] = client
	}
	return nil
}

func (s *SshDispatcher) getPrivateKeySigner() (ssh.Signer, error) {
	if s.PrivateKeyFile == "" {
		s.PrivateKeyFile = filepath.Join("~", ".ssh", "id_rsa")
	}
	keyFile, err := pathutils.AbsolutePath(s.PrivateKeyFile)
	if err != nil {
		return nil, errors.New("failed to resolve private key path: " + err.Error())
	}
	key, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, errors.New("failed to read private key file: " + err.Error())
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err == nil {
		return signer, err
	}
	// crypto/ssh does not provide an `Is` method for PassphraseMissingError so
	// resorting to this.
	if _, ok := err.(*ssh.PassphraseMissingError); ok {
		fmt.Printf("Enter passphrase for %s (hidden for security): ", keyFile)
		passphrase, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return nil, errors.New("failed to read passphrase: " + err.Error())
		}
		s.privateKeyPass = string(passphrase)
		return ssh.ParsePrivateKeyWithPassphrase(key, passphrase)
	} else {
		return nil, errors.New("failed to parse private key: " + err.Error())
	}
}

func (s *SshDispatcher) Cleanup() error {
	var err error
	for _, conn := range s.connections {
		e := conn.Close()
		if e != nil {
			err = e
		}
	}
	return err
}

func (s *SshDispatcher) DownloadProject(node dispatch.Node, source string) error {
	// Make projects directory first
	if err := s.SendCommands(
		node,
		dispatch.NewCommand(
			"mkdir -p ~/projects",
			dispatch.WithOsPipe(),
			dispatch.WithPrefixWriter(node),
		),
	); err != nil {
		return err
	}
	if strings.HasPrefix(source, "local://") {
		return s.downloadProjectLocal(node, source)
	} else {
		return s.downloadProjectGit(node, source)
	}
}

func (s *SshDispatcher) GetMasterNode() dispatch.Node {
	return dispatch.Node{Name: s.Remotes[0].FQDN, Kubename: "master", Remote: s.Remotes[0]}
}

func (s *SshDispatcher) GetNodes() []dispatch.Node {
	return sliceutils.Map(s.Remotes, func(remote dispatch.UserQualifiedHostname, i int) dispatch.Node {
		node := dispatch.Node{Name: remote.FQDN, Remote: remote}
		if i == 0 {
			node.Kubename = "master"
		} else {
			node.Kubename = fmt.Sprintf("worker-%d", i)
		}
		return node
	})
}

func (s *SshDispatcher) GetWorkerNodes() []dispatch.Node {
	return sliceutils.Map(s.Remotes[1:], func(remote dispatch.UserQualifiedHostname, i int) dispatch.Node {
		return dispatch.Node{
			Name:     remote.FQDN,
			Kubename: fmt.Sprintf("worker-%d", i+1),
			Remote:   remote,
		}
	})
}

func (s *SshDispatcher) Ready() bool {
	return len(s.connections) == s.NumNodes
}

func (s *SshDispatcher) SendCommands(node dispatch.Node, cmds ...dispatch.Command) error {
	return s.SendCommandsContext(context.Background(), node, cmds...)
}

func (s *SshDispatcher) SendCommandsContext(ctx context.Context, node dispatch.Node, cmds ...dispatch.Command) error {
	for _, cmd := range cmds {
		client, ok := s.connections[node.Name]
		if !ok {
			return errors.New("no connection found for node " + node.Name)
		}
		session, err := client.NewSession()
		if err != nil {
			return errors.New(fmt.Sprintf("failed to create session for %s: %v", node.Name, err))
		}
		defer session.Close()
		session.Stdout = cmd.Stdout()
		session.Stderr = cmd.Stderr()
		if cmd.Timeout() > 0 {
			time.AfterFunc(cmd.Timeout(), func() {
				session.Signal(ssh.SIGTERM)
			})
		}
		if err := session.Run(
			fmt.Sprintf("%s %s", stringutils.BuildEnvBindings(cmd.Env()), cmd.Cmd()),
		); err != nil {
			return err
		}
	}
	return nil
}

func (s *SshDispatcher) SendFile(node dispatch.Node, src string, dst string) error {
	var errBytes bytes.Buffer
	cmd := exec.Command("scp", "-i", s.PrivateKeyFile, src, fmt.Sprintf("%s:%s", node.Name, dst))
	cmd.Stderr = &errBytes
	if err := cmd.Run(); err != nil {
		return errors.New(errBytes.String())
	}
	return nil
}

func (s *SshDispatcher) downloadProjectLocal(node dispatch.Node, source string) error {
	src := strings.TrimPrefix(source, "local://")
	path, err := pathutils.AbsolutePath(src)
	if err != nil {
		return err
	}
	basePath := filepath.Base(path)
	if err := s.SendCommands(node, dispatch.NewCommand(
		"rm -rf ~/projects/"+basePath,
		dispatch.WithOsPipe(),
		dispatch.WithPrefixWriter(node),
	)); err != nil {
		return err
	}
	scpCmd := exec.Command(
		"scp",
		"-i",
		s.PrivateKeyFile,
		"-r",
		path,
		fmt.Sprintf("%s:~/projects/%s", node.Name, basePath),
	)
	scpCmd.Stdout = dispatch.NewPrefixWriter(node.Name, os.Stdout)
	scpCmd.Stderr = dispatch.NewPrefixWriter(node.Name, os.Stderr)
	if err := scpCmd.Run(); err != nil {
		return err
	}
	return nil
}

func (s *SshDispatcher) downloadProjectGit(node dispatch.Node, source string) error {
	return s.SendCommands(
		node,
		dispatch.NewCommands(
			[]string{
				fmt.Sprintf("rm -rf ~/projects/$(basename %s .git)", source),
				fmt.Sprintf("(cd ~/projects && git clone %s)", source),
			},
			dispatch.WithOsPipe(),
			dispatch.WithPrefixWriter(node),
		)...,
	)
}
