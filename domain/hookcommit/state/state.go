// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/secrets"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/hookcommit"
	domainsecret "github.com/juju/juju/domain/secret"
)

// State represents a type for interacting with the underlying state.
type State struct {
	*domain.StateBase
}

func (st *State) GetAppUnitUUID(ctx context.Context, unitName string) (coreapplication.ID, coreunit.UUID, error) {
	return "", "", nil
}

func (st *State) CommitHookChanges(ctx context.Context, changes hookcommit.CommitHookChangesArgs) error {
	db, err := st.DB()
	if err != nil {
		return err
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		for _, c := range changes.AppSecretCreates {
			if err := st.createCharmApplicationSecret(ctx, tx, c.Version, changes.ApplicationUUID, c.URI, c.UpsertSecretParams); err != nil {
				return err
			}
		}

		for _, c := range changes.UnitSecretCreates {
			if err := st.createCharmUnitSecret(ctx, tx, c.Version, changes.UnitUUID, c.URI, c.UpsertSecretParams); err != nil {
				return err
			}
		}
		for _, d := range changes.SecretDeletes {
			if err := st.deleteSecret(ctx, tx, d.URI, d.Revisions); err != nil {
				return err
			}
		}

		return nil
	})
}

// updateUnitPorts opens and closes ports for the endpoints of a given unit.
// The opened and closed ports for the same endpoints must not conflict.
func (st *State) updateUnitPorts(ctx context.Context, unitUUID coreunit.UUID, openPorts, closePorts network.GroupedPortRanges) error {
	return nil
}

// setUnitStateCharm replaces the agent charm
// state for the unit with the input UUID.
func (st *State) setUnitStateCharm(context.Context, string, map[string]string) error {
	return nil
}

func (st *State) createCharmApplicationSecret(
	ctx context.Context, tx *sqlair.TX, version int, appUUID coreapplication.ID, uri *secrets.URI, secret domainsecret.UpsertSecretParams,
) error {
	return nil
}

func (st *State) createCharmUnitSecret(
	ctx context.Context, tx *sqlair.TX, version int, unitUUID coreunit.UUID, uri *secrets.URI, secret domainsecret.UpsertSecretParams,
) error {
	return nil
}

func (st *State) updateSecret(ctx context.Context, uri *secrets.URI, secret domainsecret.UpsertSecretParams) error {
	return nil
}

func (st *State) deleteSecret(ctx context.Context, tx *sqlair.TX, uri *secrets.URI, revs []int) error {
	return nil
}

func (st *State) grantAccess(ctx context.Context, uri *secrets.URI, params domainsecret.GrantParams) error {
	return nil
}

func (st *State) revokeAccess(ctx context.Context, uri *secrets.URI, params domainsecret.AccessParams) error {
	return nil
}
