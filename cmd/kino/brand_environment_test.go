package main

import "testing"

func TestBrandEnvironmentKeepsExplicitKinoChoice(t *testing.T) {
	t.Setenv("VARKIV_WEB_EMULATOR_ASSETS", "legacy")
	t.Setenv("KINO_WEB_EMULATOR_ASSETS", "current")
	if got := brandEnvironment("KINO_WEB_EMULATOR_ASSETS"); got != "current" {
		t.Fatal(got)
	}
	t.Setenv("KINO_WEB_EMULATOR_ASSETS", "")
	if got := brandEnvironment("KINO_WEB_EMULATOR_ASSETS"); got != "" {
		t.Fatal(got)
	}
	t.Setenv("VARKIV_RENAME_TEST_ONLY", "retained")
	if got := brandEnvironment("KINO_RENAME_TEST_ONLY"); got != "retained" {
		t.Fatal(got)
	}
}
