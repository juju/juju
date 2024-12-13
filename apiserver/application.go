// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"
	"io"

	corecharm "github.com/juju/juju/core/charm"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/internal/errors"
)

// ApplicationService is an interface for the application domain service.
type ApplicationService interface {
	// GetCharmArchiveBySHA256Prefix returns a ReadCloser stream for the charm
	// archive who's SHA256 hash starts with the provided prefix.
	//
	// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
	// returned.
	GetCharmArchiveBySHA256Prefix(ctx context.Context, sha256Prefix string) (io.ReadCloser, error)

	// SetCharm persists the charm metadata, actions, config and manifest to
	// state.
	// If there are any non-blocking issues with the charm metadata, actions,
	// config or manifest, a set of warnings will be returned.
	SetCharm(ctx context.Context, args applicationcharm.SetCharmArgs) (corecharm.ID, []string, error)
}

// ApplicationServiceGetter is an interface for getting an ApplicationService.
type ApplicationServiceGetter interface {

	// Application returns the model's application service.
	Application(context.Context) (ApplicationService, error)
}

type applicationServiceGetter struct {
	ctxt httpContext
}

func (a *applicationServiceGetter) Application(ctx context.Context) (ApplicationService, error) {
	domainServices, err := a.ctxt.domainServicesForRequest(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return domainServices.Application(), nil
}
