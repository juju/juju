// Copyright 2014, 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateenvirons

import (
	"context"
	"sync"

	"github.com/juju/errors"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/constraints"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/state"
)

// environStatePolicy implements state.Policy in
// terms of environs.Environ and related types.
type environStatePolicy struct {
	st                       *state.State
	cloudService             CloudService
	credentialService        CredentialService
	modelConfigServiceGetter modelConfigServiceGetter
	getEnviron               NewEnvironFunc
	getBroker                NewCAASBrokerFunc
	checkerMu                sync.Mutex
	checker                  deployChecker
	storageServiceGetter     storageServiceGetter
}

// deployChecker is the subset of the Environ interface (common to Environ and
// Broker) that we need for pre-checking instances and validating constraints.
type deployChecker interface {
	environs.InstancePrechecker
	environs.ConstraintsChecker
}

type storageServiceGetter func(modelUUID coremodel.UUID) (state.StoragePoolGetter, error)
type modelConfigServiceGetter func(modelUUID coremodel.UUID) (ModelConfigService, error)

// GetNewPolicyFunc returns a state.NewPolicyFunc that will return
// a state.Policy implemented in terms of either environs.Environ
// or caas.Broker and related types.
func GetNewPolicyFunc(
	cloudService CloudService,
	credentialService CredentialService,
	modelConfigServiceGetter modelConfigServiceGetter,
	storageServiceGetter storageServiceGetter,
) state.NewPolicyFunc {
	return func(st *state.State) state.Policy {
		return &environStatePolicy{
			st:                       st,
			cloudService:             cloudService,
			credentialService:        credentialService,
			modelConfigServiceGetter: modelConfigServiceGetter,
			getEnviron:               GetNewEnvironFunc(environs.New),
			getBroker:                GetNewCAASBrokerFunc(caas.New),
			storageServiceGetter:     storageServiceGetter,
		}
	}
}

// getDeployChecker returns the cached deployChecker instance, or creates a
// new one if it hasn't yet been created and cached.
func (p *environStatePolicy) getDeployChecker() (deployChecker, error) {
	p.checkerMu.Lock()
	defer p.checkerMu.Unlock()

	if p.credentialService == nil {
		return nil, errors.NotSupportedf("deploy check without credential service")
	}
	if p.modelConfigServiceGetter == nil {
		return nil, errors.NotSupportedf("deploy check without model config service getter")
	}
	if p.checker != nil {
		return p.checker, nil
	}

	model, err := p.st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	modelConfigService, err := p.modelConfigServiceGetter(coremodel.UUID(model.UUIDOld()))
	if err != nil {
		return nil, errors.Trace(err)
	}
	if model.TypeOld() == state.ModelTypeIAAS {
		p.checker, err = p.getEnviron(model, p.cloudService, p.credentialService, modelConfigService)
	} else {
		p.checker, err = p.getBroker(model, p.cloudService, p.credentialService, modelConfigService)
	}
	return p.checker, err
}

// ConstraintsValidator implements state.Policy.
func (p *environStatePolicy) ConstraintsValidator(ctx context.Context) (constraints.Validator, error) {
	checker, err := p.getDeployChecker()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return checker.ConstraintsValidator(ctx)
}

// StorageServices implements state.Policy.
func (p *environStatePolicy) StorageServices() (state.StoragePoolGetter, error) {
	model, err := p.st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	return p.storageServiceGetter(coremodel.UUID(model.UUIDOld()))
}
