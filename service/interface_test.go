// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/juju/service/systemd"
	"github.com/juju/juju/service/upstart"
	"github.com/juju/juju/service/windows"
)

var _ Service = (*upstart.Service)(nil)
var _ Service = (*windows.Service)(nil)
var _ Service = (*systemd.Service)(nil)
