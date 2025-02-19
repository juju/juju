// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	charmtesting "github.com/juju/juju/core/charm/testing"
	storagetesting "github.com/juju/juju/core/storage/testing"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/internal/errors"
)

// TestCreateApplicationWithResources tests creation of an application with
// specified resources.
// It verifies that the charm_resource table is populated, alongside the
// resource and application_resource table with datas from charm and arguments.
func (s *applicationStateSuite) TestCreateApplicationWithStorage(c *gc.C) {
	chStorage := []charm.Storage{{
		Name: "database",
		Type: "block",
	}, {
		Name: "logs",
		Type: "filesystem",
	}}
	addStorageArgs := []application.AddApplicationStorageArg{
		{
			Name:  "database",
			Pool:  "ebs",
			Size:  10,
			Count: 2,
		},
		{
			Name:  "logs",
			Pool:  "rootfs",
			Size:  20,
			Count: 1,
		},
	}
	ctx := context.Background()

	appUUID, err := s.state.CreateApplication(ctx, "666", s.addApplicationArgForStorage(c, "666",
		chStorage, addStorageArgs), nil)
	c.Assert(err, jc.ErrorIsNil)

	var charmUUID string
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
SELECT charm_uuid
FROM application
WHERE name=?`, "666").Scan(&charmUUID)
	})
	c.Assert(err, jc.ErrorIsNil)
	var (
		foundCharmStorage []charm.Storage
		foundAppStorage   []application.AddApplicationStorageArg
	)

	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
SELECT cs.name, csk.kind
FROM charm_storage cs
JOIN charm_storage_kind csk ON csk.id=cs.storage_kind_id
WHERE charm_uuid=?`, charmUUID)
		if err != nil {
			return errors.Capture(err)
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var stor charm.Storage
			if err := rows.Scan(&stor.Name, &stor.Type); err != nil {
				return errors.Capture(err)
			}
			foundCharmStorage = append(foundCharmStorage, stor)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
SELECT storage_name, storage_pool, size_mib, count
FROM application_storage_directive
WHERE application_uuid = ? AND charm_uuid = ?`, appUUID, charmUUID)
		if err != nil {
			return errors.Capture(err)
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var stor application.AddApplicationStorageArg
			if err := rows.Scan(&stor.Name, &stor.Pool, &stor.Size, &stor.Count); err != nil {
				return errors.Capture(err)
			}
			foundAppStorage = append(foundAppStorage, stor)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(foundCharmStorage, jc.SameContents, chStorage)
	c.Assert(foundAppStorage, jc.SameContents, addStorageArgs)
}

func (s *applicationStateSuite) TestCreateApplicationWithUnrecognisedStorage(c *gc.C) {
	chStorage := []charm.Storage{{
		Name: "database",
		Type: "block",
	}}
	addStorageArgs := []application.AddApplicationStorageArg{{
		Name:  "foo",
		Pool:  "rootfs",
		Size:  20,
		Count: 1,
	}}
	ctx := context.Background()

	_, err := s.state.CreateApplication(ctx, "666", s.addApplicationArgForStorage(c, "666",
		chStorage, addStorageArgs), nil)
	c.Assert(err, gc.ErrorMatches, `.*storage \["foo"\] is not supported`)
}

func (s *applicationStateSuite) TestCreateApplicationWithStorageButCharmHasNone(c *gc.C) {
	addStorageArgs := []application.AddApplicationStorageArg{{
		Name:  "foo",
		Pool:  "rootfs",
		Size:  20,
		Count: 1,
	}}
	ctx := context.Background()

	_, err := s.state.CreateApplication(ctx, "666", s.addApplicationArgForStorage(c, "666",
		[]charm.Storage{}, addStorageArgs), nil)
	c.Assert(err, gc.ErrorMatches, `.*storage \["foo"\] is not supported`)
}

type charmStorageArg struct {
	name     string
	kind     int
	min, max int
	readOnly bool
	location string
}

func (s *applicationStateSuite) insertCharmWithStorage(c *gc.C, stor ...charmStorageArg) string {
	uuid := charmtesting.GenCharmID(c).String()

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		var err error
		if _, err = insertCharmMetadata(ctx, c, tx, uuid); err != nil {
			return errors.Capture(err)
		}

		for _, arg := range stor {
			_, err = tx.ExecContext(ctx, `
INSERT INTO charm_storage (
    charm_uuid,
    name,
    storage_kind_id,
    read_only,
    count_min,
    count_max,
    location
) VALUES
    (?, ?, ?, ?, ?, ?, ?);`,
				uuid, arg.name, arg.kind, arg.readOnly, arg.min, arg.max, arg.location)
			if err != nil {
				return errors.Capture(err)
			}
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	return uuid
}

var (
	filesystemStorage = charmStorageArg{
		name:     "pgdata",
		kind:     1,
		min:      1,
		max:      2,
		readOnly: true,
		location: "/tmp",
	}
)

func (s *applicationStateSuite) TestGetStorageUUIDByID(c *gc.C) {
	ctx := context.Background()

	charmUUID := s.insertCharmWithStorage(c, filesystemStorage)
	uuid := storagetesting.GenStorageUUID(c)

	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO storage_instance(uuid, charm_uuid, storage_name, storage_id, life_id, storage_pool, requested_size_mib)
VALUES (?, ?, ?, ?, ?, ?, ?)`, uuid, charmUUID, "pgdata", "pgdata/0", 0, "pool", 666)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.state.GetStorageUUIDByID(ctx, "pgdata/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.Equals, uuid)
}

func (s *applicationStateSuite) TestGetStorageUUIDByIDNotFound(c *gc.C) {
	ctx := context.Background()

	_, err := s.state.GetStorageUUIDByID(ctx, "pgdata/0")
	c.Assert(err, jc.ErrorIs, storageerrors.StorageNotFound)
}
