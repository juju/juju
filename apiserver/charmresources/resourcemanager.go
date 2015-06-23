// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmresources

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmresources"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.charmresources")

func init() {
	common.RegisterStandardFacade("ResourceManager", 1, NewResourceManagerAPI)
}

// ResourceManager defines the methods on the ResourceManager API end point.
type ResourceManager interface {
	ResourceList(arg params.ResourceFilterParams) (params.ListResourcesResult, error)
	ResourceDelete(arg params.ResourceFilterParams) (params.ErrorResults, error)
}

// ResourceManagerAPI provides access to the ResourceManager API facade.
type ResourceManagerAPI struct {
	st         managerState
	check      *common.BlockChecker
	resources  *common.Resources
	authorizer common.Authorizer
	canWrite   func() bool
}

var _ ResourceManager = (*ResourceManagerAPI)(nil)

func createAPI(st managerState, resources *common.Resources, authorizer common.Authorizer) (*ResourceManagerAPI, error) {
	// Only clients can access the resource manager service.
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}
	owner, err := st.EnvOwner()
	if err != nil {
		return nil, errors.Trace(err)
	}
	// For gccgo interface comparisons, we need a Tag.
	ownerName := names.Tag(owner)
	// For now, only environment owners can delete resources.
	canWrite := func() bool {
		return authorizer.GetAuthTag() == ownerName
	}
	return &ResourceManagerAPI{
		st:         st,
		check:      common.NewBlockChecker(st),
		resources:  resources,
		authorizer: authorizer,
		canWrite:   canWrite,
	}, nil
}

// NewResourceManagerAPI creates a new server-side resource manager API end point.
func NewResourceManagerAPI(st *state.State, resources *common.Resources, authorizer common.Authorizer) (*ResourceManagerAPI, error) {
	return createAPI(getState(st), resources, authorizer)
}

// ResourceList is implemented on charmresources.ResourceManager
func (api *ResourceManagerAPI) ResourceList(arg params.ResourceFilterParams) (params.ListResourcesResult, error) {
	var result params.ListResourcesResult
	manager := api.st.ResourceManager()

	// If no filter terms, we effectively ask for everything.
	filterTerms := arg.Resources
	if len(filterTerms) == 0 {
		filterTerms = append(filterTerms, params.ResourceParams{})
	}
	for _, filterTerm := range filterTerms {
		filter := charmresources.ResourceAttributes{
			User:     filterTerm.User,
			Org:      filterTerm.Org,
			Stream:   filterTerm.User,
			Series:   filterTerm.Series,
			PathName: filterTerm.PathName,
			Revision: filterTerm.Revision,
		}
		metadata, err := manager.ResourceList(filter)
		if err != nil {
			return result, common.ServerError(err)
		}
		for _, m := range metadata {
			result.Resources = append(result.Resources, params.ResourceMetadata{
				ResourcePath: m.Path,
				Size:         m.Size,
				Created:      m.Created,
			})
		}
	}
	return result, nil
}

// ResourceDelete is implemented on charmresources.ResourceManager
func (api *ResourceManagerAPI) ResourceDelete(arg params.ResourceFilterParams) (params.ErrorResults, error) {
	// Permission checks
	if !api.canWrite() {
		return params.ErrorResults{}, common.ErrPerm
	}
	if err := api.check.RemoveAllowed(); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	if len(arg.Resources) == 0 {
		return params.ErrorResults{}, errors.New("no resources specified to delete")
	}

	// Delete each set of matching resources in turn.
	var result params.ErrorResults
	result.Results = make([]params.ErrorResult, len(arg.Resources))
	manager := api.st.ResourceManager()
	for i, resourceSpec := range arg.Resources {
		attrs := charmresources.ResourceAttributes{
			User:     resourceSpec.User,
			Org:      resourceSpec.Org,
			Stream:   resourceSpec.User,
			Series:   resourceSpec.Series,
			PathName: resourceSpec.PathName,
			Revision: resourceSpec.Revision,
		}
		// Grab the path of the resource to delete.
		resourcePath, err := charmresources.ResourcePath(attrs)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		// Check resource exists.
		res, err := manager.ResourceList(attrs)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}

		if len(res) != 1 {
			result.Results[i].Error = common.ServerError(
				errors.NotFoundf("resource %s", resourcePath))
			continue
		}
		// No ready to delete.
		logger.Infof("deleting resource with metadata %+v", res[0])
		err = manager.ResourceDelete(resourcePath)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
	}
	return result, nil
}
