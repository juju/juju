// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// DeployFromRepositoryValidator defines an deploy config validator.
type DeployFromRepositoryValidator interface {
	ValidateArg(params.DeployFromRepositoryArg) []error
}

// DeployFromRepository defines an interface for deploying a charm
// from a repository.
type DeployFromRepository interface {
	DeployFromRepository(arg params.DeployFromRepositoryArg) ([]string, []*params.PendingResourceUpload, []error)
}

// DeployFromRepositoryState defines a common set of functions for retrieving state
// objects.
type DeployFromRepositoryState interface {
}

// DeployFromRepositoryAPI provides the deploy from repository
// API facade for any given version. It is expected that any API
// parameter changes should be performed before entering the API.
type DeployFromRepositoryAPI struct {
	state     DeployFromRepositoryState
	validator DeployFromRepositoryValidator
}

// NewDeployFromRepositoryAPI creates a new DeployFromRepositoryAPI.
func NewDeployFromRepositoryAPI(state DeployFromRepositoryState, validator DeployFromRepositoryValidator) DeployFromRepository {
	api := &DeployFromRepositoryAPI{
		state:     state,
		validator: validator,
	}
	return api
}

func (api *DeployFromRepositoryAPI) DeployFromRepository(arg params.DeployFromRepositoryArg) ([]string, []*params.PendingResourceUpload, []error) {
	// Validate the args.
	errs := api.validator.ValidateArg(arg)
	if len(errs) > 0 {
		return nil, nil, errs
	}

	// TODO:
	// SetCharm equivalent method called here
	// AddApplication equivalent method called here.

	// Last step, add pending resources.
	pendingResourceUploads, errs := addPendingResources()

	return nil, pendingResourceUploads, errs
}

// addPendingResource adds a pending resource doc for all resources to be
// added when deploying the charm. PendingResourceUpload is only returned
// for local resources which will require the client to upload the
// resource once DeployFromRepository returns. All resources will be
// processed. Errors are not terminal.
// TODO: determine necessary args.
func addPendingResources() ([]*params.PendingResourceUpload, []error) {
	return nil, nil
}

func makeDeployFromRepositoryValidator(modelType state.ModelType, client CharmhubClient) DeployFromRepositoryValidator {
	v := deployFromRepositoryValidator{
		client: client,
	}
	if modelType == state.ModelTypeCAAS {
		return &caasDeployFromRepositoryValidator{
			validator: v,
		}
	}
	return &iaasDeployFromRepositoryValidator{
		validator: v,
	}
}

type deployFromRepositoryValidator struct {
	client CharmhubClient
}

func (v deployFromRepositoryValidator) validate(arg params.DeployFromRepositoryArg) []error {
	return nil
}

type caasDeployFromRepositoryValidator struct {
	validator deployFromRepositoryValidator
}

func (v caasDeployFromRepositoryValidator) ValidateArg(arg params.DeployFromRepositoryArg) []error {
	// TODO: NumUnits
	// TODO: Storage
	// TODO: Warn on use of old kubernetes series in charms
	return v.validator.validate(arg)
}

type iaasDeployFromRepositoryValidator struct {
	validator deployFromRepositoryValidator
}

func (v iaasDeployFromRepositoryValidator) ValidateArg(arg params.DeployFromRepositoryArg) []error {
	// TODO: NumUnits
	// TODO: Storage
	return v.validator.validate(arg)
}
