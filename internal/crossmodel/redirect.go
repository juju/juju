// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package crossmodel holds helpers for cross-model relation consumers,
// including following controller redirects when a model has been migrated
// to another controller.
package crossmodel

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/api"
	corecrossmodel "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/network"
)

// ExternalControllerUpdater persists the location of models hosted by an
// external controller. It is implemented by the external controller
// domain service.
type ExternalControllerUpdater interface {
	UpdateExternalController(ctx context.Context, ec corecrossmodel.ControllerInfo) error
}

// ConnectWithRedirect opens an API connection to the controller in apiInfo,
// following at most one redirect if the target model has been migrated to
// another controller.
//
// apiInfo is left unchanged. The returned redirect is non-nil only when a
// redirected connection succeeds; it carries the target controller details
// for persistence by the caller (see SaveMigratedModelController).
// open is the connection factory to use
// (e.g. apicaller.NewExternalControllerConnection); it is required.
func ConnectWithRedirect(
	ctx context.Context,
	apiInfo *api.Info,
	open func(context.Context, *api.Info) (api.Connection, error),
) (api.Connection, *api.RedirectError, error) {
	conn, err := open(ctx, apiInfo)
	if err == nil {
		return conn, nil, nil
	}

	var redirectErr *api.RedirectError
	if !errors.As(err, &redirectErr) {
		return nil, nil, errors.Trace(err)
	}

	// The model was migrated to another controller; retry against the
	// redirected addresses without changing the caller's connection details.
	redirectedInfo := *apiInfo
	redirectedInfo.Addrs = network.CollapseToHostPorts(redirectErr.Servers).Strings()
	redirectedInfo.CACert = redirectErr.CACert

	conn, err = open(ctx, &redirectedInfo)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	return conn, redirectErr, nil
}

// SaveMigratedModelController persists the new location of a model that
// was redirected (migrated) to another controller, so future connections
// and controller lookups for the model go directly to the new controller.
// The redirect must carry a valid controller tag and addresses; nothing
// is saved and an error is returned otherwise.
func SaveMigratedModelController(
	ctx context.Context,
	updater ExternalControllerUpdater,
	redirect *api.RedirectError,
	modelUUID string,
) error {
	controllerInfo := corecrossmodel.ControllerInfo{
		ControllerUUID: redirect.ControllerTag.Id(),
		Alias:          redirect.ControllerAlias,
		Addrs:          network.CollapseToHostPorts(redirect.Servers).Strings(),
		CACert:         redirect.CACert,
		ModelUUIDs:     []string{modelUUID},
	}
	if err := controllerInfo.Validate(); err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(updater.UpdateExternalController(ctx, controllerInfo))
}
