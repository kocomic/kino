package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestROMDistributionHasNoCloudOrPlayerRoutes(t *testing.T) {
	store, _, root := testServer(t)
	app, err := New(store, root, WithToken("owner"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/v1/save-streams", "/api/v1/devices", "/api/v1/pairing-codes", "/api/v1/sync/sessions", "/api/v1/web-emulation/readiness", "/api/v1/web-netplay/readiness", "/api/v1/packages", "/api/multiplayer/v1/sessions", "/play/", "/netplay/"} {
		for _, method := range []string{"GET", "POST"} {
			request := httptest.NewRequest(method, path, nil)
			request.Header.Set("Authorization", "Bearer owner")
			response := httptest.NewRecorder()
			app.Handler().ServeHTTP(response, request)
			if response.Code != http.StatusNotFound {
				t.Errorf("removed %s %s = %d", method, path, response.Code)
			}
		}
	}
	for _, flag := range []string{"save_revisions", "device_agent", "web_emulation", "web_netplay", "sync_negotiation"} {
		if capabilityFeatures()[flag] {
			t.Errorf("advertised removed feature %s", flag)
		}
	}
	// Fresh defaults must not provision runnable device/emulator definitions.
	drivers, err := store.ListEmulatorDrivers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(drivers) > 0 {
		t.Fatal("ROM defaults provisioned emulator drivers")
	}
}

func TestROMRoutesMatchOpenAPIAndPreserveMethodErrors(t *testing.T) {
	store, _, root := testServer(t)
	app, err := New(store, root)
	if err != nil {
		t.Fatal(err)
	}
	operations, err := readOpenAPIOperationContracts(openAPI)
	if err != nil {
		t.Fatal(err)
	}
	documented := map[string]bool{}
	for _, op := range operations {
		path := "/api/v1" + op.path
		if op.path == "/" {
			path = "/api/v1"
		}
		documented[strings.ToUpper(op.method)+" "+path] = true
	}
	for _, pattern := range app.apiPatterns {
		_, path, _ := strings.Cut(pattern, " ")
		if path != "/api/v1" && !strings.HasPrefix(path, "/api/v1/") {
			continue
		}
		if !documented[pattern] {
			t.Errorf("undocumented %s", pattern)
		}
		delete(documented, pattern)
	}
	for key := range documented {
		t.Errorf("documented unregistered %s", key)
	}
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, httptest.NewRequest("PATCH", "/api/v1/games", nil))
	if response.Code != 405 || !strings.Contains(response.Header().Get("Allow"), "GET") {
		t.Fatalf("method error %d %s", response.Code, response.Body.String())
	}
}
