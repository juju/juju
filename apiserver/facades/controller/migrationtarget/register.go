// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationtarget

import (
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/facades"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/state/stateenvirons"
)

// Register is called to expose a package of facades onto a given registry.
func Register(requiredMigrationFacadeVersions facades.FacadeVersions) func(registry facade.FacadeRegistry) {
	return func(registry facade.FacadeRegistry) {
		registry.MustRegister("MigrationTarget", 1, func(ctx facade.Context) (facade.Facade, error) {
			return newFacadeV1(ctx)
		}, reflect.TypeOf((*APIV1)(nil)))
		registry.MustRegister("MigrationTarget", 2, func(ctx facade.Context) (facade.Facade, error) {
			return newFacadeV2(ctx)
		}, reflect.TypeOf((*APIV2)(nil)))
		registry.MustRegister("MigrationTarget", 3, func(ctx facade.Context) (facade.Facade, error) {
			return newFacade(ctx, requiredMigrationFacadeVersions)
		}, reflect.TypeOf((*API)(nil)))
		// The facade is bumped to version 4 due to a bug in exported charm
		// data, aside from the fix there are no other changes, but subsequent
		// major versions of Juju should not use previous versions of the
		// facade that may contain the bug.
		registry.MustRegister("MigrationTarget", 4, func(ctx facade.Context) (facade.Facade, error) {
			return newFacade(ctx, requiredMigrationFacadeVersions)
		}, reflect.TypeOf((*API)(nil)))
		// Bumped to version 5 for the addition of the token field in
		// the MigrationTargetInfo struct.
		registry.MustRegister("MigrationTarget", 5, func(ctx facade.Context) (facade.Facade, error) {
			return newFacade(ctx, requiredMigrationFacadeVersions)
		}, reflect.TypeOf((*API)(nil)))
	}
}

// newFacadeV1 is used for APIV1 registration.
func newFacadeV1(ctx facade.Context) (*APIV1, error) {
	api, err := NewAPI(
		ctx,
		stateenvirons.GetNewEnvironFunc(environs.New),
		stateenvirons.GetNewCAASBrokerFunc(caas.New),
		facades.FacadeVersions{},
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIV1{API: api}, nil
}

// newFacadeV2 is used for APIV2 registration.
func newFacadeV2(ctx facade.Context) (*APIV2, error) {
	api, err := NewAPI(
		ctx,
		stateenvirons.GetNewEnvironFunc(environs.New),
		stateenvirons.GetNewCAASBrokerFunc(caas.New),
		facades.FacadeVersions{},
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIV2{APIV1: &APIV1{API: api}}, nil
}

// newFacade is used for API registration.
func newFacade(ctx facade.Context, facadeVersions facades.FacadeVersions) (*API, error) {
	return NewAPI(
		ctx,
		stateenvirons.GetNewEnvironFunc(environs.New),
		stateenvirons.GetNewCAASBrokerFunc(caas.New),
		facadeVersions,
	)
}
