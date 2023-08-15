// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasadmission

import (
	"github.com/juju/juju/apiserver/common"
)

type Facade struct {
	*common.ControllerConfigAPI
}
