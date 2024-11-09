package ssh

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/kev-cao/log-console/deploy-cli/dispatch"
	"github.com/kev-cao/log-console/utils/pathutils"
	"github.com/kev-cao/log-console/utils/sliceutils"
	"golang.org/x/crypto/ssh"
)

type SshDispatcher struct {
	NumNodes int
	// Remotes to SSH to. First address is the master node.
	Remotes        []dispatch.UserQualifiedHostname
	PrivateKeyFile string
	connections    map[string]*ssh.Client
	sessions       map[string]*ssh.Session
}

var _ dispatch.ClusterDispatcher = &SshDispatcher{}

func NewSshDispatcher(remotes []dispatch.UserQualifiedHostname, privateKeyFile string) (*SshDispatcher, error) {
	dispatcher := &SshDispatcher{
		NumNodes:       len(remotes),
		Remotes:        remotes,
		PrivateKeyFile: privateKeyFile,
		connections:    make(map[string]*ssh.Client),
		sessions:       make(map[string]*ssh.Session),
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

		client, err := ssh.Dial("tcp", remote.FQDN, config)
		if err != nil {
			return fmt.Errorf("failed to connect to %s: %v", remote, err)
		}
		session, err := client.NewSession()
		if err != nil {
			return fmt.Errorf("failed to create session for %s: %v", remote, err)
		}
		s.connections[remote.String()] = client
		s.sessions[remote.String()] = session
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
	if errors.Is(err, &ssh.PassphraseMissingError{}) {
		fmt.Printf("Enter passphrase for %s: ", keyFile)
		var passphrase string
		_, err := fmt.Scanln(&passphrase)
		if err != nil {
			return nil, errors.New("failed to read passphrase: " + err.Error())
		}
		return ssh.ParsePrivateKeyWithPassphrase(key, []byte(passphrase))
	} else {
		return nil, errors.New("failed to parse private key: " + err.Error())
	}
}

func (s *SshDispatcher) Cleanup() {
	for _, session := range s.sessions {
		session.Close()
	}
	for _, conn := range s.connections {
		conn.Close()
	}
}

func (s *SshDispatcher) DownloadProject(node dispatch.Node, source string) error {
	// Make projects directory first
	if err := s.SendCommand(
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
		src := strings.TrimPrefix(source, "local://")
		path, err := pathutils.AbsolutePath(src)
		if err != nil {
			return err
		}
		if err := exec.Command(
			"scp",
			"-i",
			s.PrivateKeyFile,
			"-r",
			path,
			fmt.Sprintf("%s:~/projects/$(basename %s)", node.Name, path),
		).Run(); err != nil {
			return err
		}
	} else {
		if err := s.SendCommand(
			node,
			dispatch.NewCommands(
				[]string{
					fmt.Sprintf("rm -rf ~/projects/$(basename %s .git)", source),
					fmt.Sprintf("(cd ~/projects && git clone %s)", source),
				},
				dispatch.WithOsPipe(),
				dispatch.WithPrefixWriter(node),
			)...,
		); err != nil {
			return err
		}
	}
	return nil
}

func (s *SshDispatcher) GetMasterNode() dispatch.Node {
	return dispatch.Node{Name: s.Remotes[0].String(), Remote: s.Remotes[0]}
}

func (s *SshDispatcher) GetNodes() []dispatch.Node {
	return sliceutils.Map(s.Remotes, func(remote dispatch.UserQualifiedHostname, _ int) dispatch.Node {
		return dispatch.Node{Name: remote.String(), Remote: remote}
	})
}

func (s *SshDispatcher) GetWorkerNodes() []dispatch.Node {
	return sliceutils.Map(s.Remotes[1:], func(remote dispatch.UserQualifiedHostname, _ int) dispatch.Node {
		return dispatch.Node{Name: remote.String(), Remote: remote}
	})
}

func (s *SshDispatcher) Ready() bool {
	return len(s.connections) == s.NumNodes
}

func (s *SshDispatcher) SendCommand(node dispatch.Node, cmds ...dispatch.Command) error {
	return s.SendCommandContext(context.Background(), node, cmds...)
}

func (s *SshDispatcher) SendCommandContext(ctx context.Context, node dispatch.Node, cmds ...dispatch.Command) error {
	for _, cmd := range cmds {
		session := s.sessions[node.Name]
		session.Stdout = cmd.Stdout
		session.Stderr = cmd.Stderr
		if cmd.Timeout > 0 {
			time.AfterFunc(cmd.Timeout, func() {
				session.Signal(ssh.SIGTERM)
			})
		}
		if err := session.Run(cmd.Cmd); err != nil {
			return err
		}
	}
	return nil
}

func (s *SshDispatcher) SendFile(node dispatch.Node, src string, dst string) error {
	return exec.Command("scp", "-i", s.PrivateKeyFile, src, fmt.Sprintf("%s:%s", node.Name, dst)).Run()
}

func (s *SshDispatcher) TearDown() error {
	s.Cleanup()
	return nil
}
