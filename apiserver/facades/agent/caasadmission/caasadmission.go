// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasadmission

import (
	"github.com/juju/juju/v3/apiserver/common"
	"github.com/juju/juju/v3/apiserver/facade"
)

type Facade struct {
	auth facade.Authorizer
	*common.ControllerConfigAPI
}
