// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	coremachine "github.com/juju/juju/core/machine"
	corenetwork "github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	domainapplication "github.com/juju/juju/domain/application"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/schema/testing"
	domainsequence "github.com/juju/juju/domain/sequence"
	domainsequencestate "github.com/juju/juju/domain/sequence/state"
	domainstorage "github.com/juju/juju/domain/storage"
)

type baseSuite struct {
	testing.ModelSuite
}

// preparer implements a testing [github.com/juju/juju/domain.Preparer] that
// results in a proxied call to [sqlair.Prepare].
type preparer struct{}

// newCharm creates a new charm in the model returning the charm uuid.
func (s *baseSuite) newCharm(c *tc.C) corecharm.ID {
	charmUUID := tc.Must(c, corecharm.NewID)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(
			ctx,
			`
INSERT INTO charm (uuid, source_id, reference_name, revision, architecture_id)
VALUES (?, 0, ?, 1, 0)
`,
			charmUUID.String(), charmUUID.String(),
		)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(
			ctx,
			`
INSERT INTO charm_metadata (charm_uuid, name)
VALUES (?, ?)
`,
			charmUUID.String(), charmUUID.String(),
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	return charmUUID
}

// newApplication creates a new application in the model with the provided name.
// It also creates a new charm and associates it with the application.
// Returns the application UUID and the charm UUID.
func (s *baseSuite) newApplication(c *tc.C, name string) (coreapplication.UUID, corecharm.ID) {
	appUUID := tc.Must(c, coreapplication.NewUUID)
	charmUUID := s.newCharm(c)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO application (uuid, charm_uuid, name, life_id, space_uuid)
VALUES (?, ?, ?, "0", ?)`, appUUID.String(), charmUUID.String(), name, corenetwork.AlphaSpaceId)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	return appUUID, charmUUID
}

// newMachine create a new machine in the model returning the uuid of the new
// Machine created.
func (s *baseSuite) newMachine(c *tc.C) coremachine.UUID {
	uuid := tc.Must(c, coremachine.NewUUID)
	netNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)

	_, err := s.DB().Exec("INSERT INTO net_node VALUES (?)", netNodeUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().Exec(
		`INSERT INTO machine (uuid, name, net_node_uuid, life_id)
		VALUES (?, ?, ?, 0)`,
		uuid.String(), uuid.String(), netNodeUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	return uuid
}

// newStorageAttachment is responsible for establishing a new storage attachment
// in the model between the provided storage instance and unit.
func (s *baseSuite) newStorageAttachment(
	c *tc.C,
	storageInstanceUUID domainstorage.StorageInstanceUUID,
	unitUUID coreunit.UUID,
) domainstorage.StorageAttachmentUUID {
	saUUID := tc.Must(c, domainstorage.NewStorageAttachmentUUID)

	_, err := s.DB().Exec(`
INSERT INTO storage_attachment (uuid, storage_instance_uuid, unit_uuid, life_id)
VALUES (?, ?, ?, 1)
`,
		saUUID.String(), storageInstanceUUID.String(), unitUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)
	return saUUID
}

// newStorageInstanceForCharmWithPool is responsible for establishing a new
// storage instance in the model using the supplied storage pool. Returned is
// the new uuid for the storage instance and the storage id.
func (s *baseSuite) newStorageInstanceForCharmWithPool(
	c *tc.C,
	charmUUID corecharm.ID,
	poolUUID domainstorage.StoragePoolUUID,
	storageName string,
) (domainstorage.StorageInstanceUUID, string) {
	var charmName string
	s.DumpTable(c, "charm_metadata")
	err := s.DB().QueryRowContext(
		c.Context(),
		"SELECT name FROM charm_metadata WHERE charm_uuid = ?",
		charmUUID.String(),
	).Scan(&charmName)
	c.Assert(err, tc.ErrorIsNil)

	storageInstanceUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	storageID := fmt.Sprintf("%s/%d", storageName, s.nextStorageSequenceNumber(c))

	_, err = s.DB().Exec(`
INSERT INTO storage_instance(uuid, charm_name, storage_name, storage_id,
                             life_id, requested_size_mib, storage_pool_uuid,
                             storage_kind_id)
VALUES (?, ?, ?, ?, 0, 100, ?, 1)
`,
		storageInstanceUUID.String(),
		charmName,
		storageName,
		storageID,
		poolUUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	return storageInstanceUUID, storageID
}

// newStoragePool creates a new storage pool with name, provider type and attrs.
// It returns the UUID of the new storage pool.
func (s *baseSuite) newStoragePool(
	c *tc.C, name string, providerType string, attrs map[string]string,
) domainstorage.StoragePoolUUID {
	spUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO storage_pool (uuid, name, type)
VALUES (?, ?, ?)`, spUUID.String(), name, providerType)
		if err != nil {
			return err
		}

		for k, v := range attrs {
			_, err = tx.ExecContext(ctx, `
INSERT INTO storage_pool_attribute (storage_pool_uuid, key, value)
VALUES (?, ?, ?)`, spUUID.String(), k, v)
			if err != nil {
				return err
			}
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	return spUUID
}

// nextStorageSequenceNumber retrieves the next sequence number in the storage
// namespace.
func (s *baseSuite) nextStorageSequenceNumber(c *tc.C) uint64 {
	var id uint64
	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		id, err = domainsequencestate.NextValue(
			ctx, preparer{}, tx, domainsequence.StaticNamespace("storage"),
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	return id
}

// newUnit is responsible for establishing a unit with in the model and
// returning the units uuid. This should only be used when the test just
// requires a unit in the model and no other parameters are required.
func (s *baseSuite) newUnit(
	c *tc.C,
) coreunit.UUID {
	appUUID := tc.Must(c, coreapplication.NewUUID)
	charmUUID := tc.Must(c, corecharm.NewID)
	unitUUID := tc.Must(c, coreunit.NewUUID)
	unitNetNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(
			ctx,
			`
INSERT INTO charm (uuid, source_id, reference_name, revision, architecture_id)
VALUES (?, 0, ?, 1, 0)
`,
			charmUUID.String(), charmUUID.String(),
		)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(
			ctx,
			`
INSERT INTO application (uuid, charm_uuid, name, life_id, space_uuid)
VALUES (?, ?, ?, "0", ?)
`,
			appUUID.String(), charmUUID, appUUID.String(),
			corenetwork.AlphaSpaceId,
		)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(
			ctx,
			"INSERT INTO net_node VALUES (?)",
			unitNetNodeUUID.String(),
		)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(
			c.Context(), `
INSERT INTO unit (uuid, name, application_uuid, charm_uuid, net_node_uuid, life_id)
VALUES (?, ?, ?, ?, ?, 0)
`,
			unitUUID.String(),
			appUUID.String()+"/0",
			appUUID.String(),
			charmUUID.String(),
			unitNetNodeUUID.String(),
		)
		if err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	return unitUUID
}

// newUnitForApplication creates a new unit in the model for the supplied
// application uuid and returns the unit uuid and net node uuid.
func (s *baseSuite) newUnitForApplication(
	c *tc.C,
	appUUID coreapplication.UUID,
) (coreunit.UUID, domainnetwork.NetNodeUUID) {
	var charmUUID, appName string
	err := s.DB().QueryRowContext(
		c.Context(),
		"SELECT charm_uuid, name FROM application WHERE uuid = ?",
		appUUID.String(),
	).Scan(&charmUUID, &appName)
	c.Assert(err, tc.ErrorIsNil)

	unitUUID := tc.Must(c, coreunit.NewUUID)
	unitNetNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)

	var unitNum uint64
	err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		namespace := domainsequence.MakePrefixNamespace(
			domainapplication.ApplicationSequenceNamespace, appName,
		)
		var err error
		unitNum, err = domainsequencestate.NextValue(
			ctx, preparer{}, tx, namespace,
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	unitName := appName + "/" + strconv.FormatUint(unitNum, 10)

	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(
			ctx,
			"INSERT INTO net_node VALUES (?)",
			unitNetNodeUUID.String(),
		)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(
			ctx, `
INSERT INTO unit (uuid, name, application_uuid, charm_uuid, net_node_uuid, life_id)
VALUES (?, ?, ?, ?, ?, 0)
`,
			unitUUID.String(),
			unitName,
			appUUID.String(),
			charmUUID,
			unitNetNodeUUID.String(),
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	return unitUUID, unitNetNodeUUID
}

// Prepare proxies the call to [sqlair.Prepare] implementing the
// [github.com/juju/juju/domain.Preparer] interface.
func (p preparer) Prepare(query string, typeSamples ...any) (*sqlair.Statement, error) {
	return sqlair.Prepare(query, typeSamples...)
}
