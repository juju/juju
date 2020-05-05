// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/environs"
	envcontext "github.com/juju/juju/environs/context"
	"github.com/juju/juju/worker/common"
)

// ManifoldConfig holds the names of the resources used by, and the
// additional dependencies of, an undertaker worker.
type ManifoldConfig struct {
	APICallerName      string
	CloudDestroyerName string

	Logger                       Logger
	NewFacade                    func(base.APICaller) (Facade, error)
	NewWorker                    func(Config) (worker.Worker, error)
	NewCredentialValidatorFacade func(base.APICaller) (common.CredentialAPI, error)
}

func (config ManifoldConfig) start(context dependency.Context) (worker.Worker, error) {

	var apiCaller base.APICaller
	if err := context.Get(config.APICallerName, &apiCaller); err != nil {
		return nil, errors.Trace(err)
	}
	var destroyer environs.CloudDestroyer
	if err := context.Get(config.CloudDestroyerName, &destroyer); isErrMissing(err) {
		// Rather than bailing out, we continue with a destroyer that
		// always fails. This means that the undertaker will still
		// remove the model in the force-destroy case.
		destroyer = &unavailableDestroyer{}
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	facade, err := config.NewFacade(apiCaller)
	if err != nil {
		return nil, errors.Trace(err)
	}

	credentialAPI, err := config.NewCredentialValidatorFacade(apiCaller)
	if err != nil {
		return nil, errors.Trace(err)
	}

	worker, err := config.NewWorker(Config{
		Facade:        facade,
		Destroyer:     destroyer,
		CredentialAPI: credentialAPI,
		Logger:        config.Logger,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return worker, nil
}

// Manifold returns a dependency.Manifold that runs a worker responsible
// for shepherding a Dying model into Dead and ultimate removal.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.APICallerName,
			config.CloudDestroyerName,
		},
		Start: config.start,
	}
}

func isErrMissing(err error) bool {
	return errors.Cause(err) == dependency.ErrMissing
}

// unavailableDestroyer is an environs.CloudDestroyer that always
// fails to destroy. We use it when the real environ isn't available
// because the cloud credentials are invalid so that the undertaker
// can still remove the model if destruction is forced.
type unavailableDestroyer struct{}

// Destroy is part of environs.CloudDestroyer.
func (*unavailableDestroyer) Destroy(ctx envcontext.ProviderCallContext) error {
	return errors.New("cloud environment unavailable")
}
