// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/charmhub/transport"
	"github.com/juju/names/v4"
)

// CharmhubClient represents a way for querying the charmhub api for information
// about the application charm.
type CharmhubClient interface {
	Refresh(ctx context.Context, config charmhub.RefreshConfig) ([]transport.RefreshResponse, error)
}

// UpdateSeries defines an interface for interacting with updating a series.
type UpdateSeries interface {

	// UpdateSeries attempts to update an application series for deploying new
	// units.
	UpdateSeries(string, string, bool) error
}

// UpdateSeriesState defines a common set of functions for retrieving state
// objects.
type UpdateSeriesState interface {
	// ApplicationsFromMachine returns a list of all the applications for a
	// given machine. This includes all the subordinates.
	Application(string) (Application, error)
}

// UpdateSeriesValidator defines an application validator.
type UpdateSeriesValidator interface {
	// ValidateApplication attempts to validate an application for
	// a given series. Using force to allow the overriding of the error to
	// ensure all application validate.
	//
	// I do question if you actually need to validate anything if force is
	// employed here?
	ValidateApplication(application Application, series string, force bool) error
}

// UpdateSeriesAPI provides the update series API facade for any given version.
// It is expected that any API parameter changes should be performed before
// entering the API.
type UpdateSeriesAPI struct {
	state     UpdateSeriesState
	validator UpdateSeriesValidator
}

// NewUpdateSeriesAPI creates a new UpdateSeriesAPI
func NewUpdateSeriesAPI(
	state UpdateSeriesState,
	validator UpdateSeriesValidator,
) *UpdateSeriesAPI {
	return &UpdateSeriesAPI{
		state:     state,
		validator: validator,
	}
}

func (a *UpdateSeriesAPI) UpdateSeries(tag, series string, force bool) error {
	if series == "" {
		return errors.BadRequestf("series missing from args")
	}
	applicationTag, err := names.ParseApplicationTag(tag)
	if err != nil {
		return errors.Trace(err)
	}
	app, err := a.state.Application(applicationTag.Id())
	if err != nil {
		return errors.Trace(err)
	}
	if !app.IsPrincipal() {
		return &params.Error{
			Message: fmt.Sprintf("%q is a subordinate application, update-series not supported", applicationTag.Id()),
			Code:    params.CodeNotSupported,
		}
	}

	if err := a.validator.ValidateApplication(app, series, force); err != nil {
		return errors.Trace(err)
	}

	return app.UpdateApplicationSeries(series, force)
}

type updateSeriesValidator struct{}

func (s updateSeriesValidator) ValidateApplication(app Application, series string, force bool) error {
	return nil
}
