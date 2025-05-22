// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	coreapplication "github.com/juju/juju/core/application"
	coresecrets "github.com/juju/juju/core/secrets"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	domainsecret "github.com/juju/juju/domain/secret"
)

func getApplicationUUID(ctx context.Context, st *State, appName string) (coreapplication.ID, error) {
	var uuid coreapplication.ID
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		uuid, err = st.GetApplicationUUID(ctx, appName)
		return err
	})
	return uuid, err
}

func getUnitUUID(ctx context.Context, st *State, unitName coreunit.Name) (coreunit.UUID, error) {
	var uuid coreunit.UUID
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		uuid, err = st.GetUnitUUID(ctx, unitName)
		return err
	})
	return uuid, err
}

func getSecretOwner(ctx context.Context, st *State, uri *coresecrets.URI) (domainsecret.Owner, error) {
	var owner domainsecret.Owner
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		owner, err = st.GetSecretOwner(ctx, uri)
		return err
	})
	return owner, err
}

func checkUserSecretLabelExists(ctx context.Context, st *State, label string) (bool, error) {
	var exists bool
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		exists, err = st.CheckUserSecretLabelExists(ctx, label)
		return err
	})
	return exists, err
}

func checkApplicationSecretLabelExists(ctx context.Context, st *State, appUUID coreapplication.ID, label string) (bool, error) {
	var exists bool
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		exists, err = st.CheckApplicationSecretLabelExists(ctx, appUUID, label)
		return err
	})
	return exists, err
}

func checkUnitSecretLabelExists(ctx context.Context, st *State, unitUUID coreunit.UUID, label string) (bool, error) {
	var exists bool
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		exists, err = st.CheckUnitSecretLabelExists(ctx, unitUUID, label)
		return err
	})
	return exists, err
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

func createCharmUnitSecret(ctx context.Context, st *State, version int, uri *coresecrets.URI, unitName coreunit.Name, secret domainsecret.UpsertSecretParams) error {
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
