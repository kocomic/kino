package main

import (
	"bytes"

	"database/sql"

	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"strings"
	"testing"

	"kino/internal/buildinfo"

	"kino/internal/catalog"
)

func TestLoopbackAddress(t *testing.T) {
	tests := []struct {
		address string
		want    bool
	}{
		{"127.0.0.1:8080", true},
		{"[::1]:8080", true},
		{"localhost:8080", true},
		{"0.0.0.0:8080", false},
		{"192.168.1.20:8080", false},
		{":8080", false},
		{"not-an-address", false},
	}
	for _, test := range tests {
		t.Run(test.address, func(t *testing.T) {
			if got := loopbackAddress(test.address); got != test.want {
				t.Fatalf("loopbackAddress(%q) = %v, want %v", test.address, got, test.want)
			}
		})
	}
}

func TestVersionCommandIdentity(t *testing.T) {
	var output bytes.Buffer
	if err := versionCommand(nil, &output); err != nil {
		t.Fatal(err)
	}
	if got, want := output.String(), "Kino "+buildinfo.Version+"\n"; got != want {
		t.Fatalf("plain version = %q, want %q", got, want)
	}

	output.Reset()
	if err := versionCommand([]string{"--json"}, &output); err != nil {
		t.Fatal(err)
	}
	var identity struct {
		Format             string `json:"format"`
		ApplicationVersion string `json:"application_version"`
	}
	if err := json.Unmarshal(output.Bytes(), &identity); err != nil {
		t.Fatalf("decode version identity: %v", err)
	}
	if identity.Format != "kino-version-v1" || identity.ApplicationVersion != buildinfo.Version {
		t.Fatalf("version identity = %#v", identity)
	}
	if err := versionCommand([]string{"unexpected"}, &output); err == nil {
		t.Fatal("version accepted a positional argument")
	}
}

func TestMetadataOnlyExportsRequireExplicitHostPathConsent(t *testing.T) {
	for name, run := range map[string]func([]string) error{
		"pegasus": exportPegasus,
		"es-de":   exportESDE,
	} {
		t.Run(name, func(t *testing.T) {
			err := run([]string{"--db", filepath.Join(t.TempDir(), "missing.db"), "--out", filepath.Join(t.TempDir(), "out")})
			if err == nil || !strings.Contains(err.Error(), "--allow-host-paths") {
				t.Fatalf("metadata-only export was not safely rejected: %v", err)
			}
		})
	}
}

func TestResolveMetadataSourceAcceptsLibraryRelativeAndPreservesExplicitPaths(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "gba"), 0o755); err != nil {
		t.Fatal(err)
	}
	libraryMetadata := filepath.Join(root, "gba", "metadata.pegasus.txt")
	if err := os.WriteFile(libraryMetadata, []byte("game: fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := resolveMetadataSource(root, "gba/metadata.pegasus.txt"); got != libraryMetadata {
		t.Fatalf("library-relative metadata = %q, want %q", got, libraryMetadata)
	}
	explicit := filepath.Join(t.TempDir(), "external.xml")
	if err := os.WriteFile(explicit, []byte("<gameList/>"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := resolveMetadataSource(root, explicit); got != explicit {
		t.Fatalf("explicit metadata = %q, want %q", got, explicit)
	}
}

func TestSanitizeCommandErrorRemovesPrivateArgumentsAndDerivedPaths(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private-library")
	token := "owner-token-that-must-not-appear"
	server := "https://private-device.example.test"
	t.Setenv("GAME_LIBRARY_TOKEN", token)
	source := errors.New("open " + filepath.Join(root, "gba", "private-game.gba") + " through " + server + " using " + token)
	got := sanitizeCommandError(source, []string{"--library", root, "--server=" + server})
	for _, private := range []string{root, "private-game.gba", server, token} {
		if strings.Contains(got.Error(), private) {
			t.Fatalf("sanitized error retained private value %q: %v", private, got)
		}
	}
	if !strings.Contains(got.Error(), "<redacted>") {
		t.Fatalf("sanitized error lost its useful placeholder: %v", got)
	}
}

func TestCompleteStateBackupCommandsRoundTripToNewRoot(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state")
	if err := os.Mkdir(state, 0o700); err != nil {
		t.Fatal(err)
	}
	database := filepath.Join(state, "library.db")
	store, err := catalog.Open(database)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreateGame(t.Context(), catalog.NewGame{DefaultTitle: "CLI fixture", Platform: "gba"}); err != nil {
		store.Close()
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	backup := filepath.Join(root, "backup")
	if err = backupState([]string{"--db", database, "--state", state, "--out", backup}); err != nil {
		t.Fatal(err)
	}
	if err = checkState([]string{"--from", backup}); err != nil {
		t.Fatal(err)
	}
	restored := filepath.Join(root, "restored")
	if err = restoreState([]string{"--from", backup, "--out", restored}); err != nil {
		t.Fatal(err)
	}
	if err = dbCheck([]string{"--db", filepath.Join(restored, "library.db")}); err != nil {
		t.Fatal(err)
	}
}

func TestDatabaseCheckIsReadOnlyAndDoesNotMigrate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "older.db")
	store, err := catalog.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`PRAGMA user_version = 15`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	if err = dbCheck([]string{"--db", path}); err == nil || !strings.Contains(err.Error(), "schema 15") {
		t.Fatalf("older schema was not safely rejected: %v", err)
	}
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	var version int
	if err = db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		db.Close()
		t.Fatal(err)
	}
	db.Close()
	if version != 15 {
		t.Fatalf("db-check migrated schema to %d", version)
	}
}
