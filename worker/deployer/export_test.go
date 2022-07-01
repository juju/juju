// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	apideployer "github.com/juju/juju/v2/api/agent/deployer"
)

func MakeAPIShim(st *apideployer.State) API {
	return &apiShim{st}
}
