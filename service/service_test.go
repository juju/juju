// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/juju/service/systemd"
	"github.com/juju/juju/service/upstart"
	"github.com/juju/juju/service/windows"
)

var _ Service = (*systemd.Service)(nil)
var _ Service = (*upstart.Service)(nil)
var _ Service = (*windows.Service)(nil)
