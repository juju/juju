// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package windows

import (
	"github.com/juju/juju/service/initsystems"
)

func NewWindows(fops fileOperations, cmd cmdRunner) initsystems.InitSystem {
	return &windows{
		name: "windows",
		fops: fops,
		cmd:  cmd,
	}
}
