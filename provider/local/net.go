// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package local

import (
	"launchpad.net/juju-core/utils"
)

// getAddressForInterface is a variable so we can change the implementation
// for testing purposes.
var getAddressForInterface = utils.GetAddressForInterface
