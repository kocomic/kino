package server

import (
	"crypto/rand"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"kino/internal/buildinfo"
	"kino/internal/catalog"
	"kino/internal/platforms"

	storagex "kino/internal/storage"
)

const apiVersion = "v1"

//go:embed openapi.yaml
var openAPI []byte

type apiErrorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

type apiErrorEnvelope struct {
	Error apiErrorBody `json:"error"`
}

type pagination struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
	Total  int `json:"total"`
}

type collectionEnvelope[T any] struct {
	Data       []T        `json:"data"`
	Pagination pagination `json:"pagination"`
}

func apiContract(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api") {
			next.ServeHTTP(w, r)
			return
		}
		requestID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if !validRequestID(requestID) {
			var data [16]byte
			if _, err := rand.Read(data[:]); err == nil {
				requestID = hex.EncodeToString(data[:])
			} else {
				requestID = "request-id-unavailable"
			}
		}
		w.Header().Set("X-Request-ID", requestID)
		w.Header().Set("X-Kino-API-Version", apiVersion)

		w.Header().Set("Cache-Control", "no-store")
		if (r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/")) && r.URL.Path != "/api/v1" && !strings.HasPrefix(r.URL.Path, "/api/v1/") {
			w.Header().Set("Deprecation", "true")
			w.Header().Set("Link", `</api/v1>; rel="successor-version"`)
		}
		next.ServeHTTP(w, r)
	})
}

func validRequestID(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if !(r == '-' || r == '_' || r == '.' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
			return false
		}
	}
	return true
}

func isV1(r *http.Request) bool {
	return r.URL.Path == "/api/v1" || strings.HasPrefix(r.URL.Path, "/api/v1/")
}

func resourceLocation(r *http.Request, resource, id string) string {
	prefix := "/api"
	if isV1(r) {
		prefix = "/api/v1"
	}
	return prefix + "/" + resource + "/" + id
}

func publicAPIPath(path string) bool {
	return path == "/api" || path == "/api/v1" || path == "/api/health" || path == "/api/v1/health" || path == "/api/v1/health/live" || path == "/api/v1/health/ready" || path == "/api/v1/capabilities" || path == "/api/v1/openapi.yaml"
}

func (s *Server) apiRoot(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"name":            "Kino API",
		"api_version":     apiVersion,
		"service_version": buildinfo.Version,
		"links": map[string]string{
			"capabilities":      "/api/v1/capabilities",
			"health":            "/api/v1/health",
			"liveness":          "/api/v1/health/live",
			"readiness":         "/api/v1/health/ready",
			"openapi":           "/api/v1/openapi.yaml",
			"media":             "/api/v1/media",
			"games":             "/api/v1/games",
			"series":            "/api/v1/series",
			"sources":           "/api/v1/sources",
			"source_scans":      "/api/v1/source-scans",
			"hash_sources":      "/api/v1/hash-sources",
			"hash_pack_preview": "/api/v1/hash-packs/preview",
			"hash_pack_export":  "/api/v1/hash-packs/export",
			"source_adapters":   "/api/v1/source-adapters",
			"frontend_adapters": "/api/v1/frontend-adapters",
			"storage_cleanup":   "/api/v1/storage-cleanup/preview",
		},
	})
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "name": "Kino", "version": buildinfo.Version, "api_version": apiVersion, "auth_required": s.token != ""})
}

func (s *Server) healthLive(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) healthReady(w http.ResponseWriter, r *http.Request) {
	version, err := s.store.SchemaVersion(r.Context())
	if err != nil {
		writeAPIError(w, http.StatusServiceUnavailable, "not_ready", "database is not ready")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ready", "schema_version": version, "supported_schema_version": catalog.CurrentSchemaVersion})
}

const capabilitiesContractVersion = 1

type capabilityDocument struct {
	ContractVersion    int             `json:"contract_version"`
	APIVersion         string          `json:"api_version"`
	Imports            []string        `json:"imports"`
	Exports            []string        `json:"exports"`
	FileModes          []string        `json:"file_modes"`
	ROMImportStorage   []string        `json:"rom_import_storage"`
	MediaImportStorage []string        `json:"media_import_storage"`
	MediaUploadMaxMB   int             `json:"media_upload_max_mb"`
	Locales            []string        `json:"locales"`
	PlatformPresets    int             `json:"platform_presets"`
	CustomPlatforms    int             `json:"custom_platforms"`
	Features           map[string]bool `json:"features"`
}

func capabilityFeatures() map[string]bool {
	return map[string]bool{
		"rom_scan_preview":            true,
		"import_preview_tokens":       true,
		"atomic_import_batches":       true,
		"missing_rom_detection":       true,
		"persistent_sources":          true,
		"source_scan_audit":           true,
		"source_adapters":             true,
		"bounded_source_preview":      true,
		"unmanaged_output_guard":      true,
		"frontend_adapters":           true,
		"portable_runtime_catalog_v2": true,
		"reserved_builtin_namespace":  true,
		"neutral_manifest_v5":         true,
		"neutral_manifest_v6":         true,
		"portable_custom_platforms":   true,
		"custom_platforms":            true,
		"edition_grouping":            true,
		"game_merge_preview":          true,
		"cross_platform_series":       true,
		"multilingual_names":          true,
		"directory_rom_inventory":     true,
		"managed_roms":                true,
		"media_assets":                true,
		"media_thumbnails":            true,
		"managed_storage_quarantine":  true,
		"managed_storage_restore":     true,
		"hash_pack_import":            true,
		"hash_pack_export":            true,
		"hash_identity_provenance":    true,
	}
}

func (s *Server) capabilities(w http.ResponseWriter, r *http.Request) {
	customPlatforms, err := s.store.ListCustomPlatforms(r.Context(), true)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, capabilityDocument{
		ContractVersion:  capabilitiesContractVersion,
		APIVersion:       apiVersion,
		Imports:          []string{"rom-file", "rom-directory", "pegasus", "es-de", "kino-v6", "kino-v5-compatible", "kino-v4-compatible", "kino-hashpack-v1"},
		Exports:          []string{"pegasus", "es-de", "kino-hashpack-v1"},
		FileModes:        []string{"copy", "hardlink", "reference"},
		ROMImportStorage: []string{"reference", "copy"},
		MediaImportStorage: []string{
			"copy", "reference", "ignore",
		},
		MediaUploadMaxMB: 64,
		Locales:          []string{"zh-CN", "zh-TW", "ja", "en"},
		PlatformPresets:  len(platforms.All()),
		CustomPlatforms:  len(customPlatforms),
		Features:         capabilityFeatures(),
	})
}

func (s *Server) openAPISpec(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(openAPI)
}

func decode(w http.ResponseWriter, r *http.Request, value any) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeAPIError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "Content-Type must be application/json")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(value); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "invalid JSON: "+err.Error())
		return false
	}
	if err = decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "request body must contain exactly one JSON value")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "encoding_failed", "response encoding failed")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(append(data, '\n'))
}

func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, apiErrorEnvelope{Error: apiErrorBody{Code: code, Message: message, RequestID: w.Header().Get("X-Request-ID")}})
}

func writeError(w http.ResponseWriter, err error) {
	status, code, message := http.StatusInternalServerError, "internal_error", "internal server error"
	var maxBytes *http.MaxBytesError
	switch {
	case errors.As(err, &maxBytes):
		status, code, message = http.StatusRequestEntityTooLarge, "payload_too_large", "request body exceeds the allowed size"
	case errors.Is(err, errImportPreviewStale):
		status, code, message = http.StatusConflict, "import_preview_stale", "the import source or library changed after preview; generate a new preview"
	case errors.Is(err, errHashPackPreviewStale):
		status, code, message = http.StatusConflict, "hash_pack_preview_stale", "the hash pack changed after preview; review the exact file again"
	case errors.Is(err, catalog.ErrHashReleaseConflict):
		status, code, message = http.StatusConflict, "hash_release_conflict", "the same source release already exists with different content; publish a new release identifier"
	case errors.Is(err, catalog.ErrGameMergeStale):
		status, code, message = http.StatusConflict, "game_merge_stale", "the game, editions, media, or series changed after preview; review the merge again"
	case errors.Is(err, catalog.ErrInvalidGameMerge):
		status, code, message = http.StatusBadRequest, "invalid_argument", err.Error()
	case errors.Is(err, errManagedCleanupStale):
		status, code, message = http.StatusConflict, "managed_cleanup_stale", "managed storage or catalog references changed after preview; generate a new cleanup preview"
	case errors.Is(err, storagex.ErrManagedStorageChanged):
		status, code, message = http.StatusConflict, "managed_cleanup_stale", "managed storage changed after preview; generate a new cleanup preview"
	case errors.Is(err, storagex.ErrManagedStorageUnsafe):
		status, code, message = http.StatusConflict, "managed_storage_unsafe", "managed storage contains a symbolic link or unsupported file type; no files were moved"
	case errors.Is(err, storagex.ErrCleanupRunNotFound):
		status, code, message = http.StatusNotFound, "cleanup_run_not_found", "cleanup recovery operation not found"
	case errors.Is(err, storagex.ErrCleanupRestoreConflict):
		status, code, message = http.StatusConflict, "cleanup_restore_conflict", "a managed path is occupied; no files were restored"
	case errors.Is(err, storagex.ErrCleanupRecoveryDamaged):
		status, code, message = http.StatusConflict, "cleanup_recovery_damaged", "cleanup recovery data is unavailable or changed; no files were restored"
	case errors.Is(err, storagex.ErrMediaUnavailable):
		status, code, message = http.StatusNotFound, "media_content_unavailable", "media content is unavailable; restore the source or managed blob"
	case errors.Is(err, storagex.ErrMediaIntegrity):
		status, code, message = http.StatusConflict, "media_content_integrity_failed", "media content does not match its recorded identity; no bytes were returned"
	case errors.Is(err, errMediaThumbnailUnsupported):
		status, code, message = http.StatusUnsupportedMediaType, "media_thumbnail_unsupported", "media cannot be safely decoded as a PNG, JPEG, or GIF thumbnail"
	case errors.Is(err, errMediaThumbnailUnverified):
		status, code, message = http.StatusUnprocessableEntity, "media_thumbnail_unverified", "media needs a verified SHA-256 identity before a thumbnail can be cached"
	case errors.Is(err, errMediaThumbnailTooLarge):
		status, code, message = http.StatusUnprocessableEntity, "media_thumbnail_too_large", "media dimensions exceed thumbnail safety limits"
	case errors.Is(err, catalog.ErrImportDuplicate):
		status, code, message = http.StatusConflict, "import_batch_conflict", "the import batch conflicts with existing content; no selected item was imported"
	case errors.Is(err, catalog.ErrLegacyDataRequiresMigration):
		status, code, message = http.StatusConflict, "legacy_data_requires_migration", err.Error()
	case errors.Is(err, sql.ErrNoRows):
		status, code, message = http.StatusNotFound, "not_found", "resource not found"
	case errors.Is(err, catalog.ErrPlatformMismatch):
		status, code, message = http.StatusConflict, "platform_mismatch", err.Error()
	case errors.Is(err, catalog.ErrSourceHasScans):
		status, code, message = http.StatusConflict, "source_has_scan_history", err.Error()
	case errors.Is(err, catalog.ErrPackageProfileHasHistory):
		status, code, message = http.StatusConflict, "package_profile_has_history", err.Error()
	case errors.Is(err, catalog.ErrBuiltinImmutable):
		status, code, message = http.StatusConflict, "builtin_immutable", err.Error()
	case errors.Is(err, catalog.ErrBuiltinNamespaceReserved):
		status, code, message = http.StatusConflict, "builtin_namespace_reserved", err.Error()
	case errors.Is(err, catalog.ErrRuntimeObjectInUse):
		status, code, message = http.StatusConflict, "runtime_object_in_use", err.Error()
	case errors.Is(err, catalog.ErrCustomPlatformInUse):
		status, code, message = http.StatusConflict, "custom_platform_in_use", err.Error()
	case errors.Is(err, catalog.ErrPlatformDefinitionConflict):
		status, code, message = http.StatusConflict, "platform_definition_conflict", err.Error()
	case errors.Is(err, catalog.ErrPlatformDefinitionDisabled):
		status, code, message = http.StatusConflict, "platform_definition_disabled", err.Error()
	case errors.Is(err, catalog.ErrRuntimeDefinitionConflict):
		status, code, message = http.StatusConflict, "runtime_definition_conflict", err.Error()
	case errors.Is(err, catalog.ErrRuntimeDefinitionDisabled):
		status, code, message = http.StatusConflict, "runtime_definition_disabled", err.Error()
	case errors.Is(err, platforms.ErrRegistryConflict):
		status, code, message = http.StatusConflict, "platform_key_conflict", err.Error()
	case errors.Is(err, errArtifactOutsideLibrary):
		status, code, message = http.StatusBadRequest, "artifact_outside_library", errArtifactOutsideLibrary.Error()
	case errors.Is(err, errArtifactMissing):
		status, code, message = http.StatusBadRequest, "artifact_missing", errArtifactMissing.Error()
	case errors.Is(err, errArtifactUnreadable):
		status, code, message = http.StatusUnprocessableEntity, "artifact_unreadable", errArtifactUnreadable.Error()
	case strings.Contains(err.Error(), "UNIQUE constraint"):
		status, code, message = http.StatusConflict, "already_exists", "resource already exists"
	case strings.Contains(err.Error(), "required"), strings.Contains(err.Error(), " must "), strings.Contains(err.Error(), "inside library"), strings.Contains(err.Error(), "out of range"), strings.Contains(err.Error(), "invalid"), strings.Contains(err.Error(), "unknown"), strings.Contains(err.Error(), "malformed"), strings.Contains(err.Error(), "exactly one"):
		status, code, message = http.StatusBadRequest, "invalid_argument", err.Error()
	}
	writeAPIError(w, status, code, message)
}

func pageBounds(r *http.Request, total int) (int, int, pagination, error) {
	request, err := collectionPageRequest(r)
	if err != nil {
		return 0, 0, pagination{}, err
	}
	limit, offset := request.Limit, request.Offset
	start := min(offset, total)
	end := min(start+limit, total)
	return start, end, pagination{Limit: limit, Offset: offset, Total: total}, nil
}

func collectionPageRequest(r *http.Request) (catalog.PageRequest, error) {
	limit, offset := 100, 0
	var err error
	if value := r.URL.Query().Get("limit"); value != "" {
		limit, err = strconv.Atoi(value)
		if err != nil || limit < 1 || limit > 200 {
			return catalog.PageRequest{}, fmt.Errorf("limit must be between 1 and 200")
		}
	}
	if value := r.URL.Query().Get("offset"); value != "" {
		offset, err = strconv.Atoi(value)
		if err != nil || offset < 0 {
			return catalog.PageRequest{}, fmt.Errorf("offset must be zero or greater")
		}
	}
	return catalog.PageRequest{Limit: limit, Offset: offset}, nil
}

func writeCatalogPage[T any](w http.ResponseWriter, page catalog.Page[T]) {
	writeJSON(w, http.StatusOK, collectionEnvelope[T]{
		Data:       page.Items,
		Pagination: pagination{Limit: page.Limit, Offset: page.Offset, Total: page.Total},
	})
}

func writeCollection[T any](w http.ResponseWriter, r *http.Request, items []T) {
	if !isV1(r) {
		writeJSON(w, http.StatusOK, items)
		return
	}
	start, end, page, err := pageBounds(r, len(items))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, collectionEnvelope[T]{Data: items[start:end], Pagination: page})
}
