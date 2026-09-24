package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	"slices"
	"strings"
	"testing"

	"kino/internal/catalog"
	"kino/internal/platforms"
)

func TestPreviewTokensAreOpaqueDeterministicAndDomainSeparated(t *testing.T) {
	server := &Server{}
	payload := struct {
		RawCommand string `json:"raw_command"`
		SourceRef  string `json:"source_ref"`
	}{RawCommand: `retroarch.exe -L private_core.dll "private game.zip"`, SourceRef: "private/source/metadata.pegasus.txt"}

	first, err := server.signPreviewValue(previewTokenDomainRuntimeHintBatch, payload)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := server.signPreviewValue(previewTokenDomainRuntimeHintBatch, payload)
	if err != nil || repeated != first {
		t.Fatalf("repeated token = %q, %v", repeated, err)
	}
	otherDomain, err := server.signPreviewValue(previewTokenDomainImport, payload)
	if err != nil || otherDomain == first {
		t.Fatalf("domain-separated token = %q, %v", otherDomain, err)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(first)
	if err != nil || len(decoded) != sha256.Size {
		t.Fatalf("opaque token bytes = %d, %v", len(decoded), err)
	}
	if strings.Contains(first, payload.RawCommand) || strings.Contains(first, payload.SourceRef) {
		t.Fatalf("token exposed signed payload: %q", first)
	}
	if _, err = server.signPreviewValue("", payload); err == nil {
		t.Fatal("empty token domain was accepted")
	}
}

func testServer(t *testing.T) (*catalog.Store, http.Handler, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "library")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := catalog.Open(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	app, err := New(store, root)
	if err != nil {
		t.Fatal(err)
	}
	return store, app.Handler(), root
}

func jsonRequest(t *testing.T, handler http.Handler, method, path string, input any, output any) int {
	t.Helper()
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		body = bytes.NewReader(data)
	}
	req := httptest.NewRequest(method, path, body)
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if recorder.Code >= 400 {
		t.Fatalf("%s %s: %d %s", method, path, recorder.Code, recorder.Body.String())
	}
	if output != nil && recorder.Code != http.StatusNoContent {
		if err := json.Unmarshal(recorder.Body.Bytes(), output); err != nil {
			t.Fatal(err)
		}
	}
	return recorder.Code
}

func jsonErrorRequest(t *testing.T, handler http.Handler, method, path string, input any) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, bytes.NewReader(data))
	request.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(recorder, request)
	return recorder
}

func testSaveSetHash(logicalPath, content string) string {
	fileDigest := sha256.Sum256([]byte(content))
	setDigest := sha256.New()
	setDigest.Write([]byte(logicalPath))
	setDigest.Write([]byte{0})
	setDigest.Write([]byte(hex.EncodeToString(fileDigest[:])))
	setDigest.Write([]byte{0})
	setDigest.Write([]byte(fmt.Sprint(len(content))))
	setDigest.Write([]byte{0})
	return hex.EncodeToString(setDigest.Sum(nil))
}

func TestLibraryMaintenanceAPI(t *testing.T) {
	_, handler, _ := testServer(t)
	var presets []map[string]any
	jsonRequest(t, handler, http.MethodGet, "/api/platforms", nil, &presets)
	if len(presets) != 72 {
		t.Fatalf("platform presets missing: %d", len(presets))
	}
	var ngpcSuggestions map[string]any
	for _, preset := range presets {
		if preset["id"] == "ngpc" {
			ngpcSuggestions, _ = preset["suggested_emulators"].(map[string]any)
			break
		}
	}
	if windows, ok := ngpcSuggestions["windows"].([]any); !ok || len(windows) != 1 || windows[0] != "RetroArch · Beetle NeoPop" {
		t.Fatalf("NGPC platform suggestions missing from API: %#v", ngpcSuggestions)
	}
	var switchSuggestions map[string]any
	for _, preset := range presets {
		if preset["id"] == "switch" {
			switchSuggestions, _ = preset["suggested_emulators"].(map[string]any)
			break
		}
	}
	if windows, ok := switchSuggestions["windows"].([]any); !ok || len(windows) != 1 || windows[0] != "Eden" {
		t.Fatalf("Switch Windows suggestion missing from API: %#v", switchSuggestions)
	}
	if android, ok := switchSuggestions["android"].([]any); !ok || len(android) != 0 {
		t.Fatalf("Switch Android suggestion must remain empty: %#v", switchSuggestions)
	}
	var alias map[string]any
	jsonRequest(t, handler, http.MethodGet, "/api/platforms/ps1", nil, &alias)
	if alias["id"] != "psx" {
		t.Fatalf("platform alias was not resolved: %#v", alias)
	}
	var first, second catalog.Game
	jsonRequest(t, handler, http.MethodPost, "/api/games", catalog.NewGame{DefaultTitle: "Game", Platform: "ps1", Titles: map[string]string{"zh-CN": "游戏"}}, &first)
	if first.Platform != "psx" {
		t.Fatalf("game platform was not canonicalized: %#v", first)
	}
	jsonRequest(t, handler, http.MethodPost, "/api/games", catalog.NewGame{DefaultTitle: "Translation", Platform: "psx"}, &second)
	var created catalog.Game
	jsonRequest(t, handler, http.MethodPost, "/api/editions", editionRequest{NewEdition: catalog.NewEdition{GameID: second.ID, DefaultTitle: "Chinese", EditionType: "translation", Languages: []string{"zh-CN"}}}, &created)
	if len(created.Editions) != 1 {
		t.Fatalf("edition not created: %#v", created)
	}
	editionID := created.Editions[0].ID
	var moved catalog.Edition
	jsonRequest(t, handler, http.MethodPost, "/api/editions/"+editionID+"/move", catalog.MoveEdition{TargetGameID: first.ID}, &moved)
	if moved.GameID != first.ID {
		t.Fatalf("edition not moved: %#v", moved)
	}
	var merged catalog.Game
	jsonRequest(t, handler, http.MethodPost, "/api/games/"+first.ID+"/merge", catalog.MergeGames{SourceGameID: second.ID}, &merged)
	if len(merged.Editions) != 1 {
		t.Fatalf("merge lost edition: %#v", merged)
	}
	jsonRequest(t, handler, http.MethodPut, "/api/games/"+first.ID+"/primary", map[string]string{"edition_id": editionID}, &merged)
	if merged.PrimaryEditionID != editionID {
		t.Fatalf("primary edition not set: %#v", merged)
	}
}

func TestSignedGameMergePreviewRejectsTamperingAndDrift(t *testing.T) {
	store, handler, _ := testServer(t)
	var target, source catalog.Game
	jsonRequest(t, handler, http.MethodPost, "/api/v1/games", catalog.NewGame{DefaultTitle: "Target", Platform: "gba"}, &target)
	jsonRequest(t, handler, http.MethodPost, "/api/v1/games", catalog.NewGame{DefaultTitle: "Source", Platform: "gba"}, &source)
	invalid := jsonErrorRequest(t, handler, http.MethodPost, "/api/v1/games/"+target.ID+"/merge/preview", map[string]string{})
	if invalid.Code != http.StatusBadRequest || !strings.Contains(invalid.Body.String(), "invalid_argument") {
		t.Fatalf("invalid merge preview = %d %s", invalid.Code, invalid.Body.String())
	}
	var targetWithEdition, sourceWithEdition catalog.Game
	jsonRequest(t, handler, http.MethodPost, "/api/v1/editions", editionRequest{NewEdition: catalog.NewEdition{GameID: target.ID, DefaultTitle: "Original", EditionType: "original"}}, &targetWithEdition)
	jsonRequest(t, handler, http.MethodPost, "/api/v1/editions", editionRequest{NewEdition: catalog.NewEdition{GameID: source.ID, DefaultTitle: "Translation", EditionType: "translation"}}, &sourceWithEdition)
	oldSourceEdition := sourceWithEdition.Editions[0]

	var preview gameMergePreviewResponse
	jsonRequest(t, handler, http.MethodPost, "/api/v1/games/"+target.ID+"/merge/preview?locale=zh-CN", catalog.MergeGames{SourceGameID: source.ID}, &preview)
	if preview.PreviewToken == "" || preview.SnapshotFingerprint == "" || preview.TargetEditions != 1 || preview.SourceEditions != 1 || preview.ResultEditions != 2 || preview.FailurePolicy != "atomic" || preview.ROMFilesMoved || preview.SaveNamespacesChanged || !preview.SourceGameMetadataRemoved {
		t.Fatalf("unexpected signed preview: %#v", preview)
	}
	if strings.Contains(preview.PreviewToken, target.ID) || strings.Contains(preview.PreviewToken, source.ID) {
		t.Fatal("opaque merge token exposed catalog identity")
	}
	tampered := jsonErrorRequest(t, handler, http.MethodPost, "/api/v1/games/"+target.ID+"/merge", catalog.MergeGames{SourceGameID: source.ID, PreviewToken: preview.PreviewToken + "tampered", SnapshotFingerprint: preview.SnapshotFingerprint})
	if tampered.Code != http.StatusConflict || !strings.Contains(tampered.Body.String(), "game_merge_stale") {
		t.Fatalf("tampered merge = %d %s", tampered.Code, tampered.Body.String())
	}
	if got, err := store.GetGame(context.Background(), source.ID, ""); err != nil || len(got.Editions) != 1 {
		t.Fatalf("tampered merge wrote catalog state: %#v, %v", got, err)
	}

	driftEdition, err := store.AddEdition(context.Background(), catalog.NewEdition{GameID: source.ID, DefaultTitle: "Hack", EditionType: "hack"})
	if err != nil {
		t.Fatal(err)
	}
	stale := jsonErrorRequest(t, handler, http.MethodPost, "/api/v1/games/"+target.ID+"/merge", catalog.MergeGames{SourceGameID: source.ID, PreviewToken: preview.PreviewToken, SnapshotFingerprint: preview.SnapshotFingerprint})
	if stale.Code != http.StatusConflict || !strings.Contains(stale.Body.String(), "game_merge_stale") {
		t.Fatalf("stale merge = %d %s", stale.Code, stale.Body.String())
	}
	if got, err := store.GetGame(context.Background(), target.ID, ""); err != nil || len(got.Editions) != 1 {
		t.Fatalf("stale merge changed target: %#v, %v", got, err)
	}

	jsonRequest(t, handler, http.MethodPost, "/api/v1/games/"+target.ID+"/merge/preview", catalog.MergeGames{SourceGameID: source.ID}, &preview)
	var merged catalog.Game
	jsonRequest(t, handler, http.MethodPost, "/api/v1/games/"+target.ID+"/merge", catalog.MergeGames{SourceGameID: source.ID, PreviewToken: preview.PreviewToken, SnapshotFingerprint: preview.SnapshotFingerprint}, &merged)
	if len(merged.Editions) != 3 {
		t.Fatalf("signed merge lost editions: %#v", merged)
	}
	byID := map[string]catalog.Edition{}
	for _, edition := range merged.Editions {
		byID[edition.ID] = edition
	}
	if byID[oldSourceEdition.ID].SaveNamespace != oldSourceEdition.SaveNamespace || byID[driftEdition.ID].SaveNamespace != driftEdition.SaveNamespace {
		t.Fatal("signed merge changed edition identity or save namespace")
	}
	if _, err = store.GetGame(context.Background(), source.ID, ""); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("source survived signed merge: %v", err)
	}
}

func TestCustomPlatformAPIIsMergedAndCanonicalizesAliases(t *testing.T) {
	_, handler, _ := testServer(t)
	enabled := true
	input := catalog.NewCustomPlatform{ID: "fixture-handheld", Name: "Fixture Handheld", NameZH: "测试掌机", Category: "handheld", Aliases: []string{"fixture-hh"}, Extensions: []string{".opk"}, ESDESystems: []string{"fixture-handheld-es"}, Enabled: &enabled}
	var created catalog.CustomPlatform
	if code := jsonRequest(t, handler, http.MethodPost, "/api/v1/custom-platforms", input, &created); code != http.StatusCreated || created.ID != "fixture-handheld" || created.Builtin {
		t.Fatalf("created=%#v code=%d", created, code)
	}
	var resolved platforms.Platform
	jsonRequest(t, handler, http.MethodGet, "/api/v1/platforms/fixture-handheld-es", nil, &resolved)
	if resolved.ID != "fixture-handheld" || resolved.Builtin || len(resolved.Extensions) != 1 {
		t.Fatalf("resolved=%#v", resolved)
	}
	conflict := input
	conflict.ID, conflict.Name, conflict.Aliases = "other-hand", "Other Hand", []string{"gba"}
	conflictResponse := jsonErrorRequest(t, handler, http.MethodPost, "/api/v1/custom-platforms", conflict)
	if conflictResponse.Code != http.StatusConflict || !strings.Contains(conflictResponse.Body.String(), "platform_key_conflict") {
		t.Fatalf("platform collision: %d %s", conflictResponse.Code, conflictResponse.Body.String())
	}
	var game catalog.Game
	jsonRequest(t, handler, http.MethodPost, "/api/v1/games", catalog.NewGame{DefaultTitle: "Custom", Platform: "fixture-hh"}, &game)
	if game.Platform != "fixture-handheld" {
		t.Fatalf("custom alias was not canonicalized: %#v", game)
	}
	disabled := false
	input.Enabled = &disabled
	var updated catalog.CustomPlatform
	jsonRequest(t, handler, http.MethodPut, "/api/v1/custom-platforms/fixture-handheld", input, &updated)
	if updated.Enabled {
		t.Fatalf("updated=%#v", updated)
	}
	var active []platforms.Platform
	jsonRequest(t, handler, http.MethodGet, "/api/platforms", nil, &active)
	for _, item := range active {
		if item.ID == "fixture-handheld" {
			t.Fatal("disabled custom platform remained in active platform collection")
		}
	}
	response := jsonErrorRequest(t, handler, http.MethodDelete, "/api/v1/custom-platforms/fixture-handheld", nil)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "custom_platform_in_use") {
		t.Fatalf("delete referenced custom platform: %d %s", response.Code, response.Body.String())
	}
}

// Treat every built-in target package as a portable recovery artifact, not
// merely a renderer output. A fresh catalog must require explicit review
// before recreating any launch binding, resolve the same target-specific
// contract afterwards, and reproduce the launch sidecar byte for byte.

func TestCrossPlatformSeriesAPI(t *testing.T) {
	_, handler, _ := testServer(t)
	var gba, nds catalog.Game
	jsonRequest(t, handler, http.MethodPost, "/api/v1/games", catalog.NewGame{DefaultTitle: "Game Advance", Platform: "gba"}, &gba)
	jsonRequest(t, handler, http.MethodPost, "/api/v1/games", catalog.NewGame{DefaultTitle: "Game DS", Platform: "nds"}, &nds)
	var series catalog.Series
	status := jsonRequest(t, handler, http.MethodPost, "/api/v1/series", catalog.NewSeries{DefaultTitle: "Game Saga", Titles: map[string]string{"zh-CN": "游戏传奇"}}, &series)
	if status != http.StatusCreated || series.ID == "" {
		t.Fatalf("series not created: %#v", series)
	}
	jsonRequest(t, handler, http.MethodPut, "/api/v1/series/"+series.ID+"/members/"+gba.ID, catalog.NewSeriesMember{RelationType: "mainline", SortOrder: 10}, &series)
	jsonRequest(t, handler, http.MethodPut, "/api/v1/series/"+series.ID+"/members/"+nds.ID, catalog.NewSeriesMember{RelationType: "port", SortOrder: 20}, &series)
	if len(series.Members) != 2 || series.Members[0].Game.Platform != "gba" || series.Members[1].Game.Platform != "nds" {
		t.Fatalf("cross-platform members missing: %#v", series)
	}
	var localized catalog.Series
	jsonRequest(t, handler, http.MethodGet, "/api/v1/series/"+series.ID+"?locale=zh-CN", nil, &localized)
	if localized.DisplayTitle != "游戏传奇" {
		t.Fatalf("localized title missing: %#v", localized)
	}
	var list struct {
		Data []catalog.Series `json:"data"`
	}
	jsonRequest(t, handler, http.MethodGet, "/api/v1/series?q=nds&locale=zh-CN", nil, &list)
	if len(list.Data) != 1 {
		t.Fatalf("series member search failed: %#v", list.Data)
	}
	if status = jsonRequest(t, handler, http.MethodDelete, "/api/v1/series/"+series.ID+"/members/"+nds.ID, nil, nil); status != http.StatusNoContent {
		t.Fatalf("member delete status = %d", status)
	}
}

func TestSeriesMutationAPIIsAtomic(t *testing.T) {
	_, handler, _ := testServer(t)
	var gba, nds catalog.Game
	jsonRequest(t, handler, http.MethodPost, "/api/v1/games", catalog.NewGame{DefaultTitle: "Atomic GBA", Platform: "gba"}, &gba)
	jsonRequest(t, handler, http.MethodPost, "/api/v1/games", catalog.NewGame{DefaultTitle: "Atomic NDS", Platform: "nds"}, &nds)
	members := []catalog.SeriesMemberMutation{{GameID: gba.ID, RelationType: "mainline", SortOrder: 10}, {GameID: nds.ID, RelationType: "port", SortOrder: 20}}
	var series catalog.Series
	status := jsonRequest(t, handler, http.MethodPost, "/api/v1/series", catalog.SeriesMutation{DefaultTitle: "Atomic Series", Description: "before", Titles: map[string]string{"en": "Atomic Series"}, Members: &members}, &series)
	if status != http.StatusCreated || len(series.Members) != 2 {
		t.Fatalf("atomic create = status %d, %#v", status, series)
	}

	badMembers := []catalog.SeriesMemberMutation{{GameID: gba.ID, RelationType: "remake", SortOrder: 1}, {GameID: nds.ID, RelationType: "port", SortOrder: -1}}
	bad := jsonErrorRequest(t, handler, http.MethodPut, "/api/v1/series/"+series.ID, catalog.SeriesMutation{DefaultTitle: "Must Not Persist", Description: "after", Members: &badMembers})
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("negative member status=%d body=%s", bad.Code, bad.Body.String())
	}
	var unchanged catalog.Series
	jsonRequest(t, handler, http.MethodGet, "/api/v1/series/"+series.ID, nil, &unchanged)
	if unchanged.DefaultTitle != "Atomic Series" || unchanged.Description != "before" || len(unchanged.Members) != 2 || unchanged.Members[0].RelationType != "mainline" || unchanged.Members[1].SortOrder != 20 {
		t.Fatalf("failed request partially changed series: %#v", unchanged)
	}

	replacement := []catalog.SeriesMemberMutation{{GameID: nds.ID, RelationType: "remake", SortOrder: 3}}
	jsonRequest(t, handler, http.MethodPut, "/api/v1/series/"+series.ID, catalog.SeriesMutation{DefaultTitle: "Atomic Series II", Description: "after", Members: &replacement}, &series)
	if series.DefaultTitle != "Atomic Series II" || len(series.Members) != 1 || series.Members[0].GameID != nds.ID || series.Members[0].SortOrder != 3 {
		t.Fatalf("atomic replacement = %#v", series)
	}
}

func TestPersistentROMSourceScanCommitAndNonDestructiveDelete(t *testing.T) {
	_, handler, root := testServer(t)
	romDir := filepath.Join(root, "gba")
	if err := os.MkdirAll(romDir, 0o755); err != nil {
		t.Fatal(err)
	}
	romPath := filepath.Join(romDir, "Advance Wars.gba")
	if err := os.WriteFile(romPath, []byte("small-fixture-rom"), 0o644); err != nil {
		t.Fatal(err)
	}
	var source catalog.LibrarySource
	status := jsonRequest(t, handler, http.MethodPost, "/api/v1/sources", catalog.NewLibrarySource{Name: "GBA on NAS", Kind: "rom_directory", RootPath: "gba", Platform: "gba", ROMStoragePolicy: "reference", MediaStoragePolicy: "ignore"}, &source)
	if status != http.StatusCreated || source.RootPath != "gba" || !source.Enabled || source.SourceAdapterID != "builtin-source-direct-rom" {
		t.Fatalf("source create = %d %#v", status, source)
	}
	var preview struct {
		Scan         catalog.SourceScan `json:"scan"`
		PreviewToken string             `json:"preview_token"`
		Candidates   []importCandidate  `json:"candidates"`
	}
	jsonRequest(t, handler, http.MethodPost, "/api/v1/sources/"+source.ID+"/scans", map[string]any{}, &preview)
	if preview.Scan.Status != "ready" || preview.Scan.ImportableCount != 1 || len(preview.Candidates) != 1 || preview.PreviewToken == "" {
		t.Fatalf("source preview = %#v", preview)
	}
	var committed map[string]any
	jsonRequest(t, handler, http.MethodPost, "/api/v1/source-scans/"+preview.Scan.ID+"/commit", sourceScanCommitRequest{PreviewToken: preview.PreviewToken, SelectedTokens: []string{preview.Candidates[0].Token}}, &committed)
	if committed["imported"] != float64(1) {
		t.Fatalf("source commit = %#v", committed)
	}
	deleteResponse := httptest.NewRecorder()
	handler.ServeHTTP(deleteResponse, httptest.NewRequest(http.MethodDelete, "/api/v1/sources/"+source.ID, nil))
	if deleteResponse.Code != http.StatusConflict {
		t.Fatalf("source history deletion status = %d %s", deleteResponse.Code, deleteResponse.Body.String())
	}
	if data, err := os.ReadFile(romPath); err != nil || string(data) != "small-fixture-rom" {
		t.Fatalf("source file changed after metadata deletion attempt: %q, %v", data, err)
	}
	var scans struct {
		Data []catalog.SourceScan `json:"data"`
	}
	jsonRequest(t, handler, http.MethodGet, "/api/v1/source-scans?source_id="+source.ID, nil, &scans)
	if len(scans.Data) != 1 || scans.Data[0].Status != "committed" {
		t.Fatalf("scan audit history = %#v", scans.Data)
	}
}

func TestPersistentNeutralManifestSourceKeepsPerEntryPlatforms(t *testing.T) {
	_, handler, root := testServer(t)
	if err := os.MkdirAll(filepath.Join(root, "recovery", "roms"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "recovery", "roms", "fixture.gba"), []byte("neutral-persistent-rom"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := map[string]any{
		"format_version": 4,
		"entries": []map[string]any{{
			"game_id": "neutral-source-game", "edition_id": "neutral-source-edition", "platform": "gba",
			"game_default_title": "Neutral source game", "edition_default_title": "Original", "edition_type": "original",
			"artifacts": []string{"roms/fixture.gba"}, "media": []any{},
		}},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "recovery", "library-manifest.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	var source catalog.LibrarySource
	status := jsonRequest(t, handler, http.MethodPost, "/api/v1/sources", catalog.NewLibrarySource{Name: "Recovery manifest", Kind: "kino", MetadataPath: "recovery/library-manifest.json", ROMStoragePolicy: "reference", MediaStoragePolicy: "copy"}, &source)
	if status != http.StatusCreated || source.Kind != "kino" || source.Platform != "" || source.RootPath != "recovery" {
		t.Fatalf("neutral source create = %d %#v", status, source)
	}
	var preview struct {
		Scan         catalog.SourceScan `json:"scan"`
		PreviewToken string             `json:"preview_token"`
		Candidates   []importCandidate  `json:"candidates"`
	}
	jsonRequest(t, handler, http.MethodPost, "/api/v1/sources/"+source.ID+"/scans", map[string]any{}, &preview)
	if preview.Scan.ImportableCount != 1 || len(preview.Candidates) != 1 || preview.Candidates[0].Game.Platform != "gba" {
		t.Fatalf("neutral source preview = %#v", preview)
	}
	var committed map[string]any
	jsonRequest(t, handler, http.MethodPost, "/api/v1/source-scans/"+preview.Scan.ID+"/commit", sourceScanCommitRequest{PreviewToken: preview.PreviewToken, SelectedTokens: []string{preview.Candidates[0].Token}}, &committed)
	if committed["parsed"] != float64(1) || committed["imported"] != float64(1) || committed["skipped"] != float64(0) {
		t.Fatalf("neutral source commit = %#v", committed)
	}
}

func TestNeutralManifestV6PreviewBindsAndAtomicallyImportsCustomPlatform(t *testing.T) {
	store, handler, root := testServer(t)
	ctx := context.Background()
	recovery := filepath.Join(root, "portable-v6")
	romBody := []byte("portable-v6-custom-platform-rom")
	if err := os.MkdirAll(filepath.Join(recovery, "roms"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(recovery, "roms", "fixture.opk"), romBody, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(romBody)
	manifestPath := filepath.Join(recovery, "library-manifest.json")
	writeManifest := func(nameZH string) {
		t.Helper()
		manifest := map[string]any{
			"format_version": 6,
			"custom_platforms": []map[string]any{{
				"id": "fixture-handheld-api", "name": "Fixture Handheld API", "name_zh": nameZH, "vendor": "Community", "category": "handheld",
				"aliases": []string{"oh-api"}, "extensions": []string{".opk"}, "esde_systems": []string{"fixture-handheld-api-es"}, "bios": "none", "runtime": "native",
			}},
			"entries": []map[string]any{{
				"game_id": "portable-v6-game", "edition_id": "portable-v6-edition", "platform": "fixture-handheld-api",
				"game_default_title": "Portable v6 game", "edition_default_title": "Original", "edition_type": "original",
				"artifacts":        []string{"roms/fixture.opk"},
				"artifact_records": []map[string]any{{"path": "roms/fixture.opk", "role": "rom", "original_name": "fixture.opk", "size": len(romBody), "sha256": hex.EncodeToString(digest[:])}},
			}},
		}
		data, err := json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(manifestPath, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeManifest("便携平台")
	request := importRequest{Format: "kino", Source: "portable-v6/library-manifest.json", ROMStorage: "reference", MediaStorage: "ignore"}
	var preview struct {
		PreviewToken string            `json:"preview_token"`
		Candidates   []importCandidate `json:"candidates"`
	}
	jsonRequest(t, handler, http.MethodPost, "/api/v1/imports/preview", request, &preview)
	if len(preview.Candidates) != 1 || preview.Candidates[0].Game.PlatformDefinition == nil || preview.Candidates[0].Game.PlatformDefinition.ID != "fixture-handheld-api" {
		t.Fatalf("portable platform is absent from signed preview: %#v", preview)
	}
	request.PreviewToken = preview.PreviewToken
	request.SelectedTokens = []string{preview.Candidates[0].Token}
	writeManifest("便携平台新定义")
	stale := jsonErrorRequest(t, handler, http.MethodPost, "/api/v1/imports/commit", request)
	if stale.Code != http.StatusConflict || !strings.Contains(stale.Body.String(), "import_preview_stale") {
		t.Fatalf("changed portable definition was not bound to preview: %d %s", stale.Code, stale.Body.String())
	}
	if _, err := store.GetCustomPlatform(ctx, "fixture-handheld-api"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("stale preview wrote custom platform: %v", err)
	}

	jsonRequest(t, handler, http.MethodPost, "/api/v1/imports/preview", importRequest{Format: "kino", Source: "portable-v6/library-manifest.json", ROMStorage: "reference", MediaStorage: "ignore"}, &preview)
	request.PreviewToken = preview.PreviewToken
	request.SelectedTokens = []string{preview.Candidates[0].Token}
	var committed map[string]any
	jsonRequest(t, handler, http.MethodPost, "/api/v1/imports/commit", request, &committed)
	if committed["imported"] != float64(1) || committed["failure_policy"] != "atomic" {
		t.Fatalf("v6 commit = %#v", committed)
	}
	platform, err := store.GetCustomPlatform(ctx, "fixture-handheld-api")
	if err != nil || platform.NameZH != "便携平台新定义" || !platform.Enabled {
		t.Fatalf("imported platform = %#v, %v", platform, err)
	}
	game, err := store.GetGame(ctx, "portable-v6-game", "zh-CN")
	if err != nil || game.Platform != "fixture-handheld-api" {
		t.Fatalf("imported game = %#v, %v", game, err)
	}

	writeManifest("冲突定义")
	conflict := jsonErrorRequest(t, handler, http.MethodPost, "/api/v1/imports/preview", importRequest{Format: "kino", Source: "portable-v6/library-manifest.json", ROMStorage: "reference", MediaStorage: "ignore"})
	if conflict.Code != http.StatusConflict || !strings.Contains(conflict.Body.String(), "platform_definition_conflict") {
		t.Fatalf("conflicting portable definition = %d %s", conflict.Code, conflict.Body.String())
	}
}

func TestLibrarySourceRejectsPathsOutsideLibraryRoot(t *testing.T) {
	_, handler, _ := testServer(t)
	outside := filepath.Join(t.TempDir(), "private.gba")
	if err := os.WriteFile(outside, []byte("do-not-read"), 0o600); err != nil {
		t.Fatal(err)
	}
	response := jsonErrorRequest(t, handler, http.MethodPost, "/api/v1/sources", catalog.NewLibrarySource{Name: "Outside", Kind: "rom_directory", RootPath: outside, Platform: "gba"})
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "inside library") {
		t.Fatalf("outside path was not rejected: %d %s", response.Code, response.Body.String())
	}
}

func TestPersistentSourceRejectsTokenFromAnotherPreview(t *testing.T) {
	_, handler, root := testServer(t)
	if err := os.MkdirAll(filepath.Join(root, "gba"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "gba", "token.gba"), []byte("token-fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	var source catalog.LibrarySource
	jsonRequest(t, handler, http.MethodPost, "/api/v1/sources", catalog.NewLibrarySource{Name: "Token source", Kind: "rom_directory", RootPath: "gba", Platform: "gba"}, &source)
	var preview struct {
		Scan       catalog.SourceScan `json:"scan"`
		Candidates []importCandidate  `json:"candidates"`
	}
	jsonRequest(t, handler, http.MethodPost, "/api/v1/sources/"+source.ID+"/scans", map[string]any{}, &preview)
	response := jsonErrorRequest(t, handler, http.MethodPost, "/api/v1/source-scans/"+preview.Scan.ID+"/commit", sourceScanCommitRequest{PreviewToken: "not-this-preview", SelectedTokens: []string{preview.Candidates[0].Token}})
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "import_preview_stale") {
		t.Fatalf("mismatched source token = %d %s", response.Code, response.Body.String())
	}
	var stale catalog.SourceScan
	jsonRequest(t, handler, http.MethodGet, "/api/v1/source-scans/"+preview.Scan.ID, nil, &stale)
	if stale.Status != "stale" || stale.FailureCode != "import_preview_stale" {
		t.Fatalf("stale source scan was not audited: %#v", stale)
	}
}

func TestMediaMetadataAPIUpdatesOnlyLibrarySemantics(t *testing.T) {
	store, handler, _ := testServer(t)
	ctx := context.Background()
	game, err := store.CreateGame(ctx, catalog.NewGame{DefaultTitle: "Media API", Platform: "gba"})
	if err != nil {
		t.Fatal(err)
	}
	media, err := store.AddMedia(ctx, catalog.NewMediaAsset{GameID: game.ID, Kind: "cover", StorageKind: "managed", Path: "media/aa/blob.png", OriginalName: "cover.png", MIMEType: "image/png", Size: 3, SHA256: strings.Repeat("b", 64), SourceType: "upload"})
	if err != nil {
		t.Fatal(err)
	}
	var updated catalog.MediaAsset
	jsonRequest(t, handler, http.MethodPut, "/api/v1/media/"+media.ID, catalog.MediaMetadataUpdate{Kind: "poster", Locale: "en", SortOrder: 4}, &updated)
	if updated.Kind != "poster" || updated.Locale != "en" || updated.SortOrder != 4 {
		t.Fatalf("media metadata response = %#v", updated)
	}
	if updated.GameID != media.GameID || updated.Path != media.Path || updated.SHA256 != media.SHA256 || updated.Size != media.Size || updated.StorageKind != media.StorageKind {
		t.Fatalf("media identity changed through API: before=%#v after=%#v", media, updated)
	}
	bad := jsonErrorRequest(t, handler, http.MethodPut, "/api/v1/media/"+media.ID, catalog.MediaMetadataUpdate{Kind: "cover", SortOrder: -1})
	if bad.Code != http.StatusBadRequest || !strings.Contains(bad.Body.String(), "invalid_argument") {
		t.Fatalf("negative media ordering = %d %s", bad.Code, bad.Body.String())
	}
	missing := jsonErrorRequest(t, handler, http.MethodPut, "/api/v1/media/00000000-0000-0000-0000-000000000000", catalog.MediaMetadataUpdate{Kind: "cover", SortOrder: 0})
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing media update = %d %s", missing.Code, missing.Body.String())
	}
}

func TestManualEditionWithMissingArtifactIsRejectedAtomically(t *testing.T) {
	_, handler, root := testServer(t)
	var game catalog.Game
	jsonRequest(t, handler, http.MethodPost, "/api/v1/games", catalog.NewGame{DefaultTitle: "Planned game", Platform: "gba"}, &game)
	body, err := json.Marshal(editionRequest{
		NewEdition:   catalog.NewEdition{GameID: game.ID, DefaultTitle: "Missing ROM", EditionType: "translation"},
		ArtifactPath: "gba/not-present.gba",
		ArtifactRole: "rom",
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/editions", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var failure apiErrorEnvelope
	if err = json.Unmarshal(response.Body.Bytes(), &failure); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusBadRequest || failure.Error.Code != "artifact_missing" || strings.Contains(response.Body.String(), root) {
		t.Fatalf("missing artifact was not rejected: %d %s", response.Code, response.Body.String())
	}
	var refreshed catalog.Game
	jsonRequest(t, handler, http.MethodGet, "/api/v1/games/"+game.ID, nil, &refreshed)
	if len(refreshed.Editions) != 0 || refreshed.PrimaryEditionID != "" {
		t.Fatalf("failed manual create left partial catalog records: %#v", refreshed)
	}
}

func TestArtifactValidationErrorsAreStableAndDoNotExposeHostPaths(t *testing.T) {
	_, handler, root := testServer(t)
	var game catalog.Game
	jsonRequest(t, handler, http.MethodPost, "/api/v1/games", catalog.NewGame{DefaultTitle: "Private path test", Platform: "ps2"}, &game)
	var withEdition catalog.Game
	jsonRequest(t, handler, http.MethodPost, "/api/v1/editions", editionRequest{NewEdition: catalog.NewEdition{GameID: game.ID, DefaultTitle: "Private path test", EditionType: "original"}}, &withEdition)
	editionID := withEdition.Editions[0].ID

	directory := filepath.Join(root, "ps2", "game")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "private-target"), filepath.Join(directory, "link")); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "private.iso"), []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "ps2", "linked-parent")); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		path string
		code string
		want int
	}{
		{name: "outside library", path: filepath.Join(filepath.Dir(root), "private-outside.rom"), code: "artifact_outside_library", want: http.StatusBadRequest},
		{name: "unhashable directory", path: "ps2/game", code: "artifact_unreadable", want: http.StatusUnprocessableEntity},
		{name: "symlinked parent", path: "ps2/linked-parent/private.iso", code: "artifact_unreadable", want: http.StatusUnprocessableEntity},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body, err := json.Marshal(catalog.NewArtifact{EditionID: editionID, Path: test.path, Role: "rom"})
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, "/api/v1/artifacts", bytes.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			var failure apiErrorEnvelope
			if err = json.Unmarshal(response.Body.Bytes(), &failure); err != nil {
				t.Fatal(err)
			}
			if response.Code != test.want || failure.Error.Code != test.code || strings.Contains(response.Body.String(), root) || strings.Contains(response.Body.String(), outside) {
				t.Fatalf("unexpected private-path error: %d %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestSeparateMetadataContentRootPersistsAndParticipatesInSignedPreview(t *testing.T) {
	store, handler, root := testServer(t)
	for _, directory := range []string{"metadata/gba", "roms/gba", "other/gba"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(directory)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "roms", "gba", "separate.gba"), []byte("selected-root-rom"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "other", "gba", "separate.gba"), []byte("other-root-rom"), 0o600); err != nil {
		t.Fatal(err)
	}
	metadata := "collection: GBA\n\ngame: Separate root\nfile: separate.gba\n"
	if err := os.WriteFile(filepath.Join(root, "metadata", "gba", "metadata.pegasus.txt"), []byte(metadata), 0o600); err != nil {
		t.Fatal(err)
	}
	request := importRequest{Format: "pegasus", Source: "metadata/gba/metadata.pegasus.txt", ContentRoot: "roms/gba", Platform: "gba", ROMStorage: "reference", MediaStorage: "ignore"}
	var preview struct {
		PreviewToken string            `json:"preview_token"`
		Candidates   []importCandidate `json:"candidates"`
	}
	jsonRequest(t, handler, http.MethodPost, "/api/v1/imports/preview", request, &preview)
	if len(preview.Candidates) != 1 || preview.Candidates[0].Status != "new" || preview.Candidates[0].Game.Artifacts[0].Path != "roms/gba/separate.gba" {
		t.Fatalf("separate root preview failed: %#v", preview)
	}
	request.PreviewToken = preview.PreviewToken
	request.SelectedTokens = []string{preview.Candidates[0].Token}
	request.ContentRoot = "other/gba"
	stale := jsonErrorRequest(t, handler, http.MethodPost, "/api/v1/imports/commit", request)
	if stale.Code != http.StatusConflict || !strings.Contains(stale.Body.String(), "import_preview_stale") {
		t.Fatalf("changed content root must invalidate preview: %d %s", stale.Code, stale.Body.String())
	}
	if games, err := store.ListGames(context.Background(), ""); err != nil || len(games) != 0 {
		t.Fatalf("stale root wrote catalog data: %#v %v", games, err)
	}

	var source catalog.LibrarySource
	jsonRequest(t, handler, http.MethodPost, "/api/v1/sources", catalog.NewLibrarySource{
		Name: "Separated Pegasus", Kind: "pegasus", RootPath: "roms/gba", MetadataPath: "metadata/gba/metadata.pegasus.txt", Platform: "gba", ROMStoragePolicy: "reference", MediaStoragePolicy: "ignore",
	}, &source)
	if source.RootPath != "roms/gba" || source.MetadataPath != "metadata/gba/metadata.pegasus.txt" {
		t.Fatalf("persistent source lost separate root: %#v", source)
	}
	var scan struct {
		Scan         catalog.SourceScan `json:"scan"`
		PreviewToken string             `json:"preview_token"`
		Candidates   []importCandidate  `json:"candidates"`
	}
	jsonRequest(t, handler, http.MethodPost, "/api/v1/sources/"+source.ID+"/scans", map[string]any{}, &scan)
	if len(scan.Candidates) != 1 || scan.Candidates[0].Game.Artifacts[0].Path != "roms/gba/separate.gba" {
		t.Fatalf("persistent scan did not use content root: %#v", scan)
	}
	var result map[string]any
	jsonRequest(t, handler, http.MethodPost, "/api/v1/source-scans/"+scan.Scan.ID+"/commit", sourceScanCommitRequest{PreviewToken: scan.PreviewToken, SelectedTokens: []string{scan.Candidates[0].Token}}, &result)
	if result["imported"] != float64(1) {
		t.Fatalf("persistent separate-root commit failed: %#v", result)
	}
}

func TestROMScanPreviewAndCommitAPI(t *testing.T) {
	store, handler, root := testServer(t)
	if err := os.MkdirAll(filepath.Join(root, "gba"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "gba", "Direct.gba"), []byte("direct-rom"), 0o644); err != nil {
		t.Fatal(err)
	}
	request := romImportRequest{Source: "gba", Platform: "gba", ROMStorage: "reference"}
	var preview struct {
		Parsed            int                      `json:"parsed"`
		PreviewToken      string                   `json:"preview_token"`
		Candidates        []importCandidate        `json:"candidates"`
		SourceDiagnostics []importSourceDiagnostic `json:"source_diagnostics"`
	}
	jsonRequest(t, handler, http.MethodPost, "/api/v1/imports/roms/preview", request, &preview)
	if preview.Parsed != 1 || preview.Candidates[0].Status != "new" || preview.Candidates[0].Availability != "ready" || len(preview.SourceDiagnostics) != 0 {
		t.Fatalf("bad ROM preview: %#v", preview)
	}
	games, err := store.ListGames(context.Background(), "")
	if err != nil || len(games) != 0 {
		t.Fatalf("ROM preview mutated catalog: games=%#v err=%v", games, err)
	}
	request.PreviewToken = preview.PreviewToken
	request.SelectedTokens = []string{preview.Candidates[0].Token}
	var result struct {
		Imported int `json:"imported"`
	}
	jsonRequest(t, handler, http.MethodPost, "/api/v1/imports/roms/commit", request, &result)
	if result.Imported != 1 {
		t.Fatalf("bad ROM commit: %#v", result)
	}
	request.PreviewToken = ""
	request.SelectedTokens = nil
	jsonRequest(t, handler, http.MethodPost, "/api/v1/imports/roms/preview", request, &preview)
	if preview.Candidates[0].Status != "duplicate" {
		t.Fatalf("duplicate ROM not reported: %#v", preview)
	}
}

func TestROMImportRejectsStaleAndTamperedCandidateTokens(t *testing.T) {
	store, handler, root := testServer(t)
	romDir := filepath.Join(root, "gba")
	if err := os.MkdirAll(romDir, 0o755); err != nil {
		t.Fatal(err)
	}
	romPath := filepath.Join(romDir, "Drift.gba")
	if err := os.WriteFile(romPath, []byte("before-preview"), 0o644); err != nil {
		t.Fatal(err)
	}
	request := romImportRequest{Source: "gba", Platform: "gba", ROMStorage: "reference"}
	var preview struct {
		PreviewToken string            `json:"preview_token"`
		Candidates   []importCandidate `json:"candidates"`
	}
	jsonRequest(t, handler, http.MethodPost, "/api/v1/imports/roms/preview", request, &preview)
	request.PreviewToken = preview.PreviewToken
	request.SelectedTokens = []string{preview.Candidates[0].Token + "tampered"}
	tampered := jsonErrorRequest(t, handler, http.MethodPost, "/api/v1/imports/roms/commit", request)
	if tampered.Code != http.StatusConflict || !strings.Contains(tampered.Body.String(), "import_preview_stale") {
		t.Fatalf("tampered candidate token was not rejected: %d %s", tampered.Code, tampered.Body.String())
	}

	request.SelectedTokens = []string{preview.Candidates[0].Token, preview.Candidates[0].Token}
	duplicate := jsonErrorRequest(t, handler, http.MethodPost, "/api/v1/imports/roms/commit", request)
	if duplicate.Code != http.StatusBadRequest || !strings.Contains(duplicate.Body.String(), "selected_tokens must not contain duplicates") {
		t.Fatalf("duplicate candidate token was not rejected: %d %s", duplicate.Code, duplicate.Body.String())
	}

	request.SelectedTokens = []string{preview.Candidates[0].Token}
	if err := os.WriteFile(romPath, []byte("changed-after-preview"), 0o644); err != nil {
		t.Fatal(err)
	}
	stale := jsonErrorRequest(t, handler, http.MethodPost, "/api/v1/imports/roms/commit", request)
	if stale.Code != http.StatusConflict || !strings.Contains(stale.Body.String(), "import_preview_stale") {
		t.Fatalf("changed source was not rejected: %d %s", stale.Code, stale.Body.String())
	}
	games, err := store.ListGames(context.Background(), "")
	if err != nil || len(games) != 0 {
		t.Fatalf("stale preview wrote metadata: games=%#v err=%v", games, err)
	}
}

func TestROMImportBatchFailureIsAtomicAndCleansManagedFiles(t *testing.T) {
	store, handler, root := testServer(t)
	romDir := filepath.Join(root, "gba")
	if err := os.MkdirAll(romDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Same A.gba", "Same B.gba"} {
		if err := os.WriteFile(filepath.Join(romDir, name), []byte("identical-content"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	request := romImportRequest{Source: "gba", Platform: "gba", ROMStorage: "copy"}
	var preview struct {
		PreviewToken string            `json:"preview_token"`
		Candidates   []importCandidate `json:"candidates"`
	}
	jsonRequest(t, handler, http.MethodPost, "/api/v1/imports/roms/preview", request, &preview)
	if len(preview.Candidates) != 2 {
		t.Fatalf("expected two candidates: %#v", preview)
	}
	request.PreviewToken = preview.PreviewToken
	request.SelectedTokens = []string{preview.Candidates[0].Token, preview.Candidates[1].Token}
	failed := jsonErrorRequest(t, handler, http.MethodPost, "/api/v1/imports/roms/commit", request)
	if failed.Code != http.StatusConflict || !strings.Contains(failed.Body.String(), "import_batch_conflict") {
		t.Fatalf("atomic batch conflict not reported: %d %s", failed.Code, failed.Body.String())
	}
	games, err := store.ListGames(context.Background(), "")
	if err != nil || len(games) != 0 {
		t.Fatalf("failed batch left metadata: games=%#v err=%v", games, err)
	}
	managedPlatform := filepath.Join(root, ".library-data", "roms", "gba")
	entries, err := os.ReadDir(managedPlatform)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("failed batch left managed ROM directories: %#v", entries)
	}
}

func TestMetadataImportReportsAndSkipsMissingROM(t *testing.T) {
	store, handler, root := testServer(t)
	if err := os.MkdirAll(filepath.Join(root, "gba"), 0o755); err != nil {
		t.Fatal(err)
	}
	metadata := "game: Offline Game\nfile: missing.gba\n"
	if err := os.WriteFile(filepath.Join(root, "gba", "metadata.pegasus.txt"), []byte(metadata), 0o644); err != nil {
		t.Fatal(err)
	}
	request := importRequest{Format: "pegasus", Source: "gba/metadata.pegasus.txt", Platform: "gba", ROMStorage: "reference"}
	var preview struct {
		PreviewToken string            `json:"preview_token"`
		Candidates   []importCandidate `json:"candidates"`
	}
	jsonRequest(t, handler, http.MethodPost, "/api/v1/imports/preview", request, &preview)
	if len(preview.Candidates) != 1 || preview.Candidates[0].Status != "missing" || preview.Candidates[0].MissingArtifacts != 1 {
		t.Fatalf("missing ROM not reported: %#v", preview)
	}
	request.PreviewToken = preview.PreviewToken
	request.SelectedTokens = []string{preview.Candidates[0].Token}
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	httpRequest := httptest.NewRequest(http.MethodPost, "/api/v1/imports/commit", bytes.NewReader(body))
	httpRequest.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(recorder, httpRequest)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("missing ROM commit should fail: %d %s", recorder.Code, recorder.Body.String())
	}
	games, err := store.ListGames(context.Background(), "")
	if err != nil || len(games) != 0 {
		t.Fatalf("missing metadata created catalog records: games=%#v err=%v", games, err)
	}
}

func TestMetadataImportReportsAggregateWrappedSourceDiagnostics(t *testing.T) {
	_, handler, root := testServer(t)
	metadataDir := filepath.Join(root, "metadata", "pokemini")
	contentDir := filepath.Join(root, "downloads", "pokemini")
	if err := os.MkdirAll(metadataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(contentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(metadataDir, "metadata.pegasus.txt"), []byte("game: Wrapped source\nfile: missing.min\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"POKE MINI.7z.tkzlm", "POKE MINI.zip.001", "POKE MINI.zip.002", "private-platform.7z.tkzlm", "playable.zip"} {
		if err := os.WriteFile(filepath.Join(contentDir, name), []byte("synthetic container marker"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	request := importRequest{Format: "pegasus", Source: "metadata/pokemini/metadata.pegasus.txt", ContentRoot: "downloads/pokemini", Platform: "pokemini", ROMStorage: "reference", MediaStorage: "ignore"}
	response := jsonErrorRequest(t, handler, http.MethodPost, "/api/v1/imports/preview", request)
	if response.Code != http.StatusOK {
		t.Fatalf("wrapped source preview: %d %s", response.Code, response.Body.String())
	}
	var preview struct {
		PreviewToken      string                   `json:"preview_token"`
		Candidates        []importCandidate        `json:"candidates"`
		SourceDiagnostics []importSourceDiagnostic `json:"source_diagnostics"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if len(preview.Candidates) != 1 || preview.Candidates[0].Status != "missing" {
		t.Fatalf("missing candidate changed: %#v", preview.Candidates)
	}
	wantDiagnostics := []importSourceDiagnostic{
		{Code: "wrapped_archives_detected", Count: 2},
		{Code: "split_archives_detected", Count: 2},
		{Code: "platform_wrapped_archives_detected", Count: 1},
		{Code: "platform_split_archive_parts_detected", Count: 2},
	}
	if !slices.Equal(preview.SourceDiagnostics, wantDiagnostics) {
		t.Fatalf("aggregate source diagnostics changed: %#v", preview.SourceDiagnostics)
	}
	responseText := response.Body.String()
	for _, privateValue := range []string{"private-platform", "POKE MINI", root} {
		if strings.Contains(responseText, privateValue) {
			t.Fatalf("source diagnostic leaked private source detail %q: %s", privateValue, responseText)
		}
	}

	var source catalog.LibrarySource
	jsonRequest(t, handler, http.MethodPost, "/api/v1/sources", catalog.NewLibrarySource{
		Name: "Wrapped source", Kind: "pegasus", RootPath: "downloads/pokemini", MetadataPath: "metadata/pokemini/metadata.pegasus.txt", Platform: "pokemini", ROMStoragePolicy: "reference", MediaStoragePolicy: "ignore",
	}, &source)
	var scan struct {
		SourceDiagnostics []importSourceDiagnostic `json:"source_diagnostics"`
	}
	jsonRequest(t, handler, http.MethodPost, "/api/v1/sources/"+source.ID+"/scans", map[string]any{}, &scan)
	if !slices.Equal(scan.SourceDiagnostics, wantDiagnostics) {
		t.Fatalf("persistent source lost diagnostics: %#v", scan.SourceDiagnostics)
	}
}

func TestManagedImportAndMediaAPI(t *testing.T) {
	store, handler, root := testServer(t)
	if err := os.MkdirAll(filepath.Join(root, "gba", "media"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "gba", "managed.gba"), []byte("managed-rom"), 0o644); err != nil {
		t.Fatal(err)
	}
	png := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 520)...)
	if err := os.WriteFile(filepath.Join(root, "gba", "media", "cover.png"), png, 0o644); err != nil {
		t.Fatal(err)
	}
	gamelist := `<?xml version="1.0"?><gameList><game><path>./managed.gba</path><name>Managed Game</name><image>./media/cover.png</image></game></gameList>`
	if err := os.WriteFile(filepath.Join(root, "gba", "gamelist.xml"), []byte(gamelist), 0o644); err != nil {
		t.Fatal(err)
	}
	request := importRequest{Format: "es-de", Source: "gba/gamelist.xml", Platform: "gba", Locale: "en", ROMStorage: "copy", MediaStorage: "copy"}
	var managedPreview struct {
		PreviewToken string            `json:"preview_token"`
		Candidates   []importCandidate `json:"candidates"`
	}
	jsonRequest(t, handler, http.MethodPost, "/api/v1/imports/preview", request, &managedPreview)
	request.PreviewToken = managedPreview.PreviewToken
	request.SelectedTokens = []string{managedPreview.Candidates[0].Token}
	var committed struct {
		Imported   int `json:"imported"`
		ROMFiles   int `json:"rom_files_copied"`
		MediaFiles int `json:"media_files_copied"`
	}
	jsonRequest(t, handler, http.MethodPost, "/api/v1/imports/commit", request, &committed)
	if committed.Imported != 1 || committed.ROMFiles != 1 || committed.MediaFiles != 1 {
		t.Fatalf("bad managed import: %#v", committed)
	}
	request.PreviewToken = ""
	request.SelectedTokens = nil
	var duplicatePreview struct {
		Candidates []importCandidate `json:"candidates"`
	}
	jsonRequest(t, handler, http.MethodPost, "/api/v1/imports/preview", request, &duplicatePreview)
	if len(duplicatePreview.Candidates) != 1 || duplicatePreview.Candidates[0].Status != "duplicate" {
		t.Fatalf("managed source duplicate not detected: %#v", duplicatePreview)
	}
	games, err := store.ListGames(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	artifact, importedMedia := games[0].Editions[0].Artifacts[0], games[0].Editions[0].Media[0]
	if artifact.StorageKind != "managed" || artifact.SourcePath != "gba/managed.gba" || importedMedia.StorageKind != "managed" {
		t.Fatalf("storage provenance missing: %#v %#v", artifact, importedMedia)
	}
	if _, err = os.Stat(filepath.Join(root, ".library-data", "roms", filepath.FromSlash(artifact.Path))); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(root, ".library-data", "media", filepath.FromSlash(importedMedia.Path))); err != nil {
		t.Fatal(err)
	}
	mediaRoot := filepath.Join(root, ".library-data", "media")
	countMediaFiles := func() int {
		count := 0
		if walkErr := filepath.WalkDir(mediaRoot, func(_ string, entry os.DirEntry, walkErr error) error {
			if walkErr == nil && !entry.IsDir() {
				count++
			}
			return walkErr
		}); walkErr != nil {
			t.Fatal(walkErr)
		}
		return count
	}
	filesBeforeInvalidUpload := countMediaFiles()
	var invalidBody bytes.Buffer
	invalidWriter := multipart.NewWriter(&invalidBody)
	_ = invalidWriter.WriteField("game_id", "missing-work")
	_ = invalidWriter.WriteField("kind", "cover")
	invalidPart, err := invalidWriter.CreateFormFile("file", "orphan.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = invalidPart.Write(append(png, byte(1))); err != nil {
		t.Fatal(err)
	}
	if err = invalidWriter.Close(); err != nil {
		t.Fatal(err)
	}
	invalidUpload := httptest.NewRequest(http.MethodPost, "/api/v1/media/upload", &invalidBody)
	invalidUpload.Header.Set("Content-Type", invalidWriter.FormDataContentType())
	invalidResponse := httptest.NewRecorder()
	handler.ServeHTTP(invalidResponse, invalidUpload)
	if invalidResponse.Code != http.StatusNotFound || countMediaFiles() != filesBeforeInvalidUpload {
		t.Fatalf("invalid owner created an orphan media blob: status=%d before=%d after=%d", invalidResponse.Code, filesBeforeInvalidUpload, countMediaFiles())
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("game_id", games[0].ID)
	_ = writer.WriteField("kind", "cover")
	part, err := writer.CreateFormFile("file", "uploaded.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = part.Write(png); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	upload := httptest.NewRequest(http.MethodPost, "/api/v1/media/upload", &body)
	upload.Header.Set("Content-Type", writer.FormDataContentType())
	uploadResponse := httptest.NewRecorder()
	handler.ServeHTTP(uploadResponse, upload)
	if uploadResponse.Code != http.StatusCreated || uploadResponse.Header().Get("Location") == "" {
		t.Fatalf("media upload: %d %s", uploadResponse.Code, uploadResponse.Body.String())
	}
	var item catalog.MediaAsset
	if err = json.Unmarshal(uploadResponse.Body.Bytes(), &item); err != nil {
		t.Fatal(err)
	}
	if item.ContentStatus != "available" || item.ContentCheckedAt == "" {
		t.Fatalf("validated upload did not record media availability: %#v", item)
	}
	content := httptest.NewRecorder()
	handler.ServeHTTP(content, httptest.NewRequest(http.MethodGet, "/api/v1/media/"+item.ID+"/content", nil))
	if content.Code != http.StatusOK || content.Header().Get("ETag") == "" || !bytes.Equal(content.Body.Bytes(), png) {
		t.Fatalf("media content: %d %#v", content.Code, content.Header())
	}
	cached := httptest.NewRequest(http.MethodGet, "/api/v1/media/"+item.ID+"/content", nil)
	cached.Header.Set("If-None-Match", content.Header().Get("ETag"))
	cachedResponse := httptest.NewRecorder()
	handler.ServeHTTP(cachedResponse, cached)
	if cachedResponse.Code != http.StatusNotModified {
		t.Fatalf("media conditional GET: %d", cachedResponse.Code)
	}
	managedPath := filepath.Join(root, ".library-data", "media", filepath.FromSlash(item.Path))
	corrupt := bytes.Repeat([]byte{0x7f}, len(png))
	if err = os.WriteFile(managedPath, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	corruptRequest := httptest.NewRequest(http.MethodGet, "/api/v1/media/"+item.ID+"/content", nil)
	corruptRequest.Header.Set("If-None-Match", content.Header().Get("ETag"))
	corruptResponse := httptest.NewRecorder()
	handler.ServeHTTP(corruptResponse, corruptRequest)
	var corruptFailure apiErrorEnvelope
	if err = json.Unmarshal(corruptResponse.Body.Bytes(), &corruptFailure); err != nil {
		t.Fatal(err)
	}
	if corruptResponse.Code != http.StatusConflict || corruptFailure.Error.Code != "media_content_integrity_failed" || strings.Contains(corruptResponse.Body.String(), root) || bytes.Contains(corruptResponse.Body.Bytes(), corrupt[:16]) {
		t.Fatalf("corrupt media response leaked or bypassed validation: status=%d body=%s", corruptResponse.Code, corruptResponse.Body.String())
	}
	var corruptRecheck struct {
		Checked int `json:"checked"`
		Changed int `json:"changed"`
	}
	jsonRequest(t, handler, http.MethodPost, "/api/v1/media/recheck", map[string]any{}, &corruptRecheck)
	if corruptRecheck.Checked < 2 || corruptRecheck.Changed < 2 {
		t.Fatalf("corrupt shared blob was not reported for every relation: %#v", corruptRecheck)
	}
	var corruptItem catalog.MediaAsset
	jsonRequest(t, handler, http.MethodGet, "/api/v1/media/"+item.ID, nil, &corruptItem)
	if corruptItem.ContentStatus != "changed" || corruptItem.ContentCheckedAt == "" {
		t.Fatalf("corrupt status was not persisted: %#v", corruptItem)
	}
	if err = os.Remove(managedPath); err != nil {
		t.Fatal(err)
	}
	missingResponse := httptest.NewRecorder()
	handler.ServeHTTP(missingResponse, httptest.NewRequest(http.MethodGet, "/api/v1/media/"+item.ID+"/content", nil))
	var missingFailure apiErrorEnvelope
	if err = json.Unmarshal(missingResponse.Body.Bytes(), &missingFailure); err != nil {
		t.Fatal(err)
	}
	if missingResponse.Code != http.StatusNotFound || missingFailure.Error.Code != "media_content_unavailable" || strings.Contains(missingResponse.Body.String(), root) {
		t.Fatalf("missing media response = status=%d body=%s", missingResponse.Code, missingResponse.Body.String())
	}
	var missingRecheck struct {
		Checked int `json:"checked"`
		Missing int `json:"missing"`
	}
	jsonRequest(t, handler, http.MethodPost, "/api/v1/media/recheck", map[string]any{}, &missingRecheck)
	if missingRecheck.Checked < 2 || missingRecheck.Missing < 2 {
		t.Fatalf("missing shared blob was not reported for every relation: %#v", missingRecheck)
	}
	var missingItem catalog.MediaAsset
	jsonRequest(t, handler, http.MethodGet, "/api/v1/media/"+item.ID, nil, &missingItem)
	if missingItem.ContentStatus != "missing" || missingItem.ContentCheckedAt == "" {
		t.Fatalf("missing status was not persisted: %#v", missingItem)
	}
	if status := jsonRequest(t, handler, http.MethodDelete, "/api/v1/media/"+item.ID, nil, nil); status != http.StatusNoContent {
		t.Fatalf("media delete: %d", status)
	}
}

func TestImportSourceCollectionDirectoryNormalization(t *testing.T) {
	_, handler, root := testServer(t)
	for _, directory := range []string{"FC hack", "SFC-MSU1", "PS2 hack", "FBNEO ACT V"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, directory, "metadata.pegasus.txt"), []byte("collection: "+directory+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var sources []importSource
	jsonRequest(t, handler, http.MethodGet, "/api/import-sources?format=pegasus", nil, &sources)
	want := map[string]string{"FC hack": "nes", "SFC-MSU1": "snes", "PS2 hack": "ps2", "FBNEO ACT V": "arcade"}
	for _, source := range sources {
		directory := filepath.Base(filepath.Dir(filepath.FromSlash(source.Path)))
		if source.Platform != want[directory] {
			t.Fatalf("%s mapped to %q, want %q", directory, source.Platform, want[directory])
		}
	}
}

func TestBearerAuthProtectsAPI(t *testing.T) {
	root := filepath.Join(t.TempDir(), "library")
	_ = os.MkdirAll(root, 0o755)
	store, err := catalog.Open(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app, err := New(store, root, WithToken("secret-token"))
	if err != nil {
		t.Fatal(err)
	}
	unauthorized := httptest.NewRecorder()
	app.Handler().ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/games", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", unauthorized.Code)
	}
	unauthorizedV1 := httptest.NewRecorder()
	app.Handler().ServeHTTP(unauthorizedV1, httptest.NewRequest(http.MethodGet, "/api/v1/games", nil))
	var failure apiErrorEnvelope
	if err := json.Unmarshal(unauthorizedV1.Body.Bytes(), &failure); err != nil {
		t.Fatal(err)
	}
	if unauthorizedV1.Code != http.StatusUnauthorized || failure.Error.Code != "authentication_required" {
		t.Fatalf("expected structured v1 401, got %d %#v", unauthorizedV1.Code, failure)
	}
	authorizedRequest := httptest.NewRequest(http.MethodGet, "/api/games", nil)
	authorizedRequest.Header.Set("Authorization", "Bearer secret-token")
	authorized := httptest.NewRecorder()
	app.Handler().ServeHTTP(authorized, authorizedRequest)
	if authorized.Code != http.StatusOK {
		t.Fatalf("authorized request: %d %s", authorized.Code, authorized.Body.String())
	}
	health := httptest.NewRecorder()
	app.Handler().ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if health.Code != http.StatusOK || !strings.Contains(health.Header().Get("Content-Security-Policy"), "img-src 'self' data: blob:") {
		t.Fatalf("health/security headers missing: %d %#v", health.Code, health.Header())
	}
	for _, path := range []string{"/api/v1/health/ready", "/api/v1/capabilities", "/api/v1/openapi.yaml"} {
		public := httptest.NewRecorder()
		app.Handler().ServeHTTP(public, httptest.NewRequest(http.MethodGet, path, nil))
		if public.Code != http.StatusOK {
			t.Fatalf("public endpoint %s: %d %s", path, public.Code, public.Body.String())
		}
	}
}

func TestMutableStateCanLiveOutsideReadOnlyLibrary(t *testing.T) {
	root := filepath.Join(t.TempDir(), "library")
	state := filepath.Join(t.TempDir(), "state")
	if err := os.MkdirAll(filepath.Join(root, "gba"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "gba", "game.gba"), []byte("rom"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := catalog.Open(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	game, err := store.CreateGame(context.Background(), catalog.NewGame{DefaultTitle: "Game", Platform: "gba"})
	if err != nil {
		t.Fatal(err)
	}
	edition, err := store.AddEdition(context.Background(), catalog.NewEdition{GameID: game.ID, DefaultTitle: "Game", EditionType: "original"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.AddArtifact(context.Background(), catalog.NewArtifact{EditionID: edition.ID, Path: "gba/game.gba", Role: "rom"}); err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(root, 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(root, 0o755)
	app, err := New(store, root, WithStateRoot(state))
	if err != nil {
		t.Fatal(err)
	}
	var games []catalog.Game
	jsonRequest(t, app.Handler(), http.MethodGet, "/api/games", nil, &games)
	if len(games) != 1 {
		t.Fatalf("readonly library unavailable: %#v", games)
	}
	if _, err = os.Stat(state); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(root, ".library-data")); !os.IsNotExist(err) {
		t.Fatalf("server wrote mutable state into library: %v", err)
	}
}
