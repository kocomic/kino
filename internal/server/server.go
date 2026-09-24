package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net/http"

	"os"
	"path/filepath"
	"strings"
	"sync"

	"kino/internal/catalog"

	storagex "kino/internal/storage"
)

//go:embed web/*
var webFiles embed.FS

var (
	errArtifactOutsideLibrary = errors.New("artifact path must be inside library root")
	errArtifactMissing        = errors.New("artifact path must point to an existing file or directory before it can be added")
	errArtifactUnreadable     = errors.New("artifact file or directory cannot be read and hashed")
)

type Server struct {
	store       *catalog.Store
	libraryRoot string
	stateRoot   string
	storage     *storagex.Repository
	token       string
	importKey   [32]byte
	thumbnailMu sync.Mutex
	apiPatterns []string
	mux         *http.ServeMux
}

type Option func(*Server)

func WithToken(token string) Option {
	return func(s *Server) { s.token = strings.TrimSpace(token) }
}

func WithStateRoot(path string) Option {
	return func(s *Server) {
		if value := strings.TrimSpace(path); value != "" {
			s.stateRoot = value
		}
	}
}

func New(store *catalog.Store, libraryRoot string, options ...Option) (*Server, error) {
	s := &Server{store: store, libraryRoot: libraryRoot, stateRoot: filepath.Join(libraryRoot, ".library-data"), mux: http.NewServeMux()}
	if _, err := rand.Read(s.importKey[:]); err != nil {
		return nil, fmt.Errorf("create import signing key: %w", err)
	}
	for _, option := range options {
		option(s)
	}
	stateRoot, err := filepath.Abs(s.stateRoot)
	if err != nil {
		return nil, err
	}
	s.stateRoot = stateRoot
	if err = os.MkdirAll(s.stateRoot, 0o755); err != nil {
		return nil, err
	}
	storageRepo, err := storagex.New(s.libraryRoot, s.stateRoot)
	if err != nil {
		return nil, err
	}
	s.storage = storageRepo
	if err = EnsureDefaults(context.Background(), store); err != nil {
		return nil, err
	}
	s.routes()
	return s, nil
}

func (s *Server) Handler() http.Handler { return securityHeaders(apiContract(s.authorize(s.mux))) }

// EnsureDefaults initializes import and export adapters for every entrypoint. It intentionally has no file
// storage side effects, so CLI imports and exports can share the same database
// contract as the Web service without constructing a Server.
func EnsureDefaults(ctx context.Context, store *catalog.Store) error {
	seed := &Server{store: store}
	if err := seed.ensureRuntimeCatalog(ctx); err != nil {
		return fmt.Errorf("initialize runtime catalog: %w", err)
	}
	return nil
}

func (s *Server) routes() {
	s.apiRoutes("/api")
	s.apiRoutes("/api/v1")
	s.registerAPI("GET /api/v1/health/live", s.healthLive)
	s.registerAPI("GET /api/v1/health/ready", s.healthReady)
	s.registerAPI("GET /api/v1/capabilities", s.capabilities)
	s.registerAPI("GET /api/v1/openapi.yaml", s.openAPISpec)
	s.mux.HandleFunc("/api", s.apiNotFound)
	s.mux.HandleFunc("/api/", s.apiNotFound)
	root, _ := fs.Sub(webFiles, "web")
	s.mux.Handle("/", http.FileServer(http.FS(root)))
}

func (s *Server) apiNotFound(w http.ResponseWriter, r *http.Request) {
	if methods := s.allowedAPIMethods(r.URL.Path); len(methods) > 0 {
		w.Header().Set("Allow", strings.Join(methods, ", "))
		writeAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}
	writeAPIError(w, http.StatusNotFound, "api_route_not_found", "API route not found")
}

func (s *Server) apiRoutes(prefix string) {
	s.registerAPI("GET "+prefix, s.apiRoot)
	s.registerAPI("GET "+prefix+"/health", s.health)
	s.registerAPI("GET "+prefix+"/platforms", s.listPlatforms)
	s.registerAPI("GET "+prefix+"/platforms/{id}", s.getPlatform)
	s.registerAPI("GET "+prefix+"/custom-platforms", s.listCustomPlatforms)
	s.registerAPI("POST "+prefix+"/custom-platforms", s.createCustomPlatform)
	s.registerAPI("GET "+prefix+"/custom-platforms/{id}", s.getCustomPlatform)
	s.registerAPI("PUT "+prefix+"/custom-platforms/{id}", s.updateCustomPlatform)
	s.registerAPI("DELETE "+prefix+"/custom-platforms/{id}", s.deleteCustomPlatform)
	s.registerAPI("GET "+prefix+"/source-adapters", s.listSourceAdapters)
	s.registerAPI("POST "+prefix+"/source-adapters", s.createSourceAdapter)
	s.registerAPI("GET "+prefix+"/source-adapters/{id}", s.getSourceAdapter)
	s.registerAPI("PUT "+prefix+"/source-adapters/{id}", s.updateSourceAdapter)
	s.registerAPI("DELETE "+prefix+"/source-adapters/{id}", s.deleteSourceAdapter)
	s.registerAPI("GET "+prefix+"/frontend-adapters", s.listFrontendAdapters)
	s.registerAPI("POST "+prefix+"/frontend-adapters", s.createFrontendAdapter)
	s.registerAPI("GET "+prefix+"/frontend-adapters/{id}", s.getFrontendAdapter)
	s.registerAPI("PUT "+prefix+"/frontend-adapters/{id}", s.updateFrontendAdapter)
	s.registerAPI("DELETE "+prefix+"/frontend-adapters/{id}", s.deleteFrontendAdapter)
	s.registerAPI("POST "+prefix+"/storage-cleanup/preview", s.previewManagedStorageCleanup)
	s.registerAPI("POST "+prefix+"/storage-cleanup/commit", s.commitManagedStorageCleanup)
	s.registerAPI("GET "+prefix+"/storage-cleanup/runs", s.listManagedStorageCleanupRuns)
	s.registerAPI("POST "+prefix+"/storage-cleanup/runs/{id}/restore", s.restoreManagedStorageCleanupRun)
	s.registerAPI("GET "+prefix+"/series", s.listSeries)
	s.registerAPI("POST "+prefix+"/series", s.createSeries)
	s.registerAPI("GET "+prefix+"/series/{id}", s.getSeries)
	s.registerAPI("PUT "+prefix+"/series/{id}", s.updateSeries)
	s.registerAPI("DELETE "+prefix+"/series/{id}", s.deleteSeries)
	s.registerAPI("PUT "+prefix+"/series/{id}/members/{game_id}", s.putSeriesMember)
	s.registerAPI("DELETE "+prefix+"/series/{id}/members/{game_id}", s.deleteSeriesMember)
	s.registerAPI("GET "+prefix+"/games", s.listGames)
	s.registerAPI("GET "+prefix+"/games/{id}", s.getGame)
	s.registerAPI("POST "+prefix+"/games", s.createGame)
	s.registerAPI("PUT "+prefix+"/games/{id}", s.updateGame)
	s.registerAPI("DELETE "+prefix+"/games/{id}", s.deleteGame)
	s.registerAPI("POST "+prefix+"/games/{id}/merge/preview", s.previewGameMerge)
	s.registerAPI("POST "+prefix+"/games/{id}/merge", s.mergeGame)
	s.registerAPI("PUT "+prefix+"/games/{id}/primary", s.setPrimaryEdition)
	s.registerAPI("POST "+prefix+"/editions", s.createEdition)
	s.registerAPI("GET "+prefix+"/editions/{id}", s.getEdition)
	s.registerAPI("PUT "+prefix+"/editions/{id}", s.updateEdition)
	s.registerAPI("DELETE "+prefix+"/editions/{id}", s.deleteEdition)
	s.registerAPI("POST "+prefix+"/editions/{id}/move", s.moveEdition)
	s.registerAPI("POST "+prefix+"/artifacts", s.createArtifact)
	s.registerAPI("GET "+prefix+"/artifacts/{id}", s.getArtifact)
	s.registerAPI("PUT "+prefix+"/artifacts/{id}", s.updateArtifact)
	s.registerAPI("DELETE "+prefix+"/artifacts/{id}", s.deleteArtifact)
	s.registerAPI("POST "+prefix+"/artifacts/recheck", s.recheckArtifacts)
	s.registerAPI("GET "+prefix+"/import-sources", s.listImportSources)
	s.registerAPI("GET "+prefix+"/sources", s.listLibrarySources)
	s.registerAPI("POST "+prefix+"/sources", s.createLibrarySource)
	s.registerAPI("GET "+prefix+"/sources/{id}", s.getLibrarySource)
	s.registerAPI("PUT "+prefix+"/sources/{id}", s.updateLibrarySource)
	s.registerAPI("DELETE "+prefix+"/sources/{id}", s.deleteLibrarySource)
	s.registerAPI("POST "+prefix+"/sources/{id}/scans", s.createSourceScan)
	s.registerAPI("GET "+prefix+"/source-scans", s.listSourceScans)
	s.registerAPI("GET "+prefix+"/source-scans/{id}", s.getSourceScan)
	s.registerAPI("POST "+prefix+"/source-scans/{id}/commit", s.commitSourceScan)
	s.registerAPI("POST "+prefix+"/imports/preview", s.previewImport)
	s.registerAPI("GET "+prefix+"/hash-sources", s.listHashSources)
	s.registerAPI("GET "+prefix+"/hash-identities/{sha256}", s.resolveHashIdentity)
	s.registerAPI("POST "+prefix+"/hash-packs/preview", s.previewHashPack)
	s.registerAPI("POST "+prefix+"/hash-packs/import", s.importHashPack)
	s.registerAPI("POST "+prefix+"/hash-packs/export", s.exportHashPack)
	s.registerAPI("POST "+prefix+"/imports/commit", s.commitImport)
	s.registerAPI("POST "+prefix+"/imports/roms/preview", s.previewROMImport)
	s.registerAPI("POST "+prefix+"/imports/roms/commit", s.commitROMImport)
	s.registerAPI("GET "+prefix+"/media", s.listMedia)
	s.registerAPI("POST "+prefix+"/media/upload", s.uploadMedia)
	s.registerAPI("POST "+prefix+"/media/recheck", s.recheckMedia)
	s.registerAPI("GET "+prefix+"/media/{id}", s.getMedia)
	s.registerAPI("PUT "+prefix+"/media/{id}", s.updateMedia)
	s.registerAPI("DELETE "+prefix+"/media/{id}", s.deleteMedia)
	s.registerAPI("GET "+prefix+"/media/{id}/content", s.downloadMedia)
	s.registerAPI("GET "+prefix+"/media/{id}/thumbnail", s.downloadMediaThumbnail)
}

func (s *Server) authorize(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api") || publicAPIPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		header := r.Header.Get("Authorization")
		value := ""
		if strings.HasPrefix(header, "Bearer ") {
			value = strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		}
		if s.token != "" && subtle.ConstantTimeCompare([]byte(value), []byte(s.token)) == 1 {
			next.ServeHTTP(w, r)
			return
		}
		if s.token == "" && value == "" {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("WWW-Authenticate", `Bearer realm="kino-library"`)
		writeAPIError(w, http.StatusUnauthorized, "authentication_required", "authentication required")
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), gamepad=(self)")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; img-src 'self' data: blob:; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}
