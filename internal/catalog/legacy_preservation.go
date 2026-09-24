package catalog

import (
	"context"
	"database/sql"
	"errors"
)

var ErrLegacyDataRequiresMigration = errors.New("legacy save references must be migrated before deleting this ROM entry")

// Preserve referenced records when deleting an edition.
// The query fragment is internal, never user input.
func protectLegacyEditions(ctx context.Context, tx *sql.Tx, editions string, id string) error {
	for _, query := range []string{
		`SELECT COUNT(*) FROM save_stream_editions WHERE edition_id IN (` + editions + `)`,
		`SELECT COUNT(*) FROM save_bindings WHERE edition_id IN (` + editions + `)`,
		`SELECT COUNT(*) FROM save_streams WHERE owner_type='edition' AND owner_key IN (` + editions + `)`,
	} {
		var count int
		if err := tx.QueryRowContext(ctx, query, id).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			return ErrLegacyDataRequiresMigration
		}
	}
	return nil
}
