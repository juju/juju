// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent // not agent_test for no good reason

import (
	stdtesting "testing"

	"github.com/juju/juju/component/all"
	coretesting "github.com/juju/juju/testing"
)

func init() {
	// Required for resources support.
	if err := all.RegisterForServer(); err != nil {
		panic(err)
	}
}

func TestPackage(t *stdtesting.T) {
	// TODO(waigani) 2014-03-19 bug 1294458
	// Refactor to use base suites
	coretesting.MgoTestPackage(t)
}
