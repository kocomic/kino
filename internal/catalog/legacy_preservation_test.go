package catalog

import (
	"context"
	"errors"
	"testing"
)

func TestLegacySaveAssociationsSurviveROMDeletionAttempts(t *testing.T) {
	for _, kind := range []string{"link", "owner"} {
		t.Run(kind, func(t *testing.T) {
			s := testStore(t)
			ctx := context.Background()
			g, err := s.CreateGame(ctx, NewGame{DefaultTitle: "Legacy", Platform: "gba"})
			if err != nil {
				t.Fatal(err)
			}
			e, err := s.AddEdition(ctx, NewEdition{GameID: g.ID, DefaultTitle: "Legacy", EditionType: "original"})
			if err != nil {
				t.Fatal(err)
			}
			ownerType, ownerKey := "container", "legacy"
			if kind == "owner" {
				ownerType, ownerKey = "edition", e.ID
			}
			_, err = s.db.Exec(`INSERT INTO save_streams(id,namespace,owner_type,owner_key,created_at,updated_at) VALUES('legacy','legacy',?,?, 'old','old')`, ownerType, ownerKey)
			if err != nil {
				t.Fatal(err)
			}
			if kind == "link" {
				_, err = s.db.Exec(`INSERT INTO save_stream_editions(stream_id,edition_id,created_at) VALUES('legacy',?,'old')`, e.ID)
				if err != nil {
					t.Fatal(err)
				}
			}
			for _, remove := range []func() error{func() error { return s.DeleteEdition(ctx, e.ID) }, func() error { return s.DeleteGame(ctx, g.ID) }} {
				if err = remove(); !errors.Is(err, ErrLegacyDataRequiresMigration) {
					t.Fatalf("delete legacy association: %v", err)
				}
			}
			got, err := s.GetGame(ctx, g.ID, "")
			if err != nil || len(got.Editions) != 1 {
				t.Fatalf("legacy game altered: %#v %v", got, err)
			}
		})
	}
}
