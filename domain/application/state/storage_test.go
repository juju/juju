// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"math"
	stdtesting "testing"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	corestorage "github.com/juju/juju/core/storage"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/application/internal"
	"github.com/juju/juju/domain/life"
	domainnetwork "github.com/juju/juju/domain/network"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

// storageSuite is a suite for testing generic storage related state interfaces.
// The primary means for testing state funcs not realted to applications
// themselves.
type storageSuite struct {
	baseSuite
	storageHelper
}

func TestStorageSuite(t *stdtesting.T) {
	suite := &storageSuite{}
	suite.storageHelper.dbGetter = &suite.ModelSuite
	tc.Run(t, suite)
}

func (s *storageSuite) TestGetStorageUUIDByID(c *tc.C) {
	ctx := c.Context()

	uuid := tc.Must(c, domainstorage.NewStorageInstanceUUID)

	poolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
	_, err := s.ModelSuite.DB().Exec(`
INSERT INTO storage_pool (uuid, name, type) VALUES (?, ?, ?)`,
		poolUUID, "rootfs", "rootfs")
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.ModelSuite.DB().Exec(`
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

	st := NewState(
		s.ModelSuite.TxnRunnerFactory(),
		s.modelUUID,
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
	result, err := st.GetStorageUUIDByID(ctx, "pgdata/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.Equals, uuid)
}

func (s *storageSuite) TestGetStorageUUIDByIDNotFound(c *tc.C) {
	st := NewState(
		s.ModelSuite.TxnRunnerFactory(),
		s.modelUUID,
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
	_, err := st.GetStorageUUIDByID(c.Context(), "pgdata/0")
	c.Assert(err, tc.ErrorIs, storageerrors.StorageInstanceNotFound)
}

func (s *applicationStateSuite) createStoragePool(
	c *tc.C, name, providerType string,
) domainstorage.StoragePoolUUID {
	poolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
	_, err := s.DB().Exec(`
INSERT INTO storage_pool (uuid, name, type) VALUES (?, ?, ?)
`,
		poolUUID, name, providerType,
	)
	c.Assert(err, tc.ErrorIsNil)
	return poolUUID
}

func (s *applicationStateSuite) TestGetApplicationStorageDirectives(c *tc.C) {
	ctx := c.Context()

	ebsPoolUUID := s.createStoragePool(c, "my-ebs", "ebs")
	rootFsPoolUUID := s.createStoragePool(c, "my-rootfs", "rootfs")
	fastPoolUUID := s.createStoragePool(c, "my-fast", "ebs")

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

	directives := []domainstorage.DirectiveArg{
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
			Count:    0,
		},
		{
			Name:     "cache",
			PoolUUID: fastPoolUUID,
			Size:     30,
			Count:    1,
		},
	}

	appName := "test-app-storage"
	appUUID, _, err := s.state.CreateIAASApplication(
		ctx,
		appName,
		s.addIAASApplicationArgForStorage(
			c,
			appName,
			chStorage,
			directives,
		),
		nil,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(appUUID.IsEmpty(), tc.IsFalse)

	foundDirectives, err := s.state.GetApplicationStorageDirectives(
		ctx, appUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(
		foundDirectives,
		tc.DeepEquals,
		[]application.ApplicationStorageDirective{
			{
				Name:     "cache",
				PoolUUID: fastPoolUUID,
				PoolName: "my-fast",
				SizeMiB:  30,
				Count:    1,
			},
			{
				Name:     "database",
				PoolUUID: ebsPoolUUID,
				PoolName: "my-ebs",
				SizeMiB:  10,
				Count:    2,
			},
			{
				Name:     "logs",
				PoolUUID: rootFsPoolUUID,
				PoolName: "my-rootfs",
				SizeMiB:  20,
				Count:    0,
			},
		},
	)
}

func (s *applicationStateSuite) TestGetApplicationStorageDirectivesEmpty(c *tc.C) {
	ctx := c.Context()

	appName := "app-no-storage"
	appUUID, _, err := s.state.CreateIAASApplication(
		ctx,
		appName,
		s.addIAASApplicationArgForStorage(c, appName, nil, nil),
		nil,
	)
	c.Assert(err, tc.ErrorIsNil)

	foundDirectives, err := s.state.GetApplicationStorageDirectives(
		ctx, appUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(foundDirectives, tc.HasLen, 0)

	_, found, err := s.state.GetApplicationStorageDirectiveByName(
		ctx, appUUID.String(), "database",
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(found, tc.IsFalse)
}

func (s *applicationStateSuite) TestGetApplicationStorageDirectivesApplicationNotFound(c *tc.C) {
	ctx := c.Context()

	randomAppUUID := tc.Must(c, coreapplication.NewUUID)

	_, err := s.state.GetApplicationStorageDirectives(
		ctx, randomAppUUID.String(),
	)
	c.Check(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)

	_, _, err = s.state.GetApplicationStorageDirectiveByName(
		ctx, randomAppUUID.String(), "database",
	)
	c.Check(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationStateSuite) TestGetApplicationStorageDirectiveByName(c *tc.C) {
	ctx := c.Context()

	ebsPoolUUID := s.createStoragePool(c, "my-ebs", "ebs")
	rootFsPoolUUID := s.createStoragePool(c, "my-rootfs", "rootfs")

	chStorage := []charm.Storage{
		{
			Name: "database",
			Type: "block",
		},
		{
			Name: "logs",
			Type: "filesystem",
		},
		{
			Name:   "shared-fs",
			Type:   "filesystem",
			Shared: true,
		},
	}
	directives := []domainstorage.DirectiveArg{
		{
			Name:     "database",
			PoolUUID: ebsPoolUUID,
			Size:     1024,
			Count:    2,
		},
		{
			Name:     "logs",
			PoolUUID: rootFsPoolUUID,
			Size:     512,
			Count:    0,
		},
	}

	appName := "test-app-by-name"
	appUUID, _, err := s.state.CreateIAASApplication(
		ctx,
		appName,
		s.addIAASApplicationArgForStorage(c, appName, chStorage, directives),
		nil,
	)
	c.Assert(err, tc.ErrorIsNil)

	// Known directive with non-zero count.
	dbDirective, found, err := s.state.GetApplicationStorageDirectiveByName(
		ctx, appUUID.String(), "database",
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(found, tc.IsTrue)
	c.Check(dbDirective, tc.DeepEquals, application.ApplicationStorageDirective{
		Name:     corestorage.Name("database"),
		PoolUUID: ebsPoolUUID,
		PoolName: "my-ebs",
		SizeMiB:  1024,
		Count:    2,
	})

	// Known directive with zero count preserved.
	logsDirective, found, err := s.state.GetApplicationStorageDirectiveByName(
		ctx, appUUID.String(), "logs",
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(found, tc.IsTrue)
	c.Check(logsDirective, tc.DeepEquals, application.ApplicationStorageDirective{
		Name:     corestorage.Name("logs"),
		PoolUUID: rootFsPoolUUID,
		PoolName: "my-rootfs",
		SizeMiB:  512,
		Count:    0,
	})

	// Unsupported shared storage has no directive row and is ignored.
	_, found, err = s.state.GetApplicationStorageDirectiveByName(
		ctx, appUUID.String(), "shared-fs",
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(found, tc.IsFalse)

	// Unknown storage name.
	_, found, err = s.state.GetApplicationStorageDirectiveByName(
		ctx, appUUID.String(), "unknown",
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(found, tc.IsFalse)
}

func (s *applicationStateSuite) TestGetApplicationStorageDirectivesIgnoresUnitAllocationsAndStaleCharm(c *tc.C) {
	ctx := c.Context()

	appPoolUUID := s.createStoragePool(c, "app-pool", "ebs")
	unitPoolUUID := s.createStoragePool(c, "old-unit-pool", "ebs")
	stalePoolUUID := s.createStoragePool(c, "stale-charm-pool", "ebs")

	chStorage := []charm.Storage{{
		Name: "database",
		Type: "block",
	}}
	directives := []domainstorage.DirectiveArg{{
		Name:     "database",
		PoolUUID: appPoolUUID,
		Size:     4096,
		Count:    2,
	}}

	appName := "app-isolated"
	appUUID, _, err := s.state.CreateIAASApplication(
		ctx,
		appName,
		s.addIAASApplicationArgForStorage(c, appName, chStorage, directives),
		nil,
	)
	c.Assert(err, tc.ErrorIsNil)

	var currentCharmUUID string
	err = s.DB().QueryRowContext(
		ctx, `SELECT charm_uuid FROM application WHERE uuid = ?`, appUUID,
	).Scan(&currentCharmUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Add a unit with a different unit_storage_directive and an allocated
	// storage_instance to ensure reads never consult per-unit rows.
	unitUUID := tc.Must(c, coreunit.NewUUID)
	netNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	storageInstUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	attachUUID := tc.Must(c, domainstorage.NewStorageAttachmentUUID)
	_, err = s.DB().ExecContext(ctx, `INSERT INTO net_node (uuid) VALUES (?)`, netNodeUUID)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.DB().ExecContext(ctx, `
INSERT INTO unit (uuid, name, application_uuid, life_id, charm_uuid, net_node_uuid)
VALUES (?, ?, ?, ?, ?, ?)`,
		unitUUID, "app-isolated/0", appUUID, int(life.Alive), currentCharmUUID, netNodeUUID,
	)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.DB().ExecContext(ctx, `
INSERT INTO unit_storage_directive (unit_uuid, charm_uuid, storage_name, storage_pool_uuid, size_mib, count)
VALUES (?, ?, 'database', ?, 512, 9)`,
		unitUUID, currentCharmUUID, unitPoolUUID,
	)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.DB().ExecContext(ctx, `
INSERT INTO storage_instance (uuid, charm_name, storage_name, storage_kind_id, storage_id, life_id, storage_pool_uuid, requested_size_mib)
VALUES (?, ?, 'database', 0, 'database/0', ?, ?, 512)`,
		storageInstUUID, appName, int(life.Alive), unitPoolUUID,
	)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.DB().ExecContext(ctx, `
INSERT INTO storage_unit_owner (storage_instance_uuid, unit_uuid) VALUES (?, ?)`,
		storageInstUUID, unitUUID,
	)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.DB().ExecContext(ctx, `
INSERT INTO storage_attachment (uuid, storage_instance_uuid, unit_uuid, life_id)
VALUES (?, ?, ?, ?)`,
		attachUUID, storageInstUUID, unitUUID, int(life.Alive),
	)
	c.Assert(err, tc.ErrorIsNil)

	// Also insert a stale application_storage_directive row for another charm
	// revision on the same application.
	otherAppArg := s.addIAASApplicationArgForStorage(c, "other-charm", chStorage, nil)
	otherCharmID, _, err := s.state.AddCharm(
		ctx, otherAppArg.Charm, otherAppArg.CharmDownloadInfo, false,
	)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.DB().ExecContext(ctx, `
INSERT INTO application_storage_directive (application_uuid, charm_uuid, storage_name, storage_pool_uuid, size_mib, count)
VALUES (?, ?, 'database', ?, 99999, 7)`,
		appUUID, otherCharmID, stalePoolUUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	expected := application.ApplicationStorageDirective{
		Name:     "database",
		PoolUUID: appPoolUUID,
		PoolName: "app-pool",
		SizeMiB:  4096,
		Count:    2,
	}

	gotAll, err := s.state.GetApplicationStorageDirectives(
		ctx, appUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotAll, tc.DeepEquals, []application.ApplicationStorageDirective{expected})

	gotOne, found, err := s.state.GetApplicationStorageDirectiveByName(
		ctx, appUUID.String(), "database",
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(found, tc.IsTrue)
	c.Check(gotOne, tc.DeepEquals, expected)

	gotCharmDirectives, err := s.state.GetApplicationCharmStorageDirectives(
		ctx, appUUID,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotCharmDirectives, tc.DeepEquals, []internal.StorageDirective{{
		CharmMetadataName: appName,
		CharmStorageType:  charm.StorageBlock,
		Name:              "database",
		PoolUUID:          appPoolUUID,
		Size:              4096,
		Count:             2,
		MaxCount:          0,
	}})

	// Reading directives must not allocate storage instances, volumes, or
	// filesystems, nor mutate existing storage instance size requests.
	var instCount, volCount, fsCount, requestedSizeMiB int
	err = s.DB().QueryRowContext(
		ctx, `SELECT COUNT(*) FROM storage_instance`,
	).Scan(&instCount)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(instCount, tc.Equals, 1)

	err = s.DB().QueryRowContext(
		ctx, `SELECT requested_size_mib FROM storage_instance WHERE uuid = ?`,
		storageInstUUID,
	).Scan(&requestedSizeMiB)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(requestedSizeMiB, tc.Equals, 512)

	err = s.DB().QueryRowContext(
		ctx, `SELECT COUNT(*) FROM storage_volume`,
	).Scan(&volCount)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(volCount, tc.Equals, 0)

	err = s.DB().QueryRowContext(
		ctx, `SELECT COUNT(*) FROM storage_filesystem`,
	).Scan(&fsCount)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(fsCount, tc.Equals, 0)
}

func (s *applicationStateSuite) TestGetApplicationStorageDirectivesCheckedConversions(c *tc.C) {
	ctx := c.Context()

	poolUUID := s.createStoragePool(c, "my-ebs", "ebs")
	chStorage := []charm.Storage{{
		Name: "database",
		Type: "block",
	}}
	directives := []domainstorage.DirectiveArg{{
		Name:     "database",
		PoolUUID: poolUUID,
		Size:     1024,
		Count:    1,
	}}

	appName := "app-bounds"
	appUUID, _, err := s.state.CreateIAASApplication(
		ctx,
		appName,
		s.addIAASApplicationArgForStorage(c, appName, chStorage, directives),
		nil,
	)
	c.Assert(err, tc.ErrorIsNil)

	cases := []struct {
		name          string
		sizeMiB       int64
		count         int64
		allErrText    string
		byNameErrText string
	}{
		{
			name:          "negative size_mib",
			sizeMiB:       -1,
			count:         1,
			allErrText:    `storage directive "database": negative storage size -1 MiB`,
			byNameErrText: `negative storage size -1 MiB`,
		},
		{
			name:          "negative count",
			sizeMiB:       1024,
			count:         -1,
			allErrText:    `storage directive "database": storage directive count -1 out of uint32 range`,
			byNameErrText: `storage directive count -1 out of uint32 range`,
		},
		{
			name:          "count exceeds MaxUint32",
			sizeMiB:       1024,
			count:         int64(math.MaxUint32) + 1,
			allErrText:    `storage directive "database": .*out of uint32 range`,
			byNameErrText: `.*out of uint32 range`,
		},
	}

	for _, tcCase := range cases {
		_, err := s.DB().ExecContext(ctx, `
UPDATE application_storage_directive
SET    size_mib = ?, count = ?
WHERE  application_uuid = ? AND storage_name = 'database'`,
			tcCase.sizeMiB, tcCase.count, appUUID,
		)
		c.Assert(err, tc.ErrorIsNil)

		_, err = s.state.GetApplicationStorageDirectives(
			ctx, appUUID.String(),
		)
		c.Check(err, tc.ErrorMatches, tcCase.allErrText, tc.Commentf(tcCase.name))

		_, _, err = s.state.GetApplicationStorageDirectiveByName(
			ctx, appUUID.String(), "database",
		)
		c.Check(err, tc.ErrorMatches, tcCase.byNameErrText, tc.Commentf(tcCase.name))
	}
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
	directives := []domainstorage.DirectiveArg{
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
		foundAppStorage   []domainstorage.DirectiveArg
	)

	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		foundCharmStorage = nil

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
		return rows.Err()
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		foundAppStorage = nil

		rows, err := tx.QueryContext(ctx, `
SELECT storage_name, storage_pool_uuid, size_mib, count
FROM   application_storage_directive
WHERE application_uuid = ? AND charm_uuid = ?`, appUUID, charmUUID)
		if err != nil {
			return errors.Capture(err)
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			stor := domainstorage.DirectiveArg{}
			if err := rows.Scan(&stor.Name, &stor.PoolUUID, &stor.Size, &stor.Count); err != nil {
				return errors.Capture(err)
			}
			foundAppStorage = append(foundAppStorage, stor)
		}
		return rows.Err()
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(foundCharmStorage, tc.SameContents, chStorage)
	c.Check(foundAppStorage, tc.SameContents, directives)
}

func (s *applicationStateSuite) TestUpdateApplicationStorageDirectives(c *tc.C) {
	ctx := c.Context()

	poolUUID1 := s.createStoragePool(c, "pool-a", "lxd")
	poolUUID2 := s.createStoragePool(c, "pool-b", "ebs")

	// Setup charm with 2 storages.
	chStorage := []charm.Storage{{
		Name:        "database",
		Type:        "block",
		CountMin:    1,
		CountMax:    3,
		MinimumSize: 10,
	}, {
		Name:        "logs",
		Type:        "filesystem",
		CountMin:    1,
		CountMax:    2,
		MinimumSize: 5,
	}}

	// Setup application with 2 storage directives.
	directives := []domainstorage.DirectiveArg{
		{
			Name:     "database",
			PoolUUID: poolUUID1,
			Size:     10,
			Count:    1,
		},
		{
			Name:     "logs",
			PoolUUID: poolUUID2,
			Size:     20,
			Count:    2,
		},
	}

	// Create application with the above directives and charm storages.
	appUUID, _, err := s.state.CreateIAASApplication(
		ctx,
		"charm-name",
		s.addIAASApplicationArgForStorage(c, "charm-name", chStorage, directives),
		nil,
	)
	c.Assert(err, tc.ErrorIsNil)

	// Get charmUUID for the application.
	var charmUUID string
	err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		charmUUID, err = s.state.getCharmIDByApplicationUUID(c.Context(), tx, appUUID.String())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	// Update storage directives for the application.
	overrides := []domainstorage.DirectiveArg{
		{
			Name:     "database",
			PoolUUID: poolUUID2,
			Size:     99,
			Count:    3,
		},
		{
			Name:     "logs",
			PoolUUID: poolUUID1,
			Size:     5,
			Count:    1,
		},
	}
	err = s.TxnRunner().Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.updateApplicationStorageDirectives(ctx, tx, appUUID, charmUUID, overrides)
	})
	c.Assert(err, tc.ErrorIsNil)

	// Verify that the storage directives have been updated.
	applicationStorageDirectives, err := s.state.GetApplicationCharmStorageDirectives(ctx, appUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(applicationStorageDirectives, tc.SameContents, []internal.StorageDirective{
		{
			CharmMetadataName: "charm-name",
			CharmStorageType:  charm.StorageBlock,
			Name:              "database",
			PoolUUID:          poolUUID2,
			Size:              99,
			Count:             3,
			MaxCount:          3,
		},
		{
			CharmMetadataName: "charm-name",
			CharmStorageType:  charm.StorageFilesystem,
			Name:              "logs",
			PoolUUID:          poolUUID1,
			Size:              5,
			Count:             1,
			MaxCount:          2,
		},
	})
}

func (s *applicationStateSuite) TestUpdateApplicationStorageDirectivesMissingStorage(c *tc.C) {
	ctx := c.Context()

	poolUUID1 := s.createStoragePool(c, "pool-a", "lxd")
	poolUUID2 := s.createStoragePool(c, "pool-b", "ebs")

	// Setup charm with 3 storages.
	chStorage := []charm.Storage{{
		Name:        "database",
		Type:        "block",
		CountMin:    1,
		CountMax:    3,
		MinimumSize: 10,
	}, {
		Name:        "logs",
		Type:        "filesystem",
		CountMin:    1,
		CountMax:    2,
		MinimumSize: 5,
	}, {
		Name:        "cache",
		Type:        "block",
		CountMin:    1,
		CountMax:    1,
		MinimumSize: 1,
	}}

	// Setup application with 2 storage directives.
	directives := []domainstorage.DirectiveArg{
		{
			Name:     "database",
			PoolUUID: poolUUID1,
			Size:     10,
			Count:    1,
		},
		{
			Name:     "logs",
			PoolUUID: poolUUID2,
			Size:     20,
			Count:    2,
		},
	}

	// Create application with the above directives and charm storages.
	appUUID, _, err := s.state.CreateIAASApplication(
		ctx,
		"charm-name",
		s.addIAASApplicationArgForStorage(c, "charm-name", chStorage, directives),
		nil,
	)
	c.Assert(err, tc.ErrorIsNil)

	// Get charmUUID for the application.
	var charmUUID string
	err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		charmUUID, err = s.state.getCharmIDByApplicationUUID(c.Context(), tx, appUUID.String())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	// Attempt to update 3 storage directives for the application, even though the application currently only has 2.
	// This provides the situation where a storage directive is manually removed.
	overrides := []domainstorage.DirectiveArg{
		{
			Name:     "database",
			PoolUUID: poolUUID2,
			Size:     99,
			Count:    3,
		},
		{
			Name:     "logs",
			PoolUUID: poolUUID1,
			Size:     5,
			Count:    1,
		},
		{
			Name:     "cache",
			PoolUUID: poolUUID2,
			Size:     1,
			Count:    1,
		},
	}
	err = s.TxnRunner().Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.updateApplicationStorageDirectives(ctx, tx, appUUID, charmUUID, overrides)
	})
	c.Assert(err, tc.ErrorMatches, `missing storage directive for charm storage "cache"`)
}

// TestGetProviderTypeOfPoolNotFound tests that trying to get the provider type
// for a pool that doesn't exist returns the caller an error satisfying
// [storageerrors.StoragePoolNotFound].
func (s *storageSuite) TestGetProviderTypeForPoolNotFound(c *tc.C) {
	poolUUID, err := domainstorage.NewStoragePoolUUID()
	c.Assert(err, tc.ErrorIsNil)
	st := NewState(
		s.ModelSuite.TxnRunnerFactory(),
		s.modelUUID,
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)

	_, err = st.GetProviderTypeForPool(c.Context(), poolUUID)
	c.Check(err, tc.ErrorIs, storageerrors.StoragePoolNotFound)
}

// TestGetProviderTypeOfPool checks that the provider type of a storage pool
// is correctly returned.
func (s *storageSuite) TestGetProviderTypeForPool(c *tc.C) {
	poolUUID := s.newStoragePool(c, "test-pool", "ptype")
	st := NewState(
		s.ModelSuite.TxnRunnerFactory(),
		s.modelUUID,
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)

	pType, err := st.GetProviderTypeForPool(c.Context(), poolUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(pType, tc.Equals, "ptype")
}

// TestGetModelStoragePoolsWithModelDefaults tests getting model default storage
// pools when only the model defaults have been set via model config.
func (s *storageSuite) TestGetModelStoragePoolsWithModelConfig(c *tc.C) {
	poolUUID := s.storageHelper.newStoragePool(c, "test-pool", "ptype")

	st := NewState(
		s.ModelSuite.TxnRunnerFactory(),
		s.modelUUID,
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
	db := s.ModelSuite.DB()

	res, err := st.GetModelStoragePools(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, internal.ModelStoragePools{})

	_, err = db.Exec(
		"INSERT INTO model_config(key, value) VALUES (?, ?)",
		application.StorageDefaultBlockSourceKey,
		// "test-pool" matches the name pool created above
		"test-pool",
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
		// "test-pool" matches the name pool created above
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
	poolUUID := s.storageHelper.newStoragePool(c, "test-pool1", "ptype")

	st := NewState(
		s.ModelSuite.TxnRunnerFactory(),
		s.modelUUID,
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
	db := s.ModelSuite.DB()

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
	poolUUID1 := s.storageHelper.newStoragePool(c, "test-pool1", "ptype")
	poolUUID2 := s.storageHelper.newStoragePool(c, "test-pool2", "ptype")

	st := NewState(
		s.ModelSuite.TxnRunnerFactory(), s.modelUUID,
		clock.WallClock, loggertesting.WrapCheckLog(c),
	)
	db := s.ModelSuite.DB()
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

// TestGetStorageInstancesForProviderIDsNotFound tests that when no storage
// instances exist in the model using any of the supplied provider ids an empty
// map is returned with no error.
func (s *storageSuite) TestGetStorageInstancesForProviderIDsNotFound(c *tc.C) {
	st := NewState(
		s.ModelSuite.TxnRunnerFactory(),
		s.modelUUID,
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)

	res, err := st.GetStorageInstancesForProviderIDs(
		c.Context(), []string{"providerid1", "providerid2"},
	)

	c.Check(err, tc.ErrorIsNil)
	c.Check(res, tc.HasLen, 0)
}

// TestGetStorageInstancesForNoProviderIDs tests that when supplying no provider
// ids to [State.GetStorageInstancesForProviderIDs] the caller receives an empty
// result back.
func (s *storageSuite) TestGetStorageInstancesForNoProviderIDs(c *tc.C) {
	st := NewState(
		s.ModelSuite.TxnRunnerFactory(),
		s.modelUUID,
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)

	res, err := st.GetStorageInstancesForProviderIDs(
		c.Context(), nil,
	)

	c.Check(err, tc.ErrorIsNil)
	c.Check(res, tc.HasLen, 0)
}

// TestGetStorageInstancesForProviderIDsNotUsingProviderIDs tests that when a
// storage instance has both a filesystem and volume with the same provider id
// only one value is returned.
func (s *storageSuite) TestGetStorageInstancesForProviderIDsNotUsingProviderIDs(c *tc.C) {
	instUUID, fsUUID, vUUID := s.newStorageInstanceFilesystemBackedVolumeWithProviderID(
		c, "st1", "provider1", "provider1",
	)

	st := NewState(
		s.ModelSuite.TxnRunnerFactory(),
		s.modelUUID,
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)

	res, err := st.GetStorageInstancesForProviderIDs(
		c.Context(), []string{
			"provider1",
		},
	)
	c.Check(err, tc.ErrorIsNil)
	mc := tc.NewMultiChecker()
	mc.AddExpr("_.Filesystem.ProvisionScope", tc.Ignore)
	mc.AddExpr("_.Filesystem.Size", tc.Ignore)
	mc.AddExpr("_.Volume.ProvisionScope", tc.Ignore)
	mc.AddExpr("_.Volume.Size", tc.Ignore)
	c.Check(err, tc.ErrorIsNil)
	c.Check(
		res,
		tc.UnorderedMatch[[]domainstorage.StorageInstanceInfoForAttach](mc),
		[]domainstorage.StorageInstanceInfoForAttach{
			{
				StorageInstanceAttachInfo: domainstorage.StorageInstanceAttachInfo{
					UUID:      instUUID,
					CharmName: new("charm"),
					Filesystem: &domainstorage.StorageInstanceAttachFilesystemInfo{
						UUID: fsUUID,
					},
					Volume: &domainstorage.StorageInstanceAttachVolumeInfo{
						UUID: vUUID,
					},
					Kind:             domainstorage.StorageKindFilesystem,
					Life:             life.Alive,
					RequestedSizeMIB: 1024,
					StorageName:      "st1",
				},
			},
		},
	)
}

// TestGetStorageInstancesForProviderIDs tests getting existing storage
// instances that are utilising a provider id via either the instances volume or
// filesystem. We expect to see that this test correctly identifies the right
// storage instances and groups them together for their storage name.
func (s *storageSuite) TestGetStorageInstancesForProviderIDs(c *tc.C) {
	instUUID1, fsUUID1 := s.newStorageInstanceFilesysatemWithProviderID(c, "st1", "provider1")
	instUUID2, fsUUID2 := s.newStorageInstanceFilesysatemWithProviderID(c, "st2", "provider2")
	instUUID3, fsUUID3 := s.newStorageInstanceFilesysatemWithProviderID(c, "st1", "provider3")
	instUUID4, vUUID1 := s.newStorageInstanceVolumeWithProviderID(c, "st3", "provider4")

	st := NewState(
		s.ModelSuite.TxnRunnerFactory(),
		s.modelUUID,
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)

	res, err := st.GetStorageInstancesForProviderIDs(
		c.Context(), []string{
			"provider1",
			"provider2",
			"provider3",
			"provider4",
			"providernotexist",
		},
	)
	mc := tc.NewMultiChecker()
	mc.AddExpr("_.Filesystem.ProvisionScope", tc.Ignore)
	mc.AddExpr("_.Filesystem.Size", tc.Ignore)
	mc.AddExpr("_.Volume.ProvisionScope", tc.Ignore)
	mc.AddExpr("_.Volume.Size", tc.Ignore)
	c.Check(err, tc.ErrorIsNil)
	c.Check(
		res,
		tc.UnorderedMatch[[]domainstorage.StorageInstanceInfoForAttach](mc),
		[]domainstorage.StorageInstanceInfoForAttach{
			{
				StorageInstanceAttachInfo: domainstorage.StorageInstanceAttachInfo{
					UUID:      instUUID1,
					CharmName: new("charm"),
					Filesystem: &domainstorage.StorageInstanceAttachFilesystemInfo{
						UUID: fsUUID1,
					},
					Kind:             domainstorage.StorageKindFilesystem,
					Life:             life.Alive,
					RequestedSizeMIB: 1024,
					StorageName:      "st1",
				},
			},
			{
				StorageInstanceAttachInfo: domainstorage.StorageInstanceAttachInfo{
					UUID:      instUUID3,
					CharmName: new("charm"),
					Filesystem: &domainstorage.StorageInstanceAttachFilesystemInfo{
						UUID: fsUUID3,
					},
					Kind:             domainstorage.StorageKindFilesystem,
					Life:             life.Alive,
					RequestedSizeMIB: 1024,
					StorageName:      "st1",
				},
			},
			{
				StorageInstanceAttachInfo: domainstorage.StorageInstanceAttachInfo{
					UUID:      instUUID2,
					CharmName: new("charm"),
					Filesystem: &domainstorage.StorageInstanceAttachFilesystemInfo{
						UUID: fsUUID2,
					},
					Kind:             domainstorage.StorageKindFilesystem,
					Life:             life.Alive,
					RequestedSizeMIB: 1024,
					StorageName:      "st2",
				},
			},
			{
				StorageInstanceAttachInfo: domainstorage.StorageInstanceAttachInfo{
					UUID:      instUUID4,
					CharmName: new("charm"),
					Volume: &domainstorage.StorageInstanceAttachVolumeInfo{
						UUID: vUUID1,
					},
					Kind:             domainstorage.StorageKindFilesystem,
					Life:             life.Alive,
					RequestedSizeMIB: 1024,
					StorageName:      "st3",
				},
			},
		},
	)
}

// TestGetStorageInstancesForProviderIDSomeStorageOwned tests that when storage
// is requested matching a set of supplied provider ids the caller does not get
// back storage instances that ore owned by a unit in the model should these
// storage instances match in every other respect.
func (s *storageSuite) TestGetStorageInstancesForProviderIDSomeStorageOwned(c *tc.C) {
	// This first storage instance is to be owned by a unit in the model.
	instUUID1, _ := s.newStorageInstanceFilesysatemWithProviderID(c, "st1", "provider1")
	instUUID2, fsUUID2 := s.newStorageInstanceFilesysatemWithProviderID(c, "st2", "provider2")
	instUUID3, fsUUID3 := s.newStorageInstanceFilesysatemWithProviderID(c, "st1", "provider3")
	instUUID4, vUUID1 := s.newStorageInstanceVolumeWithProviderID(c, "st3", "provider4")

	_, unitUUIDs := s.createIAASApplicationWithNUnits(c, "foo", life.Alive, 1)
	unitUUID := unitUUIDs[0]
	s.newStorageUnitOwner(c, instUUID1, unitUUID)

	st := NewState(
		s.ModelSuite.TxnRunnerFactory(),
		s.modelUUID,
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)

	res, err := st.GetStorageInstancesForProviderIDs(
		c.Context(), []string{
			"provider1",
			"provider2",
			"provider3",
			"provider4",
			"providernotexist",
		},
	)

	mc := tc.NewMultiChecker()
	mc.AddExpr("_", tc.SameContents, tc.ExpectedValue)
	mc.AddExpr("_[_].Filesystem.ProvisionScope", tc.Ignore)
	mc.AddExpr("_[_].Filesystem.Size", tc.Ignore)
	mc.AddExpr("_[_].Volume.ProvisionScope", tc.Ignore)
	mc.AddExpr("_[_].Volume.Size", tc.Ignore)
	c.Check(err, tc.ErrorIsNil)
	c.Check(res, mc, []domainstorage.StorageInstanceInfoForAttach{
		{
			StorageInstanceAttachInfo: domainstorage.StorageInstanceAttachInfo{
				UUID:      instUUID3,
				CharmName: new("charm"),
				Filesystem: &domainstorage.StorageInstanceAttachFilesystemInfo{
					UUID: fsUUID3,
				},
				Kind:             domainstorage.StorageKindFilesystem,
				Life:             life.Alive,
				RequestedSizeMIB: 1024,
				StorageName:      "st1",
			},
		},
		{
			StorageInstanceAttachInfo: domainstorage.StorageInstanceAttachInfo{
				UUID:      instUUID2,
				CharmName: new("charm"),
				Filesystem: &domainstorage.StorageInstanceAttachFilesystemInfo{
					UUID: fsUUID2,
				},
				Kind:             domainstorage.StorageKindFilesystem,
				Life:             life.Alive,
				RequestedSizeMIB: 1024,
				StorageName:      "st2",
			},
		},
		{
			StorageInstanceAttachInfo: domainstorage.StorageInstanceAttachInfo{
				UUID:      instUUID4,
				CharmName: new("charm"),
				Volume: &domainstorage.StorageInstanceAttachVolumeInfo{
					UUID: vUUID1,
				},
				Kind:             domainstorage.StorageKindFilesystem,
				Life:             life.Alive,
				RequestedSizeMIB: 1024,
				StorageName:      "st3",
			},
		},
	})
}

// TestGetStorageInstancesForProviderIDsVolumeBackedFilesystems exists to test
// de-duplication of data. We want to see that for volume backed filesystems
// where the volume and filesystem share the same provider id still only one
// storage instance is returned. We then also want to check where the ids are
// different and that still only one storage instance is returned.
func (s *storageSuite) TestGetStorageInstancesForProviderIDsVolumeBackedFilesystems(c *tc.C) {
	instUUID1, fsUUID1, vUUID1 :=
		s.newStorageInstanceFilesystemBackedVolumeWithProviderID(
			c, "st1", "provider1", "provider1",
		)
	instUUID2, fsUUID2, vUUID2 :=
		s.newStorageInstanceFilesystemBackedVolumeWithProviderID(
			c, "st2", "providerfs", "providerv",
		)

	st := NewState(
		s.ModelSuite.TxnRunnerFactory(),
		s.modelUUID,
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)

	res, err := st.GetStorageInstancesForProviderIDs(
		c.Context(), []string{
			"provider1",
			"providerfs",
			"providerv",
		},
	)

	mc := tc.NewMultiChecker()
	mc.AddExpr("_.Filesystem.ProvisionScope", tc.Ignore)
	mc.AddExpr("_.Filesystem.Size", tc.Ignore)
	mc.AddExpr("_.Volume.ProvisionScope", tc.Ignore)
	mc.AddExpr("_.Volume.Size", tc.Ignore)
	c.Check(err, tc.ErrorIsNil)
	c.Check(
		res,
		tc.UnorderedMatch[[]domainstorage.StorageInstanceInfoForAttach](mc),
		[]domainstorage.StorageInstanceInfoForAttach{
			{
				StorageInstanceAttachInfo: domainstorage.StorageInstanceAttachInfo{
					UUID:      instUUID1,
					CharmName: new("charm"),
					Filesystem: &domainstorage.StorageInstanceAttachFilesystemInfo{
						UUID: fsUUID1,
					},
					Volume: &domainstorage.StorageInstanceAttachVolumeInfo{
						UUID: vUUID1,
					},
					Kind:             domainstorage.StorageKindFilesystem,
					Life:             life.Alive,
					RequestedSizeMIB: 1024,
					StorageName:      "st1",
				},
			},
			{
				StorageInstanceAttachInfo: domainstorage.StorageInstanceAttachInfo{
					UUID:      instUUID2,
					CharmName: new("charm"),
					Filesystem: &domainstorage.StorageInstanceAttachFilesystemInfo{
						UUID: fsUUID2,
					},
					Volume: &domainstorage.StorageInstanceAttachVolumeInfo{
						UUID: vUUID2,
					},
					Kind:             domainstorage.StorageKindFilesystem,
					Life:             life.Alive,
					RequestedSizeMIB: 1024,
					StorageName:      "st2",
				},
			},
		})
}
