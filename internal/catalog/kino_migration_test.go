package catalog

import (
	"path/filepath"
	"testing"
)

func TestKinoRenamePreservesExistingSourceAndScan(t *testing.T) {
	path := filepath.Join(t.TempDir(), "existing.db")
	all := schemaMigrations
	schemaMigrations = all[:28]
	legacy, err := Open(path)
	schemaMigrations = all
	if err != nil {
		t.Fatal(err)
	}
	defer legacy.Close()
	db := legacy.db
	_, err = db.Exec(`PRAGMA user_version=28;
 INSERT INTO library_sources(id,name,kind,source_adapter_id,root_path,metadata_path,platform,rom_storage_policy,media_storage_policy,created_at,updated_at)
 VALUES('saved-source','Existing library','varkiv','builtin-source-varkiv','/original/library','library-manifest.json','','reference','reference','before','before');
 INSERT INTO source_scans(id,source_id,status,requested_at,candidate_count,preview_token_hash,failure_detail)
 VALUES('saved-scan','saved-source','ready','before',17,'original-token','original-detail');`)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		store, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		var kind, adapter, root, token, detail string
		var count, version int
		err = store.db.QueryRow(`SELECT s.kind,s.source_adapter_id,s.root_path,c.candidate_count,c.preview_token_hash,c.failure_detail FROM library_sources s JOIN source_scans c ON c.source_id=s.id WHERE s.id='saved-source'`).Scan(&kind, &adapter, &root, &count, &token, &detail)
		if err != nil {
			t.Fatal(err)
		}
		if kind != "kino" || adapter != "builtin-source-kino" || root != "/original/library" || count != 17 || token != "original-token" || detail != "original-detail" {
			t.Fatalf("changed record: %q %q %q %d %q %q", kind, adapter, root, count, token, detail)
		}
		if err = store.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 29 {
			t.Fatalf("version=%d err=%v", version, err)
		}
		rows, err := store.db.Query(`PRAGMA foreign_key_check`)
		if err != nil {
			t.Fatal(err)
		}
		if rows.Next() {
			t.Fatal("foreign key violation")
		}
		rows.Close()
		store.Close()
	}
}
