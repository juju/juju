// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/juju/v2/service/systemd"
	"github.com/juju/juju/v2/service/upstart"
	"github.com/juju/juju/v2/service/windows"
)

var _ Service = (*upstart.Service)(nil)
var _ Service = (*windows.Service)(nil)
var _ Service = (*systemd.Service)(nil)
