// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package syslog

import (
	"os"

	"github.com/juju/errors"

	"github.com/juju/juju/service"
)

const svcName = "rsyslog"

func Restart() error {
	if os.Geteuid() == 0 {
		if err := service.Restart(svcName); err != nil {
			return errors.Annotatef(err, "failed to restart service %q", svcName)
		}
	}
	return errors.Errorf("must be root")
}
