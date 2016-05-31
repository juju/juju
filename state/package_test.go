// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	stdtesting "testing"

	"github.com/juju/utils/os"

	coretesting "github.com/juju/juju/testing"
)

func TestPackage(t *stdtesting.T) {
	// At this stage, Juju only supports running the apiservers and database
	// on Ubuntu. If we end up officially supporting CentOS, then we should
	// make sure we run the tests there.
	if os.HostOS() == os.Ubuntu {
		coretesting.MgoTestPackage(t)
	}
}
