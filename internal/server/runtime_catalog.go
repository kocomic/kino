package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"kino/internal/catalog"
)

const (
	pegasusAdapterID               = "builtin-frontend-pegasus"
	esdeAdapterID                  = "builtin-frontend-esde"
	snesRawSRMCompatibilityGroupID = "builtin-save-compat-snes9x-raw-srm-v1"
)

func boolPointer(value bool) *bool { return &value }

func (s *Server) ensureRuntimeCatalog(ctx context.Context) error {
	for _, item := range builtinSourceAdapters() {
		if current, err := s.store.GetSourceAdapter(ctx, item.ID); err == nil {
			if current.ContractVersion < item.ContractVersion {
				if _, err = s.store.ReconcileBuiltinSourceAdapter(ctx, item); err != nil {
					return fmt.Errorf("upgrade source adapter %s: %w", item.ID, err)
				}
			}
			continue
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if _, err := s.store.CreateSourceAdapter(ctx, item); err != nil {
			return fmt.Errorf("seed source adapter %s: %w", item.ID, err)
		}
	}
	for _, item := range builtinFrontendAdapters() {
		if current, err := s.store.GetFrontendAdapter(ctx, item.ID); err == nil {
			if current.ContractVersion < item.ContractVersion {
				if _, err = s.store.ReconcileBuiltinFrontendAdapter(ctx, item); err != nil {
					return fmt.Errorf("upgrade frontend adapter %s: %w", item.ID, err)
				}
			}
			continue
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if _, err := s.store.CreateFrontendAdapter(ctx, item); err != nil {
			return fmt.Errorf("seed frontend adapter %s: %w", item.ID, err)
		}
	}
	return nil
}

func builtinSourceAdapters() []catalog.NewSourceAdapter {
	return []catalog.NewSourceAdapter{
		{ID: "builtin-source-direct-rom", Name: "Direct ROM scanner", Format: "direct-rom", Handler: "rom_directory", ContractVersion: 3, Capabilities: map[string]bool{"files": true, "directories": true, "directory_platforms": true, "multi_disc": true, "preview": true, "managed_copy": true}, SupportLevel: "package-tested", Evidence: map[string]any{"scope": "fixture", "verified_at": "2026-08-27", "note": "Signed preview and atomic import tests cover files, CUE/BIN, M3U groups, and one top-level directory per game on directory-declared platforms."}, Builtin: true, Enabled: boolPointer(true)},
		{ID: "builtin-source-pegasus", Name: "Pegasus metadata", Format: "pegasus", Handler: "pegasus", ContractVersion: 3, Capabilities: map[string]bool{"metadata": true, "media": true, "multi_file_games": true, "runtime_hints": true, "separate_content_root": true, "preview": true}, SupportLevel: "package-tested", Evidence: map[string]any{"scope": "fixture", "verified_at": "2026-08-28", "note": "Metadata stored separately from ROM content is resolved through an explicit library-contained root covered by signed preview and atomic commit tests."}, Builtin: true, Enabled: boolPointer(true)},
		{ID: "builtin-source-esde", Name: "ES-DE metadata", Format: "es-de", Handler: "esde", ContractVersion: 3, Capabilities: map[string]bool{"metadata": true, "media": true, "custom_systems": true, "runtime_hints": true, "separate_content_root": true, "preview": true}, SupportLevel: "package-tested", Evidence: map[string]any{"scope": "fixture", "verified_at": "2026-08-28", "note": "Gamelists can resolve ROMs from an explicit separate library-contained root while media remains relative to the metadata tree; missing ROMs are skipped."}, Builtin: true, Enabled: boolPointer(true)},
		{ID: "builtin-source-kino", Name: "Neutral recovery manifest", Format: "kino", Handler: "kino", ContractVersion: 4, Capabilities: map[string]bool{"metadata": true, "series": true, "media": true, "runtime_hints": true, "stable_ids": true, "artifact_roles": true, "artifact_integrity": true, "neutral_manifest_v6": true, "portable_custom_platforms": true, "preview": true}, SupportLevel: "package-tested", Evidence: map[string]any{"scope": "fixture", "verified_at": "2026-08-27", "note": "Manifest v6 preserves v5 game and Artifact semantics and atomically restores referenced custom platform definitions; v4/v5 remain readable."}, Builtin: true, Enabled: boolPointer(true)},
	}
}

func builtinFrontendAdapters() []catalog.NewFrontendAdapter {
	return []catalog.NewFrontendAdapter{
		{ID: pegasusAdapterID, Name: "Pegasus", Format: "pegasus", Handler: "pegasus", ContractVersion: 5, Capabilities: map[string]bool{"import": true, "export": true, "multi_file_games": true, "custom_launch_commands": true, "neutral_manifest_v6": true, "series_native": false}, SupportLevel: "package-tested", Evidence: map[string]any{"scope": "fixture", "verified_at": "2026-08-27", "note": "Generated metadata.pegasus.txt packages through the audited Pegasus handler with a v6 recovery sidecar preserving Artifact semantics and referenced custom platforms; device launch remains driver-specific."}, Builtin: true, Enabled: boolPointer(true)},
		{ID: esdeAdapterID, Name: "ES-DE", Format: "es-de", Handler: "es-de", ContractVersion: 5, Capabilities: map[string]bool{"import": true, "export": true, "multi_file_games": false, "custom_systems": true, "neutral_manifest_v6": true, "series_native": false}, SupportLevel: "package-tested", Evidence: map[string]any{"scope": "fixture", "verified_at": "2026-08-27", "note": "Generated gamelist.xml packages through the audited ES-DE handler with a v6 recovery sidecar preserving Artifact semantics and referenced custom platforms; custom system launch remains driver-specific."}, Builtin: true, Enabled: boolPointer(true)},
	}
}
