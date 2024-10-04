// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"testing"

	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/domain"
	domainsecret "github.com/juju/juju/domain/secret"
	gc "gopkg.in/check.v1"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

func getApplicationUUID(ctx context.Context, st *State, appName string) (string, error) {
	var uuid string
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		uuid, err = st.GetApplicationUUID(ctx, appName)
		return err
	})
	return uuid, err
}

func getUnitUUID(ctx context.Context, st *State, unitName string) (string, error) {
	var uuid string
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		uuid, err = st.GetUnitUUID(ctx, unitName)
		return err
	})
	return uuid, err
}

func getSecretOwner(ctx context.Context, st *State, uri *coresecrets.URI) (coresecrets.Owner, error) {
	var owner coresecrets.Owner
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		owner, err = st.GetSecretOwner(ctx, uri)
		return err
	})
	return owner, err
}

func checkUserSecretLabelExists(ctx context.Context, st *State, label string) error {
	return st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.CheckUserSecretLabelExists(ctx, label)
	})
}

func checkApplicationSecretLabelExists(ctx context.Context, st *State, appUUID string, label string) error {
	return st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.CheckApplicationSecretLabelExists(ctx, appUUID, label)
	})
}

func checkUnitSecretLabelExists(ctx context.Context, st *State, unitUUID string, label string) error {
	return st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.CheckUnitSecretLabelExists(ctx, unitUUID, label)
	})
}

func createUserSecret(ctx context.Context, st *State, version int, uri *coresecrets.URI, secret domainsecret.UpsertSecretParams) error {
	return st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.CreateUserSecret(ctx, version, uri, secret)
	})
}

func createCharmApplicationSecret(ctx context.Context, st *State, version int, uri *coresecrets.URI, appName string, secret domainsecret.UpsertSecretParams) error {
	return st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		appUUID, err := st.GetApplicationUUID(ctx, appName)
		if err != nil {
			return err
		}
		return st.CreateCharmApplicationSecret(ctx, version, uri, appUUID, secret)
	})
}

func createCharmUnitSecret(ctx context.Context, st *State, version int, uri *coresecrets.URI, unitName string, secret domainsecret.UpsertSecretParams) error {
	return st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		unitUUID, err := st.GetUnitUUID(ctx, unitName)
		if err != nil {
			return err
		}
		return st.CreateCharmUnitSecret(ctx, version, uri, unitUUID, secret)
	})
}

func updateSecret(ctx context.Context, st *State, uri *coresecrets.URI, secret domainsecret.UpsertSecretParams,
) error {
	return st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateSecret(ctx, uri, secret)
	})
}
