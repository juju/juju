// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewallrules_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/firewallrules"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/testing"
)

type FirewallRulesSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&FirewallRulesSuite{})

func (s *FirewallRulesSuite) TestSetFirewallRule(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "FirewallRules")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "SetFirewallRules")

			args, ok := a.(params.FirewallRuleArgs)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args.Args, gc.HasLen, 1)

			rule := args.Args[0]
			c.Assert(rule.KnownService, gc.Equals, params.SSHRule)
			c.Assert(rule.WhitelistCIDRS, jc.DeepEquals, []string{"192.168.1.0/32"})

			if results, ok := result.(*params.ErrorResults); ok {
				results.Results = []params.ErrorResult{{
					Error: apiservererrors.ServerError(errors.New("fail"))}}
			}
			return nil
		})

	client := firewallrules.NewClient(apiCaller)
	err := client.SetFirewallRule("ssh", []string{"192.168.1.0/32"})
	c.Assert(err, gc.ErrorMatches, "fail")
}

func (s *FirewallRulesSuite) TestSetFirewallRuleFacadeCallError(c *gc.C) {
	msg := "facade failure"
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "FirewallRules")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "SetFirewallRules")
			return errors.New(msg)
		})
	client := firewallrules.NewClient(apiCaller)
	err := client.SetFirewallRule("ssh", nil)
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
}

func (s *FirewallRulesSuite) TestSetFirewallRuleInvalid(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Fail()
			return errors.New("unexpected")
		})

	client := firewallrules.NewClient(apiCaller)
	err := client.SetFirewallRule("foo", []string{"192.168.1.0/32"})
	c.Assert(err, gc.ErrorMatches, `known service "foo" not valid`)
}

func (s *FirewallRulesSuite) TestList(c *gc.C) {
	called := false
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "FirewallRules")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ListFirewallRules")

			called = true
			c.Assert(a, gc.IsNil)

			if results, ok := result.(*params.ListFirewallRulesResults); ok {
				results.Rules = []params.FirewallRule{{
					KnownService:   params.SSHRule,
					WhitelistCIDRS: []string{"192.168.1.0/32"},
				}}
			}
			return nil
		})

	client := firewallrules.NewClient(apiCaller)
	results, err := client.ListFirewallRules()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
	c.Assert(results, jc.DeepEquals, []params.FirewallRule{{
		KnownService:   params.SSHRule,
		WhitelistCIDRS: []string{"192.168.1.0/32"},
	}})
}

func (s *FirewallRulesSuite) TestListError(c *gc.C) {
	called := false
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "FirewallRules")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ListFirewallRules")

			called = true
			return errors.New("fail")
		})

	client := firewallrules.NewClient(apiCaller)
	_, err := client.ListFirewallRules()
	c.Assert(errors.Cause(err), gc.ErrorMatches, "fail")
	c.Assert(called, jc.IsTrue)
}
