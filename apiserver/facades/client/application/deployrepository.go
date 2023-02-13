// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/errors"
	"github.com/juju/featureflag"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/rpc/params"
)

// DeployFromRepository is a one-stop deployment method for repository
// charms. Only a charm name is required to deploy. If argument validation
// fails, a list of all errors found in validation will be returned. If a
// local resource is provided, details required for uploading the validated
// resource will be returned.
func (api *APIBase) DeployFromRepository(args params.DeployFromRepositoryArgs) (params.DeployFromRepositoryResults, error) {
	if !featureflag.Enabled(feature.ServerSideCharmDeploy) {
		return params.DeployFromRepositoryResults{}, errors.NotImplementedf("this facade method is under develop")
	}

	if err := api.checkCanWrite(); err != nil {
		return params.DeployFromRepositoryResults{}, errors.Trace(err)
	}
	if err := api.check.RemoveAllowed(); err != nil {
		return params.DeployFromRepositoryResults{}, errors.Trace(err)
	}

	results := make([]params.DeployFromRepositoryResult, len(args.Args))
	for i, entity := range args.Args {
		info, pending, errs := api.deployOneFromRepository(entity)
		if len(errs) > 0 {
			results[i].Errors = apiservererrors.ServerErrors(errs)
			continue
		}
		results[i].Info = info
		results[i].PendingResourceUploads = pending
	}
	return params.DeployFromRepositoryResults{
		Results: results,
	}, nil
}

func (api *APIBase) deployOneFromRepository(arg params.DeployFromRepositoryArg) ([]string, []*params.PendingResourceUpload, []error) {
	// Validate the args.
	errs := validateDeployFromRepositoryArgs(arg)
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

// validateDeployFromRepositoryArgs does validation of all provided
// arguments.
func validateDeployFromRepositoryArgs(_ params.DeployFromRepositoryArg) []error {
	// Are we deploying a charm? if not, fail fast here.
	// TODO: add a ErrorNotACharm or the like for the juju client.

	// Validate the other args.
	return nil
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
