// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateenvirons_test

import (
	"testing"

	"github.com/juju/juju/core/os"
	"github.com/juju/juju/core/os/ostype"
	coretesting "github.com/juju/juju/testing"
)

func TestPackage(t *testing.T) {
	// At this stage, Juju only supports running the apiservers and database
	// on Ubuntu. If we end up officially supporting CentOS, then we should
	// make sure we run the tests there.
	if os.HostOS() != ostype.Ubuntu {
		t.Skipf("skipping tests on %v", os.HostOS())
	}
	coretesting.MgoTestPackage(t)
}
