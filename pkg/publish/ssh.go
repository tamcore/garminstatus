package publish

import (
	"bytes"
	"fmt"
	"net"
	"os"

	"github.com/go-git/go-git/v5/plumbing/transport"
	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"golang.org/x/crypto/ssh"
)

// githubHostKeys are GitHub's published SSH host public keys. Pinning them means
// a push cannot be silently redirected to an impostor host.
// Source: https://docs.github.com/authentication/keeping-your-account-and-data-secure/githubs-ssh-key-fingerprints
var githubHostKeys = []string{
	"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl",
	"ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBEmKSENjQEezOmxkZMy7opKgwFB9nkt5YRrYMjNuG5N87uRgg6CLrbo5wAdT/y6v0mKV0U2w0WZ2YB/++Tpockg=", //nolint:lll
	"ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQCj7ndNxQowgcQnjshcLrqPEiiphnt+VTTvDP6mHBL9j1aNUkY4Ue1gvwnGLVlOhGeYrnZaMgRK6+PKCUXaDbC7qtbW8gIkhL7aGCsOr/C56SJMy/BCZfxd1nWzAOxSDPgVsmerOBYfNqltV9/hWCqBywINIR+5dIg6JTJ72pcEpEjcYgXkE2YEFXV1JHnsKgbLWNlhScqb2UmyRkQyytRLtL+38TGxkxCflmO+5Z8CSSNY7GidjMIZ7Q4zMjA2n1nGrlTDkzwDCsw+wqFPGQA179cnfGWOWRVruj16z6XyvxvjJwbz0wQZ75XK5tKSb7FNyeIEs4TT4jk+S4dhPeAUC5y+bDYirYgM4GC7uEnztnZyaVWQ7B381AK4Qdrwt51ZqExKbQpTUNn+EjqoTwvqNj4kqx5QUCI0ThS/YkOxJCXmPUWZbhjpCg56i+2aB6CmK2JGhn57K5mj0MNdBXA4/WnwH6XoPWJzK5Nyu2zB3nAZp+S5hpQs+p1vN1/wsjk=", //nolint:lll
}

// sshAuth builds a go-git SSH auth method from a private key file, pinning the
// GitHub host keys.
func sshAuth(keyPath string) (transport.AuthMethod, error) {
	pem, err := os.ReadFile(keyPath) //nolint:gosec // operator-controlled path
	if err != nil {
		return nil, fmt.Errorf("read ssh key %s: %w", keyPath, err)
	}
	auth, err := gitssh.NewPublicKeys("git", pem, "")
	if err != nil {
		return nil, fmt.Errorf("parse ssh key: %w", err)
	}
	cb, err := pinnedHostKeyCallback()
	if err != nil {
		return nil, err
	}
	auth.HostKeyCallback = cb
	return auth, nil
}

// pinnedHostKeyCallback accepts a connection only if the presented host key
// matches one of the pinned GitHub keys.
func pinnedHostKeyCallback() (ssh.HostKeyCallback, error) {
	pinned := make([][]byte, 0, len(githubHostKeys))
	for _, line := range githubHostKeys {
		pk, _, _, _, err := ssh.ParseAuthorizedKey([]byte(line))
		if err != nil {
			// Skip a malformed constant rather than crash — as long as at
			// least one key pins, host verification still works.
			continue
		}
		pinned = append(pinned, pk.Marshal())
	}
	if len(pinned) == 0 {
		return nil, fmt.Errorf("no valid pinned GitHub host keys")
	}
	return func(_ string, _ net.Addr, key ssh.PublicKey) error {
		got := key.Marshal()
		for _, p := range pinned {
			if bytes.Equal(got, p) {
				return nil
			}
		}
		return fmt.Errorf("unknown host key type %s (not a pinned GitHub key)", key.Type())
	}, nil
}
