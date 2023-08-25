// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"context"
	"os"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/controller"
	corebackups "github.com/juju/juju/core/backups"
)

// ControllerConfigGetter is an interface that provides the controller config.
type ControllerConfigGetter interface {
	// ControllerConfig returns the controller config.
	ControllerConfig(context.Context) (controller.Config, error)
}

type backend interface {
	ModelTag() names.ModelTag
	StateServingInfo() (controller.StateServingInfo, error)
}

// NewMetadataState composes a new backup metadata based on the current Juju state.
func NewMetadataState(db backend, controllerUUID, machine, base string) (*corebackups.Metadata, error) {
	hostname, err := os.Hostname()
	if err != nil {
		// If os.Hostname() is not working, something is woefully wrong.
		return nil, errors.Annotatef(err, "could not get hostname (system unstable?)")
	}

	meta := corebackups.NewMetadata()
	meta.Origin.Model = db.ModelTag().Id()
	meta.Origin.Machine = machine
	meta.Origin.Hostname = hostname
	meta.Origin.Base = base

	meta.Controller.UUID = controllerUUID
	return meta, nil
}
