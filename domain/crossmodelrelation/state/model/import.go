// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"

	"github.com/juju/juju/domain/crossmodelrelation"
	"github.com/juju/juju/internal/errors"
)

// ImportOffers adds offers being migrated to the current model.
func (st *State) ImportOffers(ctx context.Context, imports []crossmodelrelation.OfferImport) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	applicationNames := transform.Slice(imports, func(in crossmodelrelation.OfferImport) string {
		return in.ApplicationName
	})
	uniqueApplicationNames := set.NewStrings(applicationNames...)

	createOffersStmt, err := st.Prepare(`
INSERT INTO offer (*) VALUES ($nameAndUUID.*)`, nameAndUUID{})
	if err != nil {
		return errors.Errorf("preparing insert offer query: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		appNamesUUIDs, err := st.getApplicationUUIDs(ctx, tx, uniqueApplicationNames.Values())
		if err != nil {
			return err
		}
		if len(appNamesUUIDs) != uniqueApplicationNames.Size() {
			return errors.Errorf("expected %d application uuids, got %d", len(appNamesUUIDs), len(applicationNames))
		}

		offersToAdd := transform.Slice(imports, func(in crossmodelrelation.OfferImport) nameAndUUID {
			return nameAndUUID{
				Name: in.Name,
				UUID: in.UUID.String(),
			}
		})

		err = tx.Query(ctx, createOffersStmt, offersToAdd).Run()
		if err != nil {
			return errors.Errorf("inserting import offer rows: %w", err)
		}

		for _, o := range imports {
			appUUID := appNamesUUIDs[o.ApplicationName]
			err := st.createOfferEndpoints(ctx, tx, o.UUID.String(), appUUID, o.Endpoints)
			if err != nil {
				return errors.Errorf("inserting import offer endpoints for %q: %w", o.Name, err)
			}
		}

		return nil
	})
	return errors.Capture(err)
}
