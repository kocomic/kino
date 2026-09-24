package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"

	"path/filepath"
	"sort"
	"strings"

	"time"

	"kino/internal/buildinfo"

	"kino/internal/catalog"

	"kino/internal/exporter"
	"kino/internal/hashpack"
	"kino/internal/importer"
	"kino/internal/platforms"
	"kino/internal/scanner"
	"kino/internal/server"
	"kino/internal/statebackup"
)

func main() {
	log.SetFlags(0)
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "serve":
		err = serve(os.Args[2:])
	case "scan":
		err = scan(os.Args[2:])
	case "import-pegasus":
		err = importPegasus(os.Args[2:])
	case "import-esde":
		err = importESDE(os.Args[2:])
	case "import-kino":
		err = importKino(os.Args[2:])
	case "export-pegasus":
		err = exportPegasus(os.Args[2:])
	case "export-esde":
		err = exportESDE(os.Args[2:])
	case "hash-pack":
		err = hashPackCommand(os.Args[2:])
	case "db-check":
		err = dbCheck(os.Args[2:])
	case "backup":
		err = backupDatabase(os.Args[2:])
	case "restore-db":
		err = restoreDatabase(os.Args[2:])
	case "backup-state":
		err = backupState(os.Args[2:])
	case "check-state":
		err = checkState(os.Args[2:])
	case "restore-state":
		err = restoreState(os.Args[2:])
	case "platforms":
		err = listPlatforms(os.Args[2:])
	case "version", "--version", "-version", "-v":
		err = versionCommand(os.Args[2:], os.Stdout)
	case "help", "-h", "--help":
		usage()
	default:
		usage()
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		log.Fatal("error: ", sanitizeCommandError(err, os.Args[2:]))
	}
}

var privateCLIFlags = map[string]bool{
	"--binary": true, "--code": true, "--config": true, "--db": true,
	"--driver-root": true, "--from": true, "--library": true, "--name": true, "--out": true,
	"--path": true, "--profile": true, "--rom-root": true, "--root": true,
	"--server": true, "--source": true, "--state": true, "--token": true,
	"--user": true, "--web-emulator-assets": true, "--web-emulator-directory": true,
	"--web-netplay-emulator-assets": true, "--web-netplay-emulator-directory": true,
	"--web-netplay-signal-upstream": true, "--web-netplay-ice-servers": true,
}

// sanitizeCommandError prevents command arguments from being copied into logs.
// The command still reports a useful operation-level error, but explicit host
// paths, pairing secrets, server origins, account names, and device metadata are
// represented only by a stable placeholder.
func sanitizeCommandError(source error, args []string) error {
	if source == nil {
		return nil
	}
	replacements := map[string]struct{}{}
	for index := 0; index < len(args); index++ {
		flagName, value, inline := strings.Cut(args[index], "=")
		if !privateCLIFlags[flagName] {
			continue
		}
		if !inline && index+1 < len(args) {
			index++
			value = args[index]
		}
		addPrivateCLIValue(replacements, value)
	}
	addPrivateCLIValue(replacements, os.Getenv("GAME_LIBRARY_TOKEN"))
	if workingDirectory, err := os.Getwd(); err == nil {
		addPrivateCLIValue(replacements, workingDirectory)
	}
	values := make([]string, 0, len(replacements))
	for value := range replacements {
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool { return len(values[i]) > len(values[j]) })
	message := source.Error()
	for _, value := range values {
		if strings.Contains(message, value) {
			return errors.New("operation failed; private command details were <redacted>")
		}
	}
	return errors.New(message)
}

func addPrivateCLIValue(values map[string]struct{}, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	values[value] = struct{}{}
	if _, pathValue, ok := strings.Cut(value, "="); ok && strings.TrimSpace(pathValue) != "" {
		values[strings.TrimSpace(pathValue)] = struct{}{}
		value = strings.TrimSpace(pathValue)
	}
	if absolute, err := filepath.Abs(value); err == nil {
		values[filepath.Clean(absolute)] = struct{}{}
	}
}

func usage() {
	fmt.Print(`Kino - self-hosted personal game library

Usage:
  kino serve           --db FILE --library DIR [--state DIR] [--addr 127.0.0.1:8080] [--token SECRET]
  kino scan            --db FILE --library DIR --source DIR --platform SLUG
  kino import-pegasus  --db FILE --library DIR --source FILE_OR_LIBRARY_RELATIVE --platform SLUG [--content-root LIBRARY_RELATIVE_DIR] [--locale zh-CN]
  kino import-esde     --db FILE --library DIR --source FILE_OR_LIBRARY_RELATIVE --platform SLUG [--content-root LIBRARY_RELATIVE_DIR] [--locale zh-CN]
  kino import-kino --db FILE --library DIR --source FILE_OR_LIBRARY_RELATIVE
  kino export-pegasus  --db FILE --out DIR --allow-host-paths [--locale zh-CN]
  kino export-esde     --db FILE --out DIR --allow-host-paths [--locale zh-CN]
  kino hash-pack export  --db FILE --out NEW_FILE --source-id ID --name NAME --license LICENSE --release VERSION [--publisher NAME]
  kino hash-pack preview --db FILE --from FILE [--json]
  kino hash-pack import  --db FILE --from FILE [--accept-conflicts]
  kino db-check        --db FILE
  kino backup          --db FILE --out FILE
  kino restore-db      --from BACKUP --out NEW_DATABASE
  kino backup-state    --db FILE --state DIR --out NEW_BACKUP_DIR
  kino check-state     --from BACKUP_DIR
  kino restore-state   --from BACKUP_DIR --out NEW_RESTORE_ROOT
  kino platforms
  kino version         [--json]
`)
}

func hashPackCommand(args []string) error {
	if len(args) == 0 {
		return errors.New("hash-pack requires export, preview, or import")
	}
	switch args[0] {
	case "export":
		return exportHashPack(args[1:])
	case "preview":
		return previewHashPack(args[1:])
	case "import":
		return importHashPack(args[1:])
	default:
		return fmt.Errorf("unknown hash-pack command %q", args[0])
	}
}

func readHashPackFile(path string) ([]byte, hashpack.Pack, string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, hashpack.Pack{}, "", err
	}
	info, err := os.Lstat(absolute)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, hashpack.Pack{}, "", errors.New("hash pack must be an existing exact regular file")
	}
	if info.Size() < 1 || info.Size() > hashpack.MaxPackBytes {
		return nil, hashpack.Pack{}, "", fmt.Errorf("hash pack must contain between 1 and %d bytes", hashpack.MaxPackBytes)
	}
	data, err := os.ReadFile(absolute)
	if err != nil {
		return nil, hashpack.Pack{}, "", err
	}
	pack, err := hashpack.Decode(data)
	if err != nil {
		return nil, hashpack.Pack{}, "", err
	}
	return data, pack, hashpack.Digest(data), nil
}

func exportHashPack(args []string) error {
	fs := flag.NewFlagSet("hash-pack export", flag.ContinueOnError)
	dbPath := fs.String("db", "./data/library.db", "SQLite database path")
	out := fs.String("out", "", "brand-new .hashpack output path")
	sourceID := fs.String("source-id", "", "portable source identifier")
	name := fs.String("name", "", "shared data set name")
	publisher := fs.String("publisher", "", "optional publisher")
	license := fs.String("license", "", "metadata license identifier")
	release := fs.String("release", "", "immutable release identifier")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*out) == "" {
		return errors.New("--out is required")
	}
	store, err := openExistingDatabase(*dbPath)
	if err != nil {
		return err
	}
	defer store.Close()
	data, manifest, err := store.ExportHashPack(context.Background(), hashpack.Source{ID: *sourceID, Name: *name, Publisher: *publisher, License: *license}, *release)
	if err != nil {
		return err
	}
	absolute, err := filepath.Abs(*out)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(absolute, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.Remove(absolute)
		}
	}()
	if _, err = file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	complete = true
	fmt.Printf("hash_pack_created=true pack_id=%s records=%d\n", manifest.PackID, manifest.RecordCount)
	return nil
}

func previewHashPack(args []string) error {
	fs := flag.NewFlagSet("hash-pack preview", flag.ContinueOnError)
	dbPath := fs.String("db", "./data/library.db", "SQLite database path")
	from := fs.String("from", "", "existing .hashpack file")
	jsonOutput := fs.Bool("json", false, "write machine-readable preview")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*from) == "" {
		return errors.New("--from is required")
	}
	_, pack, digest, err := readHashPackFile(*from)
	if err != nil {
		return err
	}
	store, err := openExistingDatabase(*dbPath)
	if err != nil {
		return err
	}
	defer store.Close()
	preview, err := store.PreviewHashPack(context.Background(), pack, digest)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return json.NewEncoder(os.Stdout).Encode(preview)
	}
	fmt.Printf("source=%s release=%s records=%d new=%d existing=%d conflicts=%d existing_release=%t release_conflict=%t\n", preview.Source.ID, preview.Release, preview.RecordCount, preview.NewCount, preview.ExistingCount, preview.ConflictCount, preview.ExistingRelease, preview.ReleaseConflict)
	return nil
}

func importHashPack(args []string) error {
	fs := flag.NewFlagSet("hash-pack import", flag.ContinueOnError)
	dbPath := fs.String("db", "./data/library.db", "SQLite database path")
	from := fs.String("from", "", "existing .hashpack file")
	acceptConflicts := fs.Bool("accept-conflicts", false, "retain conflicting source-attributed metadata without overwriting other sources")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*from) == "" {
		return errors.New("--from is required")
	}
	_, pack, digest, err := readHashPackFile(*from)
	if err != nil {
		return err
	}
	store, err := openExistingDatabase(*dbPath)
	if err != nil {
		return err
	}
	defer store.Close()
	preview, err := store.PreviewHashPack(context.Background(), pack, digest)
	if err != nil {
		return err
	}
	if preview.ReleaseConflict {
		return catalog.ErrHashReleaseConflict
	}
	if preview.ConflictCount > 0 && !*acceptConflicts {
		return fmt.Errorf("hash pack has %d metadata conflicts; run preview and pass --accept-conflicts to retain both sources", preview.ConflictCount)
	}
	result, err := store.ImportHashPack(context.Background(), pack, digest)
	if err != nil {
		return err
	}
	fmt.Printf("hash_pack_imported=true source=%s release=%s records=%d existing_release=%t\n", result.Source.ID, result.Release.Version, result.ImportedRecords, result.ExistingRelease)
	return nil
}

func versionCommand(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("version", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	jsonOutput := fs.Bool("json", false, "write a stable machine-readable version identity")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("version does not accept positional arguments")
	}
	if *jsonOutput {
		return json.NewEncoder(stdout).Encode(struct {
			Format             string `json:"format"`
			ApplicationVersion string `json:"application_version"`
		}{Format: "kino-version-v1", ApplicationVersion: buildinfo.Version})
	}
	_, err := fmt.Fprintln(stdout, "Kino", buildinfo.Version)
	return err
}

type baseFlags struct{ db, library string }

func addBase(fs *flag.FlagSet) *baseFlags {
	b := &baseFlags{}
	fs.StringVar(&b.db, "db", "./data/library.db", "SQLite database path")
	fs.StringVar(&b.library, "library", "./library", "library root")
	return b
}
func open(b *baseFlags) (*catalog.Store, error) {
	if err := os.MkdirAll(filepath.Dir(b.db), 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(b.library, 0o755); err != nil {
		return nil, err
	}
	store, err := catalog.Open(b.db)
	if err != nil {
		return nil, err
	}
	if err = server.EnsureDefaults(context.Background(), store); err != nil {
		_ = store.Close()
		return nil, err
	}
	return store, nil
}

func serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	b := addBase(fs)
	addr := fs.String("addr", "127.0.0.1:8080", "listen address")
	state := fs.String("state", "", "mutable state root for saves and generated packages (default: database directory)")
	token := fs.String("token", os.Getenv("GAME_LIBRARY_TOKEN"), "owner API token (or GAME_LIBRARY_TOKEN)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !loopbackAddress(*addr) && *token == "" {
		return fmt.Errorf("--token or GAME_LIBRARY_TOKEN is required for non-loopback address %q", *addr)
	}
	if strings.TrimSpace(*state) == "" {
		*state = filepath.Dir(b.db)
	}
	store, err := open(b)
	if err != nil {
		return err
	}
	defer store.Close()
	app, err := server.New(store, b.library, server.WithToken(*token), server.WithStateRoot(*state))
	if err != nil {
		return err
	}
	srv := &http.Server{Addr: *addr, Handler: app.Handler(), ReadHeaderTimeout: 5 * time.Second}
	openAddr := *addr
	if openAddr == "" {
		openAddr = ":8080"
	}
	if openAddr[0] == ':' {
		openAddr = "localhost" + openAddr
	}
	log.Printf("Kino %s\nlibrary: configured\nstate: configured\ndatabase: configured\nopen: http://%s", buildinfo.Version, openAddr)
	if *token != "" {
		log.Print("authentication: bearer token required")
	}
	return srv.ListenAndServe()
}

func loopbackAddress(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func openExistingDatabase(path string) (*catalog.Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("--db is required")
	}
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("database is not accessible: %w", err)
	}
	return catalog.Open(path)
}

func openExistingDatabaseReadOnly(path string) (*catalog.Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("--db is required")
	}
	store, err := catalog.OpenReadOnly(path)
	if err != nil {
		return nil, fmt.Errorf("database is not accessible for read-only access: %w", err)
	}
	return store, nil
}

func dbCheck(args []string) error {
	fs := flag.NewFlagSet("db-check", flag.ContinueOnError)
	dbPath := fs.String("db", "./data/library.db", "SQLite database path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	store, err := catalog.OpenReadOnly(*dbPath)
	if err != nil {
		return fmt.Errorf("database is not accessible for a read-only check: %w", err)
	}
	defer store.Close()
	version, err := store.SchemaVersion(context.Background())
	if err != nil {
		return err
	}
	if version != catalog.CurrentSchemaVersion {
		return fmt.Errorf("database schema %d is not the current supported schema %d", version, catalog.CurrentSchemaVersion)
	}
	result, err := store.IntegrityCheck(context.Background())
	if err != nil {
		return err
	}
	if result != "ok" {
		return fmt.Errorf("database integrity check returned %q", result)
	}
	violations, err := store.ForeignKeyViolationCount(context.Background())
	if err != nil {
		return err
	}
	if violations != 0 {
		return fmt.Errorf("database foreign-key check returned %d violations", violations)
	}
	if err = store.ValidateRuntimeCatalog(context.Background()); err != nil {
		return err
	}
	fmt.Printf("schema_version=%d supported=%d integrity=%s foreign_keys=ok mode=read-only\n", version, catalog.CurrentSchemaVersion, result)
	return nil
}

func backupDatabase(args []string) error {
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	dbPath := fs.String("db", "./data/library.db", "SQLite database path")
	out := fs.String("out", "", "new backup file path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*out) == "" {
		return errors.New("--out is required")
	}
	store, err := openExistingDatabase(*dbPath)
	if err != nil {
		return err
	}
	defer store.Close()
	if err = store.Backup(context.Background(), *out); err != nil {
		return err
	}
	fmt.Println("backup_created=true")
	return nil
}

func restoreDatabase(args []string) error {
	fs := flag.NewFlagSet("restore-db", flag.ContinueOnError)
	from := fs.String("from", "", "existing SQLite backup file")
	out := fs.String("out", "", "new restored database file; existing paths are never replaced")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*from) == "" || strings.TrimSpace(*out) == "" {
		return errors.New("--from and --out are required")
	}
	version, err := catalog.RestoreDatabaseBackup(context.Background(), *from, *out)
	if err != nil {
		return err
	}
	fmt.Printf("restore_created=true schema_version=%d integrity=ok\n", version)
	return nil
}

func backupState(args []string) error {
	fs := flag.NewFlagSet("backup-state", flag.ContinueOnError)
	dbPath := fs.String("db", "./data/library.db", "SQLite database path")
	state := fs.String("state", "", "service-managed state root")
	out := fs.String("out", "", "brand-new backup directory outside the state root")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*state) == "" || strings.TrimSpace(*out) == "" {
		return errors.New("--state and --out are required")
	}
	report, err := statebackup.Create(context.Background(), *dbPath, *state, *out)
	if err != nil {
		return err
	}
	fmt.Printf("state_backup_created=true format_version=%d schema_version=%d files=%d bytes=%d managed_roms=%d managed_media=%d save_blobs=%d recovery_snapshots=%d\n",
		statebackup.FormatVersion, report.SchemaVersion, report.Files, report.Bytes, report.ManagedArtifacts, report.ManagedMedia, report.SaveBlobs, report.RecoverySnapshots)
	return nil
}

func checkState(args []string) error {
	fs := flag.NewFlagSet("check-state", flag.ContinueOnError)
	from := fs.String("from", "", "existing complete-state backup directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*from) == "" {
		return errors.New("--from is required")
	}
	report, err := statebackup.Check(context.Background(), *from)
	if err != nil {
		return err
	}
	fmt.Printf("state_backup_valid=true format_version=%d schema_version=%d files=%d bytes=%d managed_roms=%d managed_media=%d save_blobs=%d recovery_snapshots=%d\n",
		statebackup.FormatVersion, report.SchemaVersion, report.Files, report.Bytes, report.ManagedArtifacts, report.ManagedMedia, report.SaveBlobs, report.RecoverySnapshots)
	return nil
}

func restoreState(args []string) error {
	fs := flag.NewFlagSet("restore-state", flag.ContinueOnError)
	from := fs.String("from", "", "existing complete-state backup directory")
	out := fs.String("out", "", "brand-new restore root; existing paths are never replaced")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*from) == "" || strings.TrimSpace(*out) == "" {
		return errors.New("--from and --out are required")
	}
	report, err := statebackup.Restore(context.Background(), *from, *out)
	if err != nil {
		return err
	}
	fmt.Printf("state_restore_created=true schema_version=%d files=%d bytes=%d managed_roms=%d managed_media=%d save_blobs=%d recovery_snapshots=%d\n",
		report.SchemaVersion, report.Files, report.Bytes, report.ManagedArtifacts, report.ManagedMedia, report.SaveBlobs, report.RecoverySnapshots)
	return nil
}

func listPlatforms(args []string) error {
	fs := flag.NewFlagSet("platforms", flag.ContinueOnError)
	dbPath := fs.String("db", "", "optional existing SQLite database; includes enabled custom platforms")
	if err := fs.Parse(args); err != nil {
		return err
	}
	items := platforms.All()
	if strings.TrimSpace(*dbPath) != "" {
		store, err := catalog.OpenReadOnly(*dbPath)
		if err != nil {
			return err
		}
		defer store.Close()
		registry, err := store.PlatformRegistry(context.Background())
		if err != nil {
			return err
		}
		items = registry.All()
	}
	fmt.Println("ID\tNAME\tTYPE\tRUNTIME\tES-DE")
	for _, item := range items {
		kind := "custom"
		if item.Builtin {
			kind = "builtin"
		}
		fmt.Printf("%s\t%s\t%s\t%s\t%s\n", item.ID, item.Name, kind, item.Runtime, strings.Join(item.ESDESystems, ","))
	}
	return nil
}

func scan(args []string) error {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	b := addBase(fs)
	source := fs.String("source", "", "directory inside library root")
	platform := fs.String("platform", "", "platform slug")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *source == "" || *platform == "" {
		return fmt.Errorf("--source and --platform are required")
	}
	store, err := open(b)
	if err != nil {
		return err
	}
	defer store.Close()
	ctx := context.Background()
	registry, err := store.PlatformRegistry(ctx)
	if err != nil {
		return err
	}
	r, err := scanner.ScanWithRegistry(ctx, store, b.library, *source, *platform, registry)
	if err == nil {
		fmt.Printf("found=%d imported=%d skipped=%d\n", r.Found, r.Imported, r.Skipped)
	}
	return err
}

func importPegasus(args []string) error {
	fs := flag.NewFlagSet("import-pegasus", flag.ContinueOnError)
	b := addBase(fs)
	source := fs.String("source", "", "metadata.pegasus.txt")
	contentRoot := fs.String("content-root", "", "optional ROM directory inside the library root when metadata is stored separately")
	platform := fs.String("platform", "", "platform slug")
	locale := fs.String("locale", "", "source title locale")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *source == "" || *platform == "" {
		return fmt.Errorf("--source and --platform are required")
	}
	store, err := open(b)
	if err != nil {
		return err
	}
	defer store.Close()
	metadataPath := resolveMetadataSource(b.library, *source)
	ctx := context.Background()
	platformID, err := canonicalPlatformWithStore(ctx, store, *platform)
	if err != nil {
		return err
	}
	r, err := importer.ImportPegasusWithContentRoot(ctx, store, b.library, metadataPath, *contentRoot, platformID, *locale)
	if err == nil {
		fmt.Printf("parsed=%d imported=%d skipped=%d\n", r.Parsed, r.Imported, r.Skipped)
	}
	return err
}
func importESDE(args []string) error {
	fs := flag.NewFlagSet("import-esde", flag.ContinueOnError)
	b := addBase(fs)
	source := fs.String("source", "", "gamelist.xml")
	contentRoot := fs.String("content-root", "", "optional ROM directory inside the library root when metadata is stored separately")
	platform := fs.String("platform", "", "platform slug")
	locale := fs.String("locale", "", "source title locale")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *source == "" || *platform == "" {
		return fmt.Errorf("--source and --platform are required")
	}
	store, err := open(b)
	if err != nil {
		return err
	}
	defer store.Close()
	metadataPath := resolveMetadataSource(b.library, *source)
	ctx := context.Background()
	registry, err := store.PlatformRegistry(ctx)
	if err != nil {
		return err
	}
	platformID := strings.TrimSpace(*platform)
	if preset, ok := registry.Resolve(platformID); ok {
		platformID = preset.ID
	}
	games, err := importer.PreviewESDEWithContentRootAndRuntimeRegistry(b.library, metadataPath, *contentRoot, "", platformID, *locale, registry)
	if err != nil {
		return err
	}
	r, err := importer.Commit(ctx, store, games)
	if err == nil {
		fmt.Printf("parsed=%d imported=%d skipped=%d\n", r.Parsed, r.Imported, r.Skipped)
	}
	return err
}

func importKino(args []string) error {
	fs := flag.NewFlagSet("import-kino", flag.ContinueOnError)
	b := addBase(fs)
	source := fs.String("source", "", "library-manifest.json")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*source) == "" {
		return errors.New("--source is required")
	}
	store, err := open(b)
	if err != nil {
		return err
	}
	defer store.Close()
	metadataPath := resolveMetadataSource(b.library, *source)
	r, err := importer.ImportLibraryManifest(context.Background(), store, b.library, metadataPath)
	if err == nil {
		fmt.Printf("parsed=%d imported=%d skipped=%d\n", r.Parsed, r.Imported, r.Skipped)
	}
	return err
}

// resolveMetadataSource preserves the original explicit/CWD-relative CLI
// behavior when that path exists, then falls back to the path users naturally
// express relative to --library. The importer still performs its exact-file,
// size, symlink, and library-boundary checks.
func resolveMetadataSource(libraryRoot, source string) string {
	source = filepath.Clean(filepath.FromSlash(strings.TrimSpace(source)))
	if filepath.IsAbs(source) {
		return source
	}
	if _, err := os.Lstat(source); err == nil {
		return source
	}
	return filepath.Join(libraryRoot, source)
}

func canonicalPlatform(value string) string {
	value = strings.TrimSpace(value)
	if preset, ok := platforms.Resolve(value); ok {
		return preset.ID
	}
	return value
}

func canonicalPlatformWithStore(ctx context.Context, store *catalog.Store, value string) (string, error) {
	value = strings.TrimSpace(value)
	registry, err := store.PlatformRegistry(ctx)
	if err != nil {
		return "", err
	}
	if preset, ok := registry.Resolve(value); ok {
		return preset.ID, nil
	}
	return value, nil
}
func exportPegasus(args []string) error {
	fs := flag.NewFlagSet("export-pegasus", flag.ContinueOnError)
	b := addBase(fs)
	out := fs.String("out", "./export/pegasus", "output root")
	locale := fs.String("locale", "zh-CN", "display locale")
	allowHostPaths := fs.Bool("allow-host-paths", false, "allow metadata-only output to contain local ROM paths; prefer build-pack for portable exports")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !*allowHostPaths {
		return errors.New("metadata-only export can expose local ROM paths; use build-pack for a portable relative-path package, or pass --allow-host-paths after reviewing the destination")
	}
	store, err := open(b)
	if err != nil {
		return err
	}
	defer store.Close()
	n, err := exporter.ExportPegasus(context.Background(), store, b.library, *out, *locale)
	if err == nil {
		fmt.Printf("exported_editions=%d output_created=true\n", n)
	}
	return err
}
func exportESDE(args []string) error {
	fs := flag.NewFlagSet("export-esde", flag.ContinueOnError)
	b := addBase(fs)
	out := fs.String("out", "./export/esde", "output root")
	locale := fs.String("locale", "zh-CN", "display locale")
	allowHostPaths := fs.Bool("allow-host-paths", false, "allow metadata-only output to contain local ROM paths; prefer build-pack for portable exports")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !*allowHostPaths {
		return errors.New("metadata-only export can expose local ROM paths; use build-pack for a portable relative-path package, or pass --allow-host-paths after reviewing the destination")
	}
	store, err := open(b)
	if err != nil {
		return err
	}
	defer store.Close()
	n, err := exporter.ExportESDE(context.Background(), store, b.library, *out, *locale)
	if err == nil {
		fmt.Printf("exported_artifacts=%d output_created=true\n", n)
	}
	return err
}

func brandEnvironment(key string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return os.Getenv("VARKIV_" + strings.TrimPrefix(key, "KINO_"))
}
