// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upstart

import (
	"github.com/juju/juju/service/initsystems"
)

var _ initsystems.InitSystem = (*upstart)(nil)
