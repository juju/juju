// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftlease

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

type RaftLeaseClientSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&RaftLeaseClientSuite{})

// func (s *RaftLeaseClientSuite) TestRequest(c *gc.C) {
// 	client, err := NewClient(Config{})
// 	c.Assert(err, jc.ErrorIsNil)
// 	err = client.Request(context.TODO(), &raftlease.Command{})
// 	c.Assert(err, jc.ErrorIsNil)
// }
