// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params_test

import (
	stdtesting "testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/testing"
)

// TestPackage integrates the tests into gotest.
func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

type ErrorResultsSuite struct{}

var _ = gc.Suite(&ErrorResultsSuite{})

func (s *ErrorResultsSuite) TestOneError(c *gc.C) {
	for i, test := range []struct {
		results  params.ErrorResults
		errMatch string
	}{
		{
			errMatch: "expected 1 result, got 0",
		}, {
			results: params.ErrorResults{
				[]params.ErrorResult{{nil}},
			},
		}, {
			results: params.ErrorResults{
				[]params.ErrorResult{{nil}, {nil}},
			},
			errMatch: "expected 1 result, got 2",
		}, {
			results: params.ErrorResults{
				[]params.ErrorResult{
					{&params.Error{Message: "test error"}},
				},
			},
			errMatch: "test error",
		},
	} {
		c.Logf("test %d", i)
		err := test.results.OneError()
		if test.errMatch == "" {
			c.Check(err, jc.ErrorIsNil)
		} else {
			c.Check(err, gc.ErrorMatches, test.errMatch)
		}
	}
}

func (s *ErrorResultsSuite) TestCombine(c *gc.C) {
	for i, test := range []struct {
		msg      string
		results  params.ErrorResults
		errMatch string
	}{
		{
			msg: "no results, no error",
		}, {
			msg: "single nil result",
			results: params.ErrorResults{
				[]params.ErrorResult{{nil}},
			},
		}, {
			msg: "multiple nil results",
			results: params.ErrorResults{
				[]params.ErrorResult{{nil}, {nil}},
			},
		}, {
			msg: "one error result",
			results: params.ErrorResults{
				[]params.ErrorResult{
					{&params.Error{Message: "test error"}},
				},
			},
			errMatch: "test error",
		}, {
			msg: "mixed error results",
			results: params.ErrorResults{
				[]params.ErrorResult{
					{&params.Error{Message: "test error"}},
					{nil},
					{&params.Error{Message: "second error"}},
				},
			},
			errMatch: "test error\nsecond error",
		},
	} {
		c.Logf("test %d: %s", i, test.msg)
		err := test.results.Combine()
		if test.errMatch == "" {
			c.Check(err, jc.ErrorIsNil)
		} else {
			c.Check(err, gc.ErrorMatches, test.errMatch)
		}
	}
}

type importSuite struct{}

var _ = gc.Suite(&importSuite{})

func (*importSuite) TestParamsDoesNotDependOnState(c *gc.C) {
	imports := testing.FindJujuCoreImports(c, "github.com/juju/juju/rpc/params")
	for _, i := range imports {
		c.Assert(i, gc.Not(gc.Equals), "state")
	}
}
