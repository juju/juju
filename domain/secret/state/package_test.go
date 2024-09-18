// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"testing"
	"time"

	gc "gopkg.in/check.v1"

	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/domain"
	domainsecret "github.com/juju/juju/domain/secret"
	"github.com/juju/juju/internal/uuid"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

func (st *State) updateSecretForTest(ctx context.Context, uri *coresecrets.URI, secret domainsecret.UpsertSecretParams,
) error {
	return st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateSecret(ctx, uri, secret)
	})
}

func (st *State) getSecretValueForTest(ctx context.Context, uri *coresecrets.URI, revision int) (coresecrets.SecretData, *coresecrets.ValueRef, error) {
	var data coresecrets.SecretData
	var ref *coresecrets.ValueRef
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		data, ref, err = st.GetSecretValue(ctx, uri, revision)
		return err
	})
	return data, ref, err
}

func (st *State) getSecretConsumerForTest(ctx context.Context, uri *coresecrets.URI, unitName string) (*coresecrets.SecretConsumerMetadata, int, error) {
	var (
		consumerMetadata *coresecrets.SecretConsumerMetadata
		latestRevision   int
	)
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		consumerMetadata, latestRevision, err = st.GetSecretConsumer(ctx, uri, unitName)
		return err
	})
	if err != nil {
		return nil, latestRevision, err
	}
	return consumerMetadata, latestRevision, err
}

func (st *State) saveSecretConsumerForTest(ctx context.Context, uri *coresecrets.URI, unitName string, md *coresecrets.SecretConsumerMetadata) error {
	return st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.SaveSecretConsumer(ctx, uri, unitName, md)
	})
}

func (st *State) getSecretRemoteConsumerForTest(ctx context.Context, uri *coresecrets.URI, unitName string) (*coresecrets.SecretConsumerMetadata, int, error) {
	var (
		consumerMetadata *coresecrets.SecretConsumerMetadata
		latestRevision   int
	)
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		consumerMetadata, latestRevision, err = st.GetSecretRemoteConsumer(ctx, uri, unitName)
		return err
	})
	if err != nil {
		return nil, latestRevision, err
	}
	return consumerMetadata, latestRevision, err
}

func (st *State) saveSecretRemoteConsumerForTest(ctx context.Context, uri *coresecrets.URI, unitName string, md *coresecrets.SecretConsumerMetadata) error {
	return st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.SaveSecretRemoteConsumer(ctx, uri, unitName, md)
	})
}

func (st *State) getRotationExpiryInfoForTest(ctx context.Context, uri *coresecrets.URI) (*domainsecret.RotationExpiryInfo, error) {
	var info *domainsecret.RotationExpiryInfo
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		info, err = st.GetRotationExpiryInfo(ctx, uri)
		return err
	})
	return info, err
}

func (st *State) getRotatePolicyForTest(ctx context.Context, uri *coresecrets.URI) (coresecrets.RotatePolicy, error) {
	var policy coresecrets.RotatePolicy
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		policy, err = st.GetRotatePolicy(ctx, uri)
		return err
	})
	return policy, err
}

func (st *State) secretRotatedForTest(ctx context.Context, uri *coresecrets.URI, next time.Time) error {
	return st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.SecretRotated(ctx, uri, next)
	})
}

func (st *State) getSecretAccessForTest(ctx context.Context, uri *coresecrets.URI, params domainsecret.AccessParams) (string, error) {
	var access string
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		access, err = st.GetSecretAccess(ctx, uri, params)
		return err
	})
	return access, err
}

func (st *State) grantAccessForTest(ctx context.Context, uri *coresecrets.URI, params domainsecret.GrantParams) error {
	return st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.GrantAccess(ctx, uri, params)
	})
}

func (st *State) revokeAccessForTest(ctx context.Context, uri *coresecrets.URI, params domainsecret.AccessParams) error {
	return st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.RevokeAccess(ctx, uri, params)
	})
}

func (st *State) listCharmSecretsForTest(ctx context.Context, appOwners domainsecret.ApplicationOwners, unitOwners domainsecret.UnitOwners,
) ([]*coresecrets.SecretMetadata, [][]*coresecrets.SecretRevisionMetadata, error) {
	var (
		mds  []*coresecrets.SecretMetadata
		revs [][]*coresecrets.SecretRevisionMetadata
	)
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		mds, revs, err = st.ListCharmSecrets(ctx, appOwners, unitOwners)
		return err
	})
	return mds, revs, err
}

func (st *State) changeSecretBackendForTest(
	ctx context.Context, revisionID uuid.UUID, valueRef *coresecrets.ValueRef, data coresecrets.SecretData,
) error {
	return st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.ChangeSecretBackend(ctx, revisionID, valueRef, data)
	})
}
