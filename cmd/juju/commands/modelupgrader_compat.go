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
// Facade versions > 0 were added as part of the Juju 4
// Dqlite migration, and selectively depend on model scope for their service
// backings.
// Version 0 of the facade was a controller-only facade.
// This sniffs the best facade version and returns a model-based client if > 0,
// falling back to the legacy controller-based client otherwise.
func modelUpgraderAPIRoot(
	ctx context.Context,
	newModelRoot newModelUpgraderRootFunc,
	newControllerRoot newModelUpgraderRootFunc,
) (api.Connection, error) {
	root, err := newModelRoot(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if root.BestFacadeVersion("ModelUpgrader") > 0 {
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
