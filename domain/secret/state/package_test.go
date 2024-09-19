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
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

func updateSecret(ctx context.Context, st *State, uri *coresecrets.URI, secret domainsecret.UpsertSecretParams,
) error {
	return st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateSecret(ctx, uri, secret)
	})
}

func getSecretConsumer(ctx context.Context, st *State, uri *coresecrets.URI, unitName string) (*coresecrets.SecretConsumerMetadata, int, error) {
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

func saveSecretConsumer(ctx context.Context, st *State, uri *coresecrets.URI, unitName string, md *coresecrets.SecretConsumerMetadata) error {
	return st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.SaveSecretConsumer(ctx, uri, unitName, md)
	})
}

func getSecretRemoteConsumer(ctx context.Context, st *State, uri *coresecrets.URI, unitName string) (*coresecrets.SecretConsumerMetadata, int, error) {
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

func saveSecretRemoteConsumer(ctx context.Context, st *State, uri *coresecrets.URI, unitName string, md *coresecrets.SecretConsumerMetadata) error {
	return st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.SaveSecretRemoteConsumer(ctx, uri, unitName, md)
	})
}

func getRotationExpiryInfo(ctx context.Context, st *State, uri *coresecrets.URI) (*domainsecret.RotationExpiryInfo, error) {
	var info *domainsecret.RotationExpiryInfo
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		info, err = st.GetRotationExpiryInfo(ctx, uri)
		return err
	})
	return info, err
}

func getRotatePolicy(ctx context.Context, st *State, uri *coresecrets.URI) (coresecrets.RotatePolicy, error) {
	var policy coresecrets.RotatePolicy
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		policy, err = st.GetRotatePolicy(ctx, uri)
		return err
	})
	return policy, err
}

func secretRotated(ctx context.Context, st *State, uri *coresecrets.URI, next time.Time) error {
	return st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.SecretRotated(ctx, uri, next)
	})
}

func listCharmSecrets(ctx context.Context, st *State, appOwners domainsecret.ApplicationOwners, unitOwners domainsecret.UnitOwners,
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
