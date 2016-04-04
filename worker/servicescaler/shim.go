// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package servicescaler

import (
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/servicescaler"
	"github.com/juju/juju/api/watcher"
)

// NewFacade creates a Facade from a base.APICaller.
// It's a sensible value for ManifoldConfig.NewFacade.
func NewFacade(apiCaller base.APICaller) (Facade, error) {
	return servicescaler.NewAPI(
		apiCaller,
		watcher.NewStringsWatcher,
	), nil
}
