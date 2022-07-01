// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environupgrader

import (
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/controller/environupgrader"
)

func NewFacade(apiCaller base.APICaller) (Facade, error) {
	facade := environupgrader.NewClient(apiCaller)
	return facade, nil
}
