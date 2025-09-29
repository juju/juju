// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	stdtesting "testing"

	"github.com/juju/clock"
	"github.com/juju/tc"

	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/application/internal"
	schematesting "github.com/juju/juju/domain/schema/testing"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	storagetesting "github.com/juju/juju/domain/storage/testing"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type baseStorageSuite struct {
	baseSuite

	state *State
}

// storageSuite is a suite for testing generic storage related state interfaces.
// The primary means for testing state funcs not realted to applications
// themselves.
type storageSuite struct {
	schematesting.ModelSuite
	storageHelper
}

func TestStorageSuite(t *stdtesting.T) {
	suite := &storageSuite{}
	suite.storageHelper.dbGetter = &suite.ModelSuite
	tc.Run(t, suite)
}

func (s *baseStorageSuite) SetUpTest(c *tc.C) {
	s.baseSuite.SetUpTest(c)

	s.state = NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
}

func (s *baseStorageSuite) TestGetStorageUUIDByID(c *tc.C) {
	ctx := c.Context()

	uuid := storagetesting.GenStorageInstanceUUID(c)

	poolUUID := storagetesting.GenStoragePoolUUID(c)
	_, err := s.DB().Exec(`
INSERT INTO storage_pool (uuid, name, type) VALUES (?, ?, ?)`,
		poolUUID, "rootfs", "rootfs")
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.DB().Exec(`
INSERT INTO storage_instance(uuid, charm_name, storage_name, storage_id,
                             storage_kind_id, life_id, storage_pool_uuid,
                             requested_size_mib)
VALUES (?, ?, ?, ?, 1, ?, ?, ?)`,
		uuid.String(),
		"mycharm",
		"pgdata",
		"pgdata/0",
		0,
		poolUUID,
		666,
	)
	c.Assert(err, tc.ErrorIsNil)

	result, err := s.state.GetStorageUUIDByID(ctx, "pgdata/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.Equals, uuid)
}

func (s *baseStorageSuite) TestGetStorageUUIDByIDNotFound(c *tc.C) {
	ctx := c.Context()

	_, err := s.state.GetStorageUUIDByID(ctx, "pgdata/0")
	c.Assert(err, tc.ErrorIs, storageerrors.StorageNotFound)
}

func (s *applicationStateSuite) createStoragePool(
	c *tc.C, name, providerType string,
) domainstorage.StoragePoolUUID {
	poolUUID := storagetesting.GenStoragePoolUUID(c)
	_, err := s.DB().Exec(`
INSERT INTO storage_pool (uuid, name, type) VALUES (?, ?, ?)
`,
		poolUUID, name, providerType,
	)
	c.Assert(err, tc.ErrorIsNil)
	return poolUUID
}

// TestCreateApplicationWithResources tests creation of an application with
// specified resources.
// It verifies that the charm_resource table is populated, alongside the
// resource and application_resource table with datas from charm and arguments.
func (s *applicationStateSuite) TestCreateApplicationWithStorage(c *tc.C) {
	ctx := c.Context()
	ebsPoolUUID := s.createStoragePool(c, "ebs", "ebs")
	rootFsPoolUUID := s.createStoragePool(c, "rootfs", "rootfs")
	fastPoolUUID := s.createStoragePool(c, "fast", "ebs")
	chStorage := []charm.Storage{{
		Name: "database",
		Type: "block",
	}, {
		Name: "logs",
		Type: "filesystem",
	}, {
		Name: "cache",
		Type: "block",
	}}
	directives := []application.CreateApplicationStorageDirectiveArg{
		{
			Name:     "database",
			PoolUUID: ebsPoolUUID,
			Size:     10,
			Count:    2,
		},
		{
			Name:     "logs",
			PoolUUID: rootFsPoolUUID,
			Size:     20,
			Count:    1,
		},
		{
			Name:     "cache",
			PoolUUID: fastPoolUUID,
			Size:     30,
			Count:    1,
		},
	}

	appUUID, _, err := s.state.CreateIAASApplication(ctx, "666", s.addIAASApplicationArgForStorage(c, "666",
		chStorage, directives), nil)
	c.Assert(err, tc.ErrorIsNil)

	var charmUUID string
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
SELECT charm_uuid
FROM application
WHERE name=?`, "666").Scan(&charmUUID)
	})
	c.Assert(err, tc.ErrorIsNil)
	var (
		foundCharmStorage []charm.Storage
		foundAppStorage   []application.CreateApplicationStorageDirectiveArg
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
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
SELECT storage_name, storage_pool_uuid, size_mib, count
FROM   application_storage_directive
WHERE application_uuid = ? AND charm_uuid = ?`, appUUID, charmUUID)
		if err != nil {
			return errors.Capture(err)
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			stor := application.CreateApplicationStorageDirectiveArg{}
			if err := rows.Scan(&stor.Name, &stor.PoolUUID, &stor.Size, &stor.Count); err != nil {
				return errors.Capture(err)
			}
			foundAppStorage = append(foundAppStorage, stor)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(foundCharmStorage, tc.SameContents, chStorage)
	c.Check(foundAppStorage, tc.SameContents, directives)
}

// TestGetProviderTypeOfPoolNotFound tests that trying to get the provider type
// for a pool that doesn't exist returns the caller an error satisfying
// [storageerrors.PoolNotFoundError].
func (s *storageSuite) TestGetProviderTypeForPoolNotFound(c *tc.C) {
	poolUUID, err := domainstorage.NewStoragePoolUUID()
	c.Assert(err, tc.ErrorIsNil)
	st := NewState(
		s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c),
	)

	_, err = st.GetProviderTypeForPool(c.Context(), poolUUID)
	c.Check(err, tc.ErrorIs, storageerrors.PoolNotFoundError)
}

// TestGetProviderTypeOfPool checks that the provider type of a storage pool
// is correctly returned.
func (s *storageSuite) TestGetProviderTypeForPool(c *tc.C) {
	poolUUID := s.newStoragePool(c, "test-pool", "ptype")
	st := NewState(
		s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c),
	)

	pType, err := st.GetProviderTypeForPool(c.Context(), poolUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(pType, tc.Equals, "ptype")
}

// TestGetModelStoragePoolsWithModelDefaults tests getting model default storage
// pools when only the model defaults have been set via model config.
func (s *storageSuite) TestGetModelStoragePoolsWithModelConfig(c *tc.C) {
	poolUUID := s.createStoragePool(c, "test-pool", "ptype")

	st := NewState(
		s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c),
	)
	db := s.ModelSuite.DB()

	res, err := st.GetModelStoragePools(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, internal.ModelStoragePools{})

	_, err = db.Exec(
		"INSERT INTO model_config(key, value) VALUES (?, ?)",
		application.StorageDefaultBlockSourceKey, "test-pool",
	)
	c.Assert(err, tc.ErrorIsNil)
	res, err = st.GetModelStoragePools(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, internal.ModelStoragePools{
		BlockDevicePoolUUID: &poolUUID,
	})

	_, err = db.Exec(
		"INSERT INTO model_config(key, value) VALUES (?, ?)",
		application.StorageDefaultFilesystemSourceKey,
		"test-pool",
	)
	c.Assert(err, tc.ErrorIsNil)
	res, err = st.GetModelStoragePools(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, internal.ModelStoragePools{
		BlockDevicePoolUUID: &poolUUID,
		FilesystemPoolUUID:  &poolUUID,
	})

	_, err = db.Exec(
		"UPDATE model_config SET value = ? WHERE key = ?",
		"", application.StorageDefaultBlockSourceKey,
	)
	c.Assert(err, tc.ErrorIsNil)
	res, err = st.GetModelStoragePools(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, internal.ModelStoragePools{
		FilesystemPoolUUID: &poolUUID,
	})
}

// TestGetModelStoragePoolsWithModelDefaults tests getting model default storage
// pools when only the model defaults have been set on the tables.
func (s *storageSuite) TestGetModelStoragePoolsWithModelDefaults(c *tc.C) {
	poolUUID := s.createStoragePool(c, "test-pool", "ptype")

	st := NewState(
		s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c),
	)
	db := s.DB()

	res, err := st.GetModelStoragePools(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, internal.ModelStoragePools{})

	_, err = db.Exec(
		`
INSERT INTO model_storage_pool(storage_kind_id, storage_pool_uuid)
VALUES (?, ?)
`,
		int(domainstorage.StorageKindBlock),
		poolUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	res, err = st.GetModelStoragePools(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, internal.ModelStoragePools{
		BlockDevicePoolUUID: &poolUUID,
	})

	_, err = db.Exec(
		`
INSERT INTO model_storage_pool(storage_kind_id, storage_pool_uuid)
VALUES (?, ?)
`,
		int(domainstorage.StorageKindFilesystem),
		poolUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)
	res, err = st.GetModelStoragePools(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, internal.ModelStoragePools{
		BlockDevicePoolUUID: &poolUUID,
		FilesystemPoolUUID:  &poolUUID,
	})

	_, err = db.Exec(
		"DELETE FROM model_storage_pool WHERE storage_kind_id = ?",
		int(domainstorage.StorageKindBlock),
	)
	c.Assert(err, tc.ErrorIsNil)
	res, err = st.GetModelStoragePools(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, internal.ModelStoragePools{
		FilesystemPoolUUID: &poolUUID,
	})
}

// TestGetModelStoragePoolsMix tests getting model default storage pools from
// a combination of model defaults and model config.
func (s *storageSuite) TestGetModelStoragePoolsMix(c *tc.C) {
	poolUUID1 := s.createStoragePool(c, "test-pool1", "ptype")
	poolUUID2 := s.createStoragePool(c, "test-pool2", "ptype")

	st := NewState(
		s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c),
	)
	db := s.DB()
	_, err := db.Exec(
		"INSERT INTO model_config(key, value) VALUES (?, ?)",
		application.StorageDefaultBlockSourceKey, "test-pool1",
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = db.Exec(
		`
INSERT INTO model_storage_pool(storage_kind_id, storage_pool_uuid)
VALUES (?, ?)
`,
		int(domainstorage.StorageKindFilesystem),
		poolUUID2.String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	res, err := st.GetModelStoragePools(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, internal.ModelStoragePools{
		BlockDevicePoolUUID: &poolUUID1,
		FilesystemPoolUUID:  &poolUUID2,
	})

	_, err = db.Exec(
		"INSERT INTO model_config(key, value) VALUES (?, ?)",
		application.StorageDefaultFilesystemSourceKey, "test-pool1",
	)
	c.Assert(err, tc.ErrorIsNil)

	res, err = st.GetModelStoragePools(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, internal.ModelStoragePools{
		BlockDevicePoolUUID: &poolUUID1,
		FilesystemPoolUUID:  &poolUUID1,
	})
}
