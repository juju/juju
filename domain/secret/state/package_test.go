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

func createUserSecret(ctx context.Context, st *State, version int, uri *coresecrets.URI, secret domainsecret.UpsertSecretParams) error {
	return st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.CreateUserSecret(ctx, version, uri, secret)
	})
}

func createCharmApplicationSecret(ctx context.Context, st *State, version int, uri *coresecrets.URI, appName string, secret domainsecret.UpsertSecretParams) error {
	return st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.CreateCharmApplicationSecret(ctx, version, uri, appName, secret)
	})
}

func createCharmUnitSecret(ctx context.Context, st *State, version int, uri *coresecrets.URI, unitName string, secret domainsecret.UpsertSecretParams) error {
	return st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.CreateCharmUnitSecret(ctx, version, uri, unitName, secret)
	})
}

func updateSecret(ctx context.Context, st *State, uri *coresecrets.URI, secret domainsecret.UpsertSecretParams,
) error {
	return st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateSecret(ctx, uri, secret)
	})
}

func getSecretValue(ctx context.Context, st *State, uri *coresecrets.URI, revision int) (coresecrets.SecretData, *coresecrets.ValueRef, error) {
	var data coresecrets.SecretData
	var ref *coresecrets.ValueRef
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		data, ref, err = st.GetSecretValue(ctx, uri, revision)
		return err
	})
	return data, ref, err
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

func getSecretAccess(ctx context.Context, st *State, uri *coresecrets.URI, params domainsecret.AccessParams) (string, error) {
	var access string
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		access, err = st.GetSecretAccess(ctx, uri, params)
		return err
	})
	return access, err
}

func grantAccess(ctx context.Context, st *State, uri *coresecrets.URI, params domainsecret.GrantParams) error {
	return st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.GrantAccess(ctx, uri, params)
	})
}

func revokeAccess(ctx context.Context, st *State, uri *coresecrets.URI, params domainsecret.AccessParams) error {
	return st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.RevokeAccess(ctx, uri, params)
	})
}

func listCharmSecrets(ctx context.Context, st *State, appOwners domainsecret.ApplicationOwners, unitOwners domainsecret.UnitOwners,
) ([]*domainsecret.SecretMetadata, error) {
	var mds []*domainsecret.SecretMetadata
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		mds, err = st.ListCharmSecrets(ctx, appOwners, unitOwners)
		return err
	})
	return mds, err
}

func changeSecretBackend(
	ctx context.Context, st *State, revisionID uuid.UUID, valueRef *coresecrets.ValueRef, data coresecrets.SecretData,
) error {
	return st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.ChangeSecretBackend(ctx, revisionID, valueRef, data)
	})
}

func listAllSecrets(ctx context.Context, st *State) ([]*coresecrets.SecretMetadata, [][]*domainsecret.SecretRevision, error) {
	var (
		mds  []*coresecrets.SecretMetadata
		revs [][]*domainsecret.SecretRevision
	)
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		mds, revs, err = st.ListAllSecrets(ctx)
		return err
	})
	return mds, revs, err
}

func listAllRemoteSecrets(ctx context.Context, st *State) ([]domainsecret.RemoteSecretInfo, error) {
	var infos []domainsecret.RemoteSecretInfo
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		infos, err = st.ListAllRemoteSecrets(ctx)
		return err
	})
	return infos, err
}

func listAllSecretGrants(ctx context.Context, st *State) (map[string][]domainsecret.GrantParams, error) {
	var grants map[string][]domainsecret.GrantParams
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		grants, err = st.ListAllSecretGrants(ctx)
		return err
	})
	return grants, err
}

func listAllSecretConsumers(ctx context.Context, st *State) (map[string][]domainsecret.ConsumerInfo, error) {
	var consumers map[string][]domainsecret.ConsumerInfo
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		consumers, err = st.ListAllSecretConsumers(ctx)
		return err
	})
	return consumers, err
}

func listAllSecretRemoteConsumers(ctx context.Context, st *State) (map[string][]domainsecret.ConsumerInfo, error) {
	var consumers map[string][]domainsecret.ConsumerInfo
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		consumers, err = st.ListAllSecretRemoteConsumers(ctx)
		return err
	})
	return consumers, err
}

func getURIByConsumerLabel(ctx context.Context, st *State, label string, unitName string) (*coresecrets.URI, error) {
	var uri *coresecrets.URI
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		uri, err = st.GetURIByConsumerLabel(ctx, label, unitName)
		return err
	})
	return uri, err
}
