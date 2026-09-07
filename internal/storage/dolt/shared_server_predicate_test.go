package dolt

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSharedServerDatabase pins which topologies the #5920 consent gate
// applies to. Getting this wrong is expensive in both directions: too broad
// and bd refuses to migrate a workspace's own database (the release blocker
// this predicate was introduced to fix), too narrow and an upgraded client
// silently promotes the schema for a whole team.
//
// Every case fixes the environment explicitly — the predicate reads process
// state, so a leaked BEADS_DOLT_* from another test would otherwise decide the
// answer.
func TestSharedServerDatabase(t *testing.T) {
	// A workspace with no metadata.json: doltserver resolves it as a server
	// whose lifecycle bd owns.
	ownedWorkspace := func(t *testing.T) string {
		t.Helper()
		dir := filepath.Join(t.TempDir(), ".beads")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		return dir
	}

	clearEnv := func(t *testing.T) {
		t.Helper()
		for _, k := range []string{
			"BEADS_DOLT_SHARED_SERVER", "BEADS_DOLT_SERVER_MODE",
			"BEADS_DOLT_SERVER_HOST", "BEADS_DOLT_SERVER_PORT", "BEADS_DOLT_PORT",
		} {
			t.Setenv(k, "")
		}
	}

	t.Run("bd-owned local server is not shared", func(t *testing.T) {
		clearEnv(t)
		cfg := &Config{BeadsDir: ownedWorkspace(t), ServerHost: "127.0.0.1", ServerPort: 3307}
		if sharedServerDatabase(cfg) {
			t.Fatal("a server bd auto-started for this workspace must migrate on open, as embedded does")
		}
	})

	t.Run("no workspace fails closed", func(t *testing.T) {
		clearEnv(t)
		// A bare dolt.New pointed at an endpoint: nothing proves bd owns it,
		// and a library caller attached to a team's server must not migrate it.
		cfg := &Config{ServerHost: "127.0.0.1", ServerPort: 3307}
		if !sharedServerDatabase(cfg) {
			t.Fatal("without a workspace to prove ownership, the database must be treated as shared")
		}
		if !sharedServerDatabase(nil) {
			t.Fatal("a nil config must be treated as shared")
		}
	})

	t.Run("shared-server mode is shared", func(t *testing.T) {
		clearEnv(t)
		t.Setenv("BEADS_DOLT_SHARED_SERVER", "1")
		cfg := &Config{BeadsDir: ownedWorkspace(t), ServerHost: "127.0.0.1", ServerPort: 3308}
		if !sharedServerDatabase(cfg) {
			t.Fatal("shared-server mode is one server for many workspaces — the #5920 shape")
		}
	})

	t.Run("explicit external server mode is shared", func(t *testing.T) {
		clearEnv(t)
		t.Setenv("BEADS_DOLT_SERVER_MODE", "1")
		cfg := &Config{BeadsDir: ownedWorkspace(t), ServerHost: "127.0.0.1", ServerPort: 3307}
		if !sharedServerDatabase(cfg) {
			t.Fatal("an operator-managed server is not bd's to migrate unprompted")
		}
	})

	for _, tt := range []struct {
		name string
		cfg  Config
	}{
		{name: "remote host", cfg: Config{ServerHost: "db.example.com", ServerPort: 3307}},
		{name: "TLS endpoint", cfg: Config{ServerHost: "127.0.0.1", ServerPort: 3307, ServerTLS: true}},
		{name: "unix socket", cfg: Config{ServerHost: "127.0.0.1", ServerSocket: "/tmp/dolt.sock"}},
	} {
		t.Run(tt.name+" is shared", func(t *testing.T) {
			clearEnv(t)
			cfg := tt.cfg
			cfg.BeadsDir = ownedWorkspace(t)
			if !sharedServerDatabase(&cfg) {
				t.Fatalf("%s cannot be a server bd auto-started for this workspace", tt.name)
			}
		})
	}
}
