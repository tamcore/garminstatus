package publish

import (
	"crypto/ed25519"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/tamcore/garminstatus/pkg/garminstatus"
)

func writeTestKey(t *testing.T) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	blk, err := ssh.MarshalPrivateKey(priv, "test")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(path, pem.EncodeToMemory(blk), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestNew(t *testing.T) {
	if _, err := New("file:///tmp/x", "", t.TempDir(), time.Hour); err != nil {
		t.Fatalf("New without key: %v", err)
	}
	if _, err := New("git@github.com:o/r.git", "/does/not/exist", t.TempDir(), time.Hour); err == nil {
		t.Error("New with missing key should error")
	}
	p, err := New("git@github.com:o/r.git", writeTestKey(t), t.TempDir(), time.Hour)
	if err != nil {
		t.Fatalf("New with valid key: %v", err)
	}
	if p.auth == nil {
		t.Error("expected ssh auth to be set")
	}
}

func TestPinnedHostKeyCallback(t *testing.T) {
	cb, err := pinnedHostKeyCallback()
	if err != nil {
		t.Fatalf("callback build: %v", err)
	}
	pk, _, _, _, err := ssh.ParseAuthorizedKey([]byte(githubHostKeys[0]))
	if err != nil {
		t.Fatal(err)
	}
	if err := cb("github.com:22", &net.TCPAddr{}, pk); err != nil {
		t.Errorf("pinned key should be accepted: %v", err)
	}
	pub, _, _ := ed25519.GenerateKey(nil)
	other, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	if err := cb("github.com:22", &net.TCPAddr{}, other); err == nil {
		t.Error("unknown host key should be rejected")
	}
}

func TestCyclePropagatesGitError(t *testing.T) {
	p := newTestPublisher("file:///nonexistent/repo.git", t.TempDir(),
		func() (garminstatus.GarminStatus, error) { return up("A"), nil },
		func() time.Time { return time.Unix(0, 0).UTC() },
	)
	if err := p.Cycle(); err == nil {
		t.Error("Cycle should error when the data branch cannot be opened")
	}
}
