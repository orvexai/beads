//go:build cgo

package embeddeddolt

import (
	"context"
	"database/sql"

	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/storage/schema"
)

var _ storage.SchemaMigrationVersionReader = (*EmbeddedDoltStore)(nil)

// SchemaMigrationVersions reports the main and clone-local migration cursors
// from one read transaction, so diagnostics see a consistent database state.
func (s *EmbeddedDoltStore) SchemaMigrationVersions(ctx context.Context) (storage.SchemaMigrationVersions, error) {
	var versions storage.SchemaMigrationVersions
	err := s.withConn(ctx, false, func(tx *sql.Tx) error {
		var err error
		versions.Main, err = schema.CurrentVersion(ctx, tx)
		if err != nil {
			return err
		}
		versions.CloneLocal, err = schema.CurrentIgnoredVersion(ctx, tx)
		return err
	})
	return versions, err
}
