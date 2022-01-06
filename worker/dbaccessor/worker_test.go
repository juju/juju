// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

var _ = gc.Suite(&detectLocalAddressSuite{})

type detectLocalAddressSuite struct{}

func (s *detectLocalAddressSuite) TestDetectLocalDQliteAddr(c *gc.C) {
	addrList := []string{
		"localhost:9999",
		"10.0.0.1:9999",
		"10.0.0.2:9999",
	}

	localAddr, peerAddrs, err := detectLocalDQliteAddr(addrList)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(localAddr, gc.Equals, "localhost:9999")
	c.Assert(peerAddrs, gc.DeepEquals, addrList[1:])
}
