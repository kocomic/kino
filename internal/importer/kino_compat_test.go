package importer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestKinoReadsLegacyExportAndPrefersNewFields(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "game.gba"), []byte("synthetic fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	metadata := filepath.Join(root, "metadata.pegasus.txt")
	for _, explicit := range []bool{false, true} {
		content := "game: Example\nfile: game.gba\nx-varkiv-game-id: retained-game\nx-varkiv-edition-id: retained-edition\nx-varkiv-game-title: Old title\nx-varkiv-game-title-ja: 日本語\n"
		if explicit {
			content += "x-kino-game-title: Kino title\n"
		}
		if err := os.WriteFile(metadata, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		games, err := PreviewPegasus(root, metadata, "gba", "en")
		if err != nil || len(games) != 1 {
			t.Fatalf("games=%v err=%v", games, err)
		}
		game := games[0]
		want := "Old title"
		if explicit {
			want = "Kino title"
		}
		if game.GameID != "retained-game" || game.EditionID != "retained-edition" || game.DefaultTitle != want || game.Titles["ja"] != "日本語" {
			t.Fatalf("identity or titles changed: %#v", game)
		}
	}
	old := filepath.Join(root, "varkiv-launches.json")
	if err := os.WriteFile(old, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := findLaunchManifest(root, metadata); err != nil || got != old {
		t.Fatalf("legacy sidecar=%q err=%v", got, err)
	}
	current := filepath.Join(root, launchManifestName)
	if err := os.WriteFile(current, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := findLaunchManifest(root, metadata); err != nil || got != current {
		t.Fatalf("new sidecar=%q err=%v", got, err)
	}
}
