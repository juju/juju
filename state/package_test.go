// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"testing"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/utils/os"
)

func TestPackage(t *testing.T) {
	// At this stage, Juju only supports running the apiservers and database
	// on Ubuntu. If we end up officially supporting CentOS, then we should
	// make sure we run the tests there.
	if os.HostOS() != os.Ubuntu {
		t.Skipf("skipping tests on %v", os.HostOS())
	}
	coretesting.MgoTestPackage(t)
}
