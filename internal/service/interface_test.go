// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/juju/internal/service/systemd"
)

var _ Service = (*systemd.Service)(nil)
