package server

import (
	"bytes"
	"testing"

	"kino/internal/buildinfo"
)

func TestEmbeddedAssetsUseRuntimeVersion(t *testing.T) {
	index, err := webFiles.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, asset := range []string{"styles.css", "theme.js", "app.js"} {
		want := []byte(asset + "?v=" + buildinfo.Version)
		if !bytes.Contains(index, want) {
			t.Errorf("embedded index is missing versioned asset %q", want)
		}
	}
	for _, asset := range []string{"favicon.svg", "kino-logo.svg", "kino-mark.svg"} {
		if _, err := webFiles.ReadFile("web/assets/" + asset); err != nil {
			t.Errorf("embedded brand asset %q is missing: %v", asset, err)
		}
	}
	if !bytes.Contains(index, []byte("<title>Kino · ROM Library</title>")) {
		t.Error("embedded index is missing the Kino product title")
	}
	if bytes.Contains(index, []byte("私人游戏库")) {
		t.Error("embedded index still contains the retired brand subtitle")
	}
	if !bytes.Contains(openAPI, []byte("version: "+buildinfo.Version)) {
		t.Errorf("OpenAPI info.version does not match runtime %q", buildinfo.Version)
	}
}
