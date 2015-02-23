// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package windows

import (
	"github.com/juju/juju/service/initsystems"
)

var _ initsystems.InitSystem = (*windows)(nil)
