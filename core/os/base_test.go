// Copyright 2024 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package os_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/os"
	"github.com/juju/juju/internal/testhelpers"
)

type baseSuite struct {
	testhelpers.CleanupSuite
}

func TestBaseSuite(t *stdtesting.T) { tc.Run(t, &baseSuite{}) }

var b = corebase.Base{OS: "freelunch", Channel: corebase.Channel{Track: "0"}}

func (s *baseSuite) TestHostBaseOverride(c *tc.C) {
	// Really just tests that HostBase is overridable
	s.PatchValue(&os.HostBase, func() (corebase.Base, error) {
		return b, nil
	})
	ser, err := os.HostBase()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ser, tc.Equals, b)
}
