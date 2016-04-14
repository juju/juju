// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package retrystrategy

import (
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/retrystrategy"
)

// NewFacade creates a Facade from a base.APICaller.
// It's a sensible value for ManifoldConfig.NewFacade.
func NewFacade(apiCaller base.APICaller) Facade {
	return retrystrategy.NewClient(apiCaller)
}
