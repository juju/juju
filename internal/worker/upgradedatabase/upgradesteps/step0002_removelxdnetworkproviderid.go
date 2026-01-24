// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradesteps

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/errors"
)

var (
	getModelDBCloudTypeQuery = sqlair.MustPrepare(`
SELECT m.cloud_type AS &cloudType.type
FROM   model AS m;
`, cloudType{})
	deleteProviderSubnetQuery = sqlair.MustPrepare(`
DELETE FROM provider_subnet;
`)
	updateProviderNetworkQuery = sqlair.MustPrepare(`
UPDATE provider_network
SET    provider_network_id = trim(provider_network_id, 'net-')
WHERE  provider_network_id LIKE 'net-%';
`)
)

// Step0002_RemoveLXDSubnetProviderID for models with a cloud type of LXD,
// remove the provider_subnet rows and update provider_network_id to start
// start with `net-`.
func Step0002_RemoveLXDSubnetProviderID(ctx context.Context, _, modelDB database.TxnRunner, _ model.UUID) error {
	return modelDB.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var ct cloudType
		if err := tx.Query(ctx, getModelDBCloudTypeQuery).Get(&ct); err != nil {
			return errors.Errorf("getting model cloud type :%w", err)
		}
		if ct.Type != "lxd" {
			return nil
		}
		if err := tx.Query(ctx, deleteProviderSubnetQuery).Run(); err != nil {
			return errors.Errorf("deleting provider_subnet rows :%w", err)
		}
		if err := tx.Query(ctx, updateProviderNetworkQuery).Run(); err != nil {
			return errors.Errorf("updating provider_network rows, remove net- :%w", err)
		}
		return nil
	})
}
