// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package base

import (
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type BasesSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&BasesSuite{})

func (s *BasesSuite) TestWorkloadBases(c *gc.C) {
	tests := []struct {
		name          string
		requestedBase Base
		imageStream   string
		err           string
		expectedBase  []Base
	}{{
		name:          "no base",
		requestedBase: Base{},
		imageStream:   Daily,
		expectedBase: []Base{
			MustParseBaseFromString("centos@7/stable"),
			MustParseBaseFromString("centos@9/stable"),
			MustParseBaseFromString("genericlinux@genericlinux/stable"),
			MustParseBaseFromString("ubuntu@20.04/stable"),
			MustParseBaseFromString("ubuntu@22.04/stable"),
			MustParseBaseFromString("ubuntu@24.04/stable"),
		},
	}, {
		name:          "requested base",
		requestedBase: MustParseBaseFromString("ubuntu@22.04"),
		imageStream:   Daily,
		expectedBase: []Base{
			MustParseBaseFromString("centos@7/stable"),
			MustParseBaseFromString("centos@9/stable"),
			MustParseBaseFromString("genericlinux@genericlinux/stable"),
			MustParseBaseFromString("ubuntu@20.04/stable"),
			MustParseBaseFromString("ubuntu@22.04/stable"),
			MustParseBaseFromString("ubuntu@24.04/stable"),
		},
	}, {
		name:          "invalid base",
		requestedBase: MustParseBaseFromString("foo@bar"),
		imageStream:   Daily,
		err:           `os "foo" version "bar" not found`,
	}}
	for _, test := range tests {
		c.Logf("test %q", test.name)

		result, err := WorkloadBases(time.Now(), test.requestedBase, test.imageStream)
		if test.err != "" {
			c.Assert(err, gc.ErrorMatches, test.err)
			continue
		}
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(result, gc.DeepEquals, test.expectedBase)
	}
}
