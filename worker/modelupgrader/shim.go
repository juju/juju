// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelupgrader

import (
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/modelupgrader"
)

func NewFacade(apiCaller base.APICaller) (Facade, error) {
	facade := modelupgrader.NewClient(apiCaller)
	return facade, nil
}
