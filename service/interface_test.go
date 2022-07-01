// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/juju/v3/service/systemd"
	"github.com/juju/juju/v3/service/upstart"
)

var _ Service = (*upstart.Service)(nil)
var _ Service = (*systemd.Service)(nil)
