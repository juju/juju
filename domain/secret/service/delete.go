// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/internal/secrets/provider"
)

func (s *SecretService) DeleteCharmSecret(ctx context.Context, uri *secrets.URI, revisions []int, canDelete func(uri *secrets.URI) error) error {
	panic("implement me")
}

// DeleteObsoleteUserSecrets deletes any obsolete user secret revisions that are marked as auto-prune.
func (s *SecretService) DeleteObsoleteUserSecrets(ctx context.Context) error {
	panic("implement me")
}

// DeleteUserSecret removes the specified user supplied secret.
// The secret is removed from state and backend.
func (s *SecretService) DeleteUserSecret(ctx context.Context, uri *secrets.URI, revisions []int) error {
	// TODO(secrets) - get model uuid from state
	var modelUUID string

	return s.deleteSecret(
		ctx,
		uri, revisions,
		func(ctx context.Context, p provider.SecretBackendProvider, cfg provider.ModelBackendConfig, revs provider.SecretRevisions) error {
			backend, err := p.NewBackend(&cfg)
			if err != nil {
				return errors.Trace(err)
			}
			for _, revId := range revs.RevisionIDs() {
				if err = backend.DeleteContent(ctx, revId); err != nil {
					return errors.Trace(err)
				}
			}
			// Ideally we'd not use tags but secret API uses them.
			ownerTag := names.NewModelTag(modelUUID)
			if err := p.CleanupSecrets(ctx, &cfg, ownerTag, revs); err != nil {
				return errors.Trace(err)
			}
			return nil
		},
	)
}

func (s *SecretService) deleteSecret(
	ctx context.Context,
	uri *secrets.URI,
	revisions []int,
	removeFromBackend func(context.Context, provider.SecretBackendProvider, provider.ModelBackendConfig, provider.SecretRevisions) error,
) error {
	cfgInfo, err := s.adminConfigGetter(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	removeFromExternal := func(uri *secrets.URI, revisions ...int) error {
		externalRevs := make(map[string]provider.SecretRevisions)
		gatherExternalRevs := func(valRef *secrets.ValueRef) {
			if valRef == nil {
				// Internal secret, nothing to do here.
				return
			}
			if _, ok := externalRevs[valRef.BackendID]; !ok {
				externalRevs[valRef.BackendID] = provider.SecretRevisions{}
			}
			externalRevs[valRef.BackendID].Add(uri, valRef.RevisionID)
		}
		if len(revisions) == 0 {
			// Remove all revisions.
			revs, err := s.ListSecretRevisions(ctx, uri)
			if err != nil {
				return errors.Trace(err)
			}
			for _, rev := range revs {
				gatherExternalRevs(rev.ValueRef)
			}
		} else {
			for _, rev := range revisions {
				revMeta, err := s.GetSecretRevision(ctx, uri, rev)
				if err != nil {
					return errors.Trace(err)
				}
				gatherExternalRevs(revMeta.ValueRef)
			}
		}

		for backendID, r := range externalRevs {
			backendCfg, ok := cfgInfo.Configs[backendID]
			if !ok {
				return errors.NotFoundf("secret backend %q", backendID)
			}
			provider, err := s.providerGetter(backendCfg.BackendType)
			if err != nil {
				return errors.Trace(err)
			}
			if err := removeFromBackend(ctx, provider, backendCfg, r); err != nil {
				return errors.Trace(err)
			}
		}
		return nil
	}

	// We remove the secret from the backend first.
	if err := removeFromExternal(uri, revisions...); err != nil {
		return errors.Trace(err)
	}

	panic("implement me")
}
