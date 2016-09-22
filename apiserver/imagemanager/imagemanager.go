// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemanager

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/imagestorage"
)

var logger = loggo.GetLogger("juju.apiserver.imagemanager")

func init() {
	common.RegisterStandardFacade("ImageManager", 2, NewImageManagerAPI)
}

// ImageManager defines the methods on the imagemanager API end point.
type ImageManager interface {
	ListImages(arg params.ImageFilterParams) (params.ListImageResult, error)
	DeleteImages(arg params.ImageFilterParams) (params.ErrorResults, error)
}

// ImageManagerAPI implements the ImageManager interface and is the concrete
// implementation of the api end point.
type ImageManagerAPI struct {
	state      stateInterface
	resources  facade.Resources
	authorizer facade.Authorizer
	check      *common.BlockChecker
}

var _ ImageManager = (*ImageManagerAPI)(nil)

var getState = func(st *state.State) stateInterface {
	return stateShim{st}
}

// NewImageManagerAPI creates a new server-side imagemanager API end point.
func NewImageManagerAPI(st *state.State, resources facade.Resources, authorizer facade.Authorizer) (*ImageManagerAPI, error) {
	// Only clients can access the image manager service.
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}
	return &ImageManagerAPI{
		state:      getState(st),
		resources:  resources,
		authorizer: authorizer,
		check:      common.NewBlockChecker(st),
	}, nil
}

// ListImages returns images matching the specified filter.
func (api *ImageManagerAPI) ListImages(arg params.ImageFilterParams) (params.ListImageResult, error) {
	var result params.ListImageResult
	admin, err := api.authorizer.HasPermission(permission.SuperuserAccess, api.state.ControllerTag())
	if err != nil {
		return result, errors.Trace(err)
	}
	if !admin {
		return result, common.ServerError(common.ErrPerm)
	}

	if len(arg.Images) > 1 {
		return result, errors.New("image filter with multiple terms not supported")
	}
	filter := imagestorage.ImageFilter{}
	if len(arg.Images) == 1 {
		filter = imagestorage.ImageFilter{
			Kind:   arg.Images[0].Kind,
			Series: arg.Images[0].Series,
			Arch:   arg.Images[0].Arch,
		}
	}
	stor := api.state.ImageStorage()
	metadata, err := stor.ListImages(filter)
	if err != nil {
		return result, nil
	}
	result.Result = make([]params.ImageMetadata, len(metadata))
	for i, m := range metadata {
		result.Result[i] = params.ImageMetadata{
			Kind:    m.Kind,
			Series:  m.Series,
			Arch:    m.Arch,
			URL:     m.SourceURL,
			Created: m.Created,
		}
	}
	return result, nil
}

// DeleteImages deletes the images matching the specified filter.
func (api *ImageManagerAPI) DeleteImages(arg params.ImageFilterParams) (params.ErrorResults, error) {
	var result params.ErrorResults
	admin, err := api.authorizer.HasPermission(permission.SuperuserAccess, api.state.ControllerTag())
	if err != nil {
		return result, errors.Trace(err)
	}
	if !admin {
		return result, common.ServerError(common.ErrPerm)
	}

	if err := api.check.ChangeAllowed(); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	result.Results = make([]params.ErrorResult, len(arg.Images))
	stor := api.state.ImageStorage()
	for i, imageSpec := range arg.Images {
		filter := imagestorage.ImageFilter{
			Kind:   imageSpec.Kind,
			Series: imageSpec.Series,
			Arch:   imageSpec.Arch,
		}
		imageMetadata, err := stor.ListImages(filter)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		if len(imageMetadata) != 1 {
			result.Results[i].Error = common.ServerError(
				errors.NotFoundf("image %s/%s/%s", filter.Kind, filter.Series, filter.Arch))
			continue
		}
		logger.Infof("deleting image with metadata %+v", *imageMetadata[0])
		err = stor.DeleteImage(imageMetadata[0])
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
		}
	}
	return result, nil
}
