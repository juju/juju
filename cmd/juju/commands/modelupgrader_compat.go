// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/api"
)

type newModelUpgraderRootFunc func(ctx context.Context) (api.Connection, error)

// modelUpgraderAPIRoot is a compatibility shim for commands using the
// ModelUpgrader facade.
// Version 1 is the model-based Juju 4 facade. Version 2 restores a
// controller-based facade that can route requests to the target model.
// A best version of 0 means the facade is absent from the probed root.
// This returns a model-based client for version 1, falling back to a
// controller-based client otherwise.
func modelUpgraderAPIRoot(
	ctx context.Context,
	newModelRoot newModelUpgraderRootFunc,
	newControllerRoot newModelUpgraderRootFunc,
) (api.Connection, error) {
	root, err := newModelRoot(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if root.BestFacadeVersion("ModelUpgrader") == 1 {
		return root, nil
	}

	if err := root.Close(); err != nil {
		return nil, errors.Trace(err)
	}

	root, err = newControllerRoot(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return root, nil
}
