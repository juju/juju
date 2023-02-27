// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewallrules_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basemocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/firewallrules"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/rpc/params"
)

type FirewallRulesSuite struct {
}

var _ = gc.Suite(&FirewallRulesSuite{})

func (s *FirewallRulesSuite) TestSetFirewallRule(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.FirewallRuleArgs{
		Args: []params.FirewallRule{{
			KnownService:   "ssh",
			WhitelistCIDRS: []string{"192.168.1.0/32"},
		}},
	}
	res := new(params.ErrorResults)
	results := params.ErrorResults{
		Results: []params.ErrorResult{{
			Error: apiservererrors.ServerError(errors.New("fail"))}},
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("SetFirewallRules", args, res).SetArg(2, results).Return(nil)
	client := firewallrules.NewClientFromCaller(mockFacadeCaller)

	err := client.SetFirewallRule("ssh", []string{"192.168.1.0/32"})
	c.Assert(err, gc.ErrorMatches, "fail")
}

func (s *FirewallRulesSuite) TestSetFirewallRuleFacadeCallError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	msg := "facade failure"
	args := params.FirewallRuleArgs{
		Args: []params.FirewallRule{{
			KnownService: "ssh",
		}},
	}
	res := new(params.ErrorResults)

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("SetFirewallRules", args, res).Return(errors.New(msg))
	client := firewallrules.NewClientFromCaller(mockFacadeCaller)

	err := client.SetFirewallRule("ssh", nil)
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
}

func (s *FirewallRulesSuite) TestSetFirewallRuleInvalid(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	client := firewallrules.NewClientFromCaller(mockFacadeCaller)

	err := client.SetFirewallRule("foo", []string{"192.168.1.0/32"})
	errors.Is(err, errors.NotValid)
	c.Assert(err, gc.ErrorMatches, `service "foo" not valid`)

}

func (s *FirewallRulesSuite) TestList(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	res := new(params.ListFirewallRulesResults)
	results := params.ListFirewallRulesResults{
		Rules: []params.FirewallRule{{
			KnownService:   params.SSHRule,
			WhitelistCIDRS: []string{"192.168.1.0/32"},
		}},
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("ListFirewallRules", nil, res).SetArg(2, results).Return(nil)
	client := firewallrules.NewClientFromCaller(mockFacadeCaller)

	ress, err := client.ListFirewallRules()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ress, jc.DeepEquals, results.Rules)
}

func (s *FirewallRulesSuite) TestListError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	res := new(params.ListFirewallRulesResults)
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("ListFirewallRules", nil, res).Return(errors.New("fail"))
	client := firewallrules.NewClientFromCaller(mockFacadeCaller)

	_, err := client.ListFirewallRules()
	c.Assert(errors.Cause(err), gc.ErrorMatches, "fail")
}
