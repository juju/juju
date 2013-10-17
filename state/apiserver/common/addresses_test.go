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
	s.addresser = common.NewAddresser(fakeAddresses{})
}

func (s *addresserSuite) TestStateAddresses(c *gc.C) {
	result, err := s.addresser.StateAddresses()
	c.Assert(err, gc.IsNil)
	c.Assert(result.Result, gc.DeepEquals, []string{"addresses:1", "addresses:2"})
}

func (s *addresserSuite) TestAPIAddresses(c *gc.C) {
	result, err := s.addresser.APIAddresses()
	c.Assert(err, gc.IsNil)
	c.Assert(result.Result, gc.DeepEquals, []string{"apiaddresses:1", "apiaddresses:2"})
}

func (s *addresserSuite) TestCACert(c *gc.C) {
	result := s.addresser.CACert()
	c.Assert(string(result.Result), gc.Equals, "a cert")
}

type fakeAddresses struct{}

func (fakeAddresses) Addresses() ([]string, error) {
	return []string{"addresses:1", "addresses:2"}, nil
}

func (fakeAddresses) APIAddresses() ([]string, error) {
	return []string{"apiaddresses:1", "apiaddresses:2"}, nil
}

func (fakeAddresses) CACert() []byte {
	return []byte("a cert")
}