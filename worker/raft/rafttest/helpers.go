// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rafttest

import (
	"reflect"

	"github.com/hashicorp/raft"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/v3/testing"
)

// CheckConfiguration waits some time for the Raft
// to have the expected server configuration.
func CheckConfiguration(c *gc.C, r *raft.Raft, expectedServers []raft.Server) {
	var configuration raft.Configuration
	for a := coretesting.LongAttempt.Start(); a.Next(); {
		f := r.GetConfiguration()
		c.Assert(f.Error(), jc.ErrorIsNil)
		configuration = f.Configuration()
		if reflect.DeepEqual(configuration.Servers, expectedServers) {
			return
		}
	}
	c.Assert(
		configuration.Servers, jc.SameContents, expectedServers,
		gc.Commentf(
			"waited %s and still did not see the expected configuration",
			coretesting.LongAttempt.Total,
		),
	)
}
