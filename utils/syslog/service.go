// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package syslog

import (
	"os"

	"github.com/juju/errors"

	"github.com/juju/juju/service"
)

const svcName = "rsyslog"

// These are patched out during tests.
var (
	getEuid = func() int {
		return os.Geteuid()
	}
	restart = func(name string) error {
		return service.Restart(name)
	}
)

func Restart() error {
	if getEuid() == 0 {
		if err := restart(svcName); err != nil {
			return errors.Annotatef(err, "failed to restart service %q", svcName)
		}
		return nil
	}
	return errors.Errorf("must be root")
}
