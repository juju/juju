// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	corenetwork "github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/schema/testing"
	domainsequence "github.com/juju/juju/domain/sequence"
	domainsequencestate "github.com/juju/juju/domain/sequence/state"
	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageprovisioning "github.com/juju/juju/domain/storageprovisioning"
)

type baseSuite struct {
	testing.ModelSuite
}

// preparer implements a testing [github.com/juju/juju/domain.Preparer] that
// results in a proxied call to [sqlair.Prepare].
type preparer struct{}

// newStorageAttachment is responsible for establishing a new storage attachment
// in the model between the provided storage instance and unit.
func (s *baseSuite) newStorageAttachment(
	c *tc.C,
	storageInstanceUUID domainstorage.StorageInstanceUUID,
	unitUUID coreunit.UUID,
) domainstorageprovisioning.StorageAttachmentUUID {
	saUUID := tc.Must(c, domainstorageprovisioning.NewStorageAttachmentUUID)

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
	charmName string,
	poolUUID domainstorage.StoragePoolUUID,
	storageName string,
) (domainstorage.StorageInstanceUUID, string) {
	storageInstanceUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	storageID := fmt.Sprintf("%s/%d", storageName, s.nextStorageSequenceNumber(c))

	_, err := s.DB().Exec(`
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

// Prepare proxies the call to [sqlair.Prepare] implementing the
// [github.com/juju/juju/domain.Preparer] interface.
func (p preparer) Prepare(query string, typeSamples ...any) (*sqlair.Statement, error) {
	return sqlair.Prepare(query, typeSamples...)
}
