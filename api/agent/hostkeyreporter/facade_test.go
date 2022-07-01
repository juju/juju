// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package hostkeyreporter_test

import (
	"errors"

	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v2/api/agent/hostkeyreporter"
	basetesting "github.com/juju/juju/v2/api/base/testing"
	"github.com/juju/juju/v2/rpc/params"
)

type facadeSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&facadeSuite{})

func (s *facadeSuite) TestReportKeys(c *gc.C) {
	stub := new(testing.Stub)
	apiCaller := basetesting.APICallerFunc(func(
		objType string, version int,
		id, request string,
		args, response interface{},
	) error {
		c.Check(objType, gc.Equals, "HostKeyReporter")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		stub.AddCall(request, args)
		*response.(*params.ErrorResults) = params.ErrorResults{
			Results: []params.ErrorResult{{
				(*params.Error)(nil),
			}},
		}
		return nil
	})
	facade := hostkeyreporter.NewFacade(apiCaller)

	err := facade.ReportKeys("42", []string{"rsa", "dsa"})
	c.Assert(err, jc.ErrorIsNil)

	stub.CheckCalls(c, []testing.StubCall{{
		"ReportKeys", []interface{}{params.SSHHostKeySet{
			EntityKeys: []params.SSHHostKeys{{
				Tag:        names.NewMachineTag("42").String(),
				PublicKeys: []string{"rsa", "dsa"},
			}},
		}},
	}})
}

func (s *facadeSuite) TestCallError(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(
		objType string, version int,
		id, request string,
		args, response interface{},
	) error {
		return errors.New("blam")
	})
	facade := hostkeyreporter.NewFacade(apiCaller)

	err := facade.ReportKeys("42", []string{"rsa", "dsa"})
	c.Assert(err, gc.ErrorMatches, "blam")
}

func (s *facadeSuite) TestInnerError(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(
		objType string, version int,
		id, request string,
		args, response interface{},
	) error {
		*response.(*params.ErrorResults) = params.ErrorResults{
			Results: []params.ErrorResult{{
				&params.Error{Message: "blam"},
			}},
		}
		return nil
	})
	facade := hostkeyreporter.NewFacade(apiCaller)

	err := facade.ReportKeys("42", []string{"rsa", "dsa"})
	c.Assert(err, gc.ErrorMatches, "blam")
}
