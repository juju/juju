// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state/apiserver/common"
)

type addresserSuite struct {
	testing.JujuConnSuite
	addresser *common.Addresser
}

var _ = gc.Suite(&addresserSuite{})

func (s *addresserSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.addresser = common.NewAddresser(s.State)
}

func (s *addresserSuite) TestStateAddresses(c *gc.C) {
	stateAddresses, err := s.State.Addresses()
	c.Assert(err, gc.IsNil)

	result, err := s.addresser.StateAddresses()
	c.Assert(err, gc.IsNil)
	c.Assert(result.Result, gc.DeepEquals, stateAddresses)
}

func (s *addresserSuite) TestAPIAddresses(c *gc.C) {
	apiInfo := s.APIInfo(c)

	result, err := s.addresser.APIAddresses()
	c.Assert(err, gc.IsNil)
	c.Assert(result.Result, gc.DeepEquals, apiInfo.Addrs)
}

func (s *addresserSuite) TestCACert(c *gc.C) {
	result := s.addresser.CACert()
	c.Assert(result.Result, gc.DeepEquals, s.State.CACert())
}
