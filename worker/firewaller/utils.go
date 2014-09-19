// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"github.com/juju/loggo"

	"github.com/juju/juju/state/watcher"
)

var logger = loggo.GetLogger("juju.worker.firewaller")

// stop a watcher with logging of a possible error.
func stop(what string, stopper watcher.Stopper) {
	if err := stopper.Stop(); err != nil {
		logger.Errorf("error stopping %s: %v", what, err)
	}
}
