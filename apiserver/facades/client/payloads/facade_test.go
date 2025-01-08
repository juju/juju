// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package payloads_test

import (
	"context"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/client/payloads"
	"github.com/juju/juju/rpc/params"
)

type Suite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&Suite{})

func (s *Suite) TestListV1(c *gc.C) {
	facade := payloads.NewAPIV1()
	args := params.PayloadListArgs{
		Patterns: []string{},
	}
	results, err := facade.List(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(results, jc.DeepEquals, params.PayloadListResults{})
}

func (s *Suite) TestListV2(c *gc.C) {
	// There is no v2 of the API, so the test is superfluous.
}
