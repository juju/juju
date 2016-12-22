// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"errors"

	jujutesting "github.com/juju/juju/testing"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/network"
)

type BridgeSuite struct {
	testing.IsolationSuite

	bridger     *fakeBridger
	bridgeError error
}

var _ = gc.Suite(&BridgeSuite{})

type fakeBridger struct {
	cfg   network.BridgerConfig
	suite *BridgeSuite
}

var _ network.Bridger = (*fakeBridger)(nil)

func newFakeBridger(s *BridgeSuite) *fakeBridger {
	return &fakeBridger{
		suite: s,
		cfg: network.BridgerConfig{
			Clock: testing.NewClock(jujutesting.ZeroTime()),
		},
	}
}

func (s *BridgeSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpSuite(c)
	s.bridger = newFakeBridger(s)
	s.bridgeError = nil
}

func (s *BridgeSuite) TestBridgeFails(c *gc.C) {
	s.bridgeError = errors.New("bridging failed")
	err := s.bridger.Bridge([]string{"eno1", "eno2"})
	c.Check(err, gc.ErrorMatches, "bridging failed")
}

func (s *BridgeSuite) TestBridgeSuccess(c *gc.C) {
	err := s.bridger.Bridge([]string{"eno1", "eno2"})
	c.Assert(err, jc.ErrorIsNil)
}

func (f *fakeBridger) Bridge(deviceNames []string) error {
	return f.suite.bridgeError
}
