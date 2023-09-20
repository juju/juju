// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/charmhub/transport"
	corebase "github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// CharmhubClient represents a way for querying the charmhub api for information
// about the application charm.
type CharmhubClient interface {
	Refresh(ctx context.Context, config charmhub.RefreshConfig) ([]transport.RefreshResponse, error)
}

// UpdateBase defines an interface for interacting with updating a base.
type UpdateBase interface {

	// UpdateBase attempts to update an application base for deploying new
	// units.
	UpdateBase(string, corebase.Base, bool) error
}

// UpdateBaseState defines a common set of functions for retrieving state
// objects.
type UpdateBaseState interface {
	// Application returns a list of all the applications for a
	// given machine. This includes all the subordinates.
	Application(string) (Application, error)
}

// UpdateBaseValidator defines an application validator.
type UpdateBaseValidator interface {
	// ValidateApplication attempts to validate an application for
	// a given base. Using force to allow the overriding of the error to
	// ensure all application validate.
	//
	// I do question if you actually need to validate anything if force is
	// employed here?
	ValidateApplication(application Application, base corebase.Base, force bool) error
}

// UpdateBaseAPI provides the update series API facade for any given version.
// It is expected that any API parameter changes should be performed before
// entering the API.
type UpdateBaseAPI struct {
	state     UpdateBaseState
	validator UpdateBaseValidator
}

// NewUpdateBaseAPI creates a new UpdateBaseAPI
func NewUpdateBaseAPI(
	state UpdateBaseState,
	validator UpdateBaseValidator,
) *UpdateBaseAPI {
	return &UpdateBaseAPI{
		state:     state,
		validator: validator,
	}
}

func (a *UpdateBaseAPI) UpdateBase(tag string, base corebase.Base, force bool) error {
	if base.String() == "" {
		return errors.BadRequestf("base missing from args")
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

	if err := a.validator.ValidateApplication(app, base, force); err != nil {
		return errors.Trace(err)
	}

	return app.UpdateApplicationBase(state.Base{
		OS: base.OS, Channel: base.Channel.String(),
	}, force)
}

type updateSeriesValidator struct {
	localValidator  UpdateBaseValidator
	remoteValidator UpdateBaseValidator
}

func makeUpdateSeriesValidator(client CharmhubClient) updateSeriesValidator {
	return updateSeriesValidator{
		localValidator: stateSeriesValidator{},
		remoteValidator: charmhubSeriesValidator{
			client: client,
		},
	}
}

func (s updateSeriesValidator) ValidateApplication(app Application, base corebase.Base, force bool) error {
	// This is not a charmhub charm, so we can fallback to querying state
	// for the supported series.
	if origin := app.CharmOrigin(); origin == nil || !corecharm.CharmHub.Matches(origin.Source) {
		return s.localValidator.ValidateApplication(app, base, force)
	}

	return s.remoteValidator.ValidateApplication(app, base, force)
}

// stateSeriesValidator validates an application using the state (database)
// version of the charm.
// NOTE: stateSeriesValidator also exists in apiserver/facades/client/machinemanager/upgrade_series.go,
// When making changes here, review the copy for required changes as well.
type stateSeriesValidator struct{}

// ValidateApplication attempts to validate an applications for
// a given base.
func (s stateSeriesValidator) ValidateApplication(application Application, base corebase.Base, force bool) error {
	ch, _, err := application.Charm()
	if err != nil {
		return errors.Trace(err)
	}
	supportedBases, err := corecharm.ComputedBases(ch)
	if err != nil {
		return errors.Trace(err)
	}
	if len(supportedBases) == 0 {
		err := errors.NewNotSupported(nil, fmt.Sprintf("charm %q does not support any bases. Not valid", ch.Meta().Name))
		return apiservererrors.ServerError(err)
	}
	_, baseSupportedErr := corecharm.BaseForCharm(base, supportedBases)
	if baseSupportedErr != nil && !force {
		return apiservererrors.NewErrIncompatibleBase(supportedBases, base, ch.Meta().Name)
	}
	return nil
}

// NOTE: charmhubSeriesValidator also exists in apiserver/facades/client/machinemanager/upgrade_series.go,
// When making changes here, review the copy for required changes as well.
type charmhubSeriesValidator struct {
	client CharmhubClient
}

// ValidateApplication attempts to validate an application for
// a given base.
func (s charmhubSeriesValidator) ValidateApplication(application Application, base corebase.Base, force bool) error {
	// We can be assured that the charm origin is not nil, because we
	// guarded against it before.
	origin := application.CharmOrigin()
	rev := origin.Revision
	if rev == nil {
		return errors.Errorf("no revision found for application %q", application.Name())
	}

	cfg, err := charmhub.DownloadOneFromRevision(origin.ID, *rev)
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
			return errors.Annotatef(err, "unable to locate application with base %s", base.DisplayString())
		}
	}
	// DownloadOneFromRevision does not take a base, however the response contains the bases
	// supported by the given revision.  Validate against provided series.
	channelToValidate := base.Channel.Track
	for _, resp := range refreshResp {
		var found bool
		for _, base := range resp.Entity.Bases {
			if channelToValidate == base.Channel || force {
				found = true
				break
			}
		}
		if !found {
			return errors.Errorf("charm %q does not support %s, force not used", resp.Name, base.DisplayString())
		}
	}
	return nil
}
