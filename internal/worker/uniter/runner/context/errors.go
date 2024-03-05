// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"github.com/juju/errors"
)

var ErrRequeueAndReboot = errors.New("reboot now")
var ErrReboot = errors.New("reboot after hook")
