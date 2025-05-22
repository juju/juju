// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package hostkeyreporter_test

import (
	"errors"
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/api/agent/hostkeyreporter"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

type facadeSuite struct {
	testhelpers.IsolationSuite
}

func TestFacadeSuite(t *testing.T) {
	tc.Run(t, &facadeSuite{})
}

func (s *facadeSuite) TestReportKeys(c *tc.C) {
	stub := new(testhelpers.Stub)
	apiCaller := basetesting.APICallerFunc(func(
		objType string, version int,
		id, request string,
		args, response interface{},
	) error {
		c.Check(objType, tc.Equals, "HostKeyReporter")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		stub.AddCall(request, args)
		*response.(*params.ErrorResults) = params.ErrorResults{
			Results: []params.ErrorResult{{
				(*params.Error)(nil),
			}},
		}
		return nil
	})
	facade := hostkeyreporter.NewFacade(apiCaller)

	err := facade.ReportKeys(c.Context(), "42", []string{"rsa", "dsa"})
	c.Assert(err, tc.ErrorIsNil)

	stub.CheckCalls(c, []testhelpers.StubCall{{
		"ReportKeys", []interface{}{params.SSHHostKeySet{
			EntityKeys: []params.SSHHostKeys{{
				Tag:        names.NewMachineTag("42").String(),
				PublicKeys: []string{"rsa", "dsa"},
			}},
		}},
	}})
}

func (s *facadeSuite) TestCallError(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(
		objType string, version int,
		id, request string,
		args, response interface{},
	) error {
		return errors.New("blam")
	})
	facade := hostkeyreporter.NewFacade(apiCaller)

	err := facade.ReportKeys(c.Context(), "42", []string{"rsa", "dsa"})
	c.Assert(err, tc.ErrorMatches, "blam")
}

func (s *facadeSuite) TestInnerError(c *tc.C) {
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

	err := facade.ReportKeys(c.Context(), "42", []string{"rsa", "dsa"})
	c.Assert(err, tc.ErrorMatches, "blam")
}
