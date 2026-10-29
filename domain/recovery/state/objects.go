// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	domainrecovery "github.com/juju/juju/domain/recovery"
	"github.com/juju/juju/internal/errors"
)

// GetObjectMetadataPaths retrieves every metadata row's blob hash, with its
// path where one is recorded. Metadata rows without a path remain reachable
// by hash (the metadata view left-joins paths), so they are included with an
// empty path: the completeness check must cover them too.
func (st *State) GetObjectMetadataPaths(ctx context.Context) ([]domainrecovery.ObjectMetadataPath, error) {
	type path struct {
		Path   string `db:"path"`
		SHA384 string `db:"sha_384"`
	}
	stmt, err := st.Prepare(`
SELECT COALESCE(osmp.path, '') AS &path.path, osm.sha_384 AS &path.sha_384
FROM object_store_metadata AS osm
LEFT JOIN object_store_metadata_path AS osmp ON osmp.metadata_uuid = osm.uuid
ORDER BY osmp.path`, path{})
	if err != nil {
		return nil, errors.Capture(err)
	}
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	var rows []path
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).GetAll(&rows)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	result := make([]domainrecovery.ObjectMetadataPath, len(rows))
	for i, row := range rows {
		result[i] = domainrecovery.ObjectMetadataPath{Path: row.Path, SHA384: row.SHA384}
	}
	return result, nil
}
