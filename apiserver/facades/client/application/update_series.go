// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"context"
	"fmt"

	"github.com/juju/charm/v9"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/charmhub/transport"
	corecharm "github.com/juju/juju/core/charm"
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
	// Application returns a list of all the applications for a
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

type updateSeriesValidator struct {
	localValidator  UpdateSeriesValidator
	removeValidator UpdateSeriesValidator
}

func makeUpdateSeriesValidator(client CharmhubClient) updateSeriesValidator {
	return updateSeriesValidator{
		localValidator: stateSeriesValidator{},
		removeValidator: charmhubSeriesValidator{
			client: client,
		},
	}
}

func (s updateSeriesValidator) ValidateApplication(app Application, series string, force bool) error {
	// This is not a charmhub charm, so we can fallback to querying state
	// for the supported series.
	if origin := app.CharmOrigin(); origin == nil || !corecharm.CharmHub.Matches(origin.Source) {
		return s.localValidator.ValidateApplication(app, series, force)
	}

	return s.removeValidator.ValidateApplication(app, series, force)
}

// stateSeriesValidator validates an application using the state (database)
// version of the charm.
type stateSeriesValidator struct{}

// ValidateApplications attempts to validate a series of applications for
// a given series.
func (s stateSeriesValidator) ValidateApplication(application Application, series string, force bool) error {
	ch, _, err := application.Charm()
	if err != nil {
		return errors.Trace(err)
	}
	supportedSeries, err := corecharm.ComputedSeries(ch)
	if err != nil {
		return errors.Trace(err)
	}
	if len(supportedSeries) == 0 {
		supportedSeries = append(supportedSeries, ch.URL().Series)
	}
	_, seriesSupportedErr := charm.SeriesForCharm(series, supportedSeries)
	if seriesSupportedErr != nil && !force {
		// TODO (stickupkid): Once all commands are placed in this API, we
		// should relocate these to the API server.
		return apiservererrors.NewErrIncompatibleSeries(supportedSeries, series, ch.String())
	}
	return nil
}

type charmhubSeriesValidator struct {
	client CharmhubClient
}

// ValidateApplications attempts to validate a series of applications for
// a given series.
func (s charmhubSeriesValidator) ValidateApplication(application Application, series string, force bool) error {
	// We can be assured that the charm origin is not nil, because we
	// guarded against it before.
	origin := application.CharmOrigin()
	rev := origin.Revision
	if rev == nil {
		return errors.Errorf("no revision found for application %q", application.Name())
	}

	base := charmhub.RefreshBase{
		Architecture: origin.Platform.Architecture,
		Name:         origin.Platform.OS,
		Channel:      series,
	}
	cfg, err := charmhub.DownloadOneFromRevision(origin.ID, *rev, base)
	if err != nil {
		return errors.Trace(err)
	}

	refreshResp, err := s.client.Refresh(context.TODO(), cfg)
	if err != nil {
		return errors.Trace(err)
	}
	if len(refreshResp) != 1 {
		return errors.Errorf("unexpected number of responses %d for applications 1", len(refreshResp))
	}
	for _, resp := range refreshResp {
		if err := resp.Error; err != nil && !force {
			return errors.Annotatef(err, "unable to locate application with series %s", series)
		}
	}
	return nil
}
