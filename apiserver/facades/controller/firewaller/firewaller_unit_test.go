// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/controller/firewaller"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&RemoteFirewallerSuite{})

type RemoteFirewallerSuite struct {
	coretesting.BaseSuite

	resources  *common.Resources
	authorizer *apiservertesting.FakeAuthorizer
	st         *mockState
	api        *firewaller.FirewallerAPIV4
}

func (s *RemoteFirewallerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}

	s.st = newMockState(coretesting.ModelTag.Id())
	api, err := firewaller.NewFirewallerAPI(s.st, s.resources, s.authorizer, &mockCloudSpecAPI{})
	c.Assert(err, jc.ErrorIsNil)
	s.api = &firewaller.FirewallerAPIV4{FirewallerAPIV3: api, ControllerConfigAPI: common.NewControllerConfig(s.st)}
}

func (s *RemoteFirewallerSuite) TestWatchIngressAddressesForRelations(c *gc.C) {
	db2Relation := newMockRelation(123)
	s.st.relations["remote-db2:db django:db"] = db2Relation

	result, err := s.api.WatchIngressAddressesForRelations(
		params.Entities{Entities: []params.Entity{{
			Tag: names.NewRelationTag("remote-db2:db django:db").String(),
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Changes, jc.SameContents, []string{"1.2.3.4/32"})
	c.Assert(result.Results[0].Error, gc.IsNil)
	c.Assert(result.Results[0].StringsWatcherId, gc.Equals, "1")

	resource := s.resources.Get("1")
	c.Assert(resource, gc.NotNil)
	c.Assert(resource, gc.Implements, new(state.StringsWatcher))

	s.st.CheckCalls(c, []testing.StubCall{
		{"KeyRelation", []interface{}{"remote-db2:db django:db"}},
	})
}

func (s *RemoteFirewallerSuite) TestControllerAPIInfoForModels(c *gc.C) {
	controllerInfo := &mockControllerInfo{
		uuid: "some uuid",
		info: crossmodel.ControllerInfo{
			Addrs:  []string{"1.2.3.4/32"},
			CACert: coretesting.CACert,
		},
	}
	s.st.controllerInfo[coretesting.ModelTag.Id()] = controllerInfo
	result, err := s.api.ControllerAPIInfoForModels(
		params.Entities{Entities: []params.Entity{{
			Tag: coretesting.ModelTag.String(),
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Addresses, jc.SameContents, []string{"1.2.3.4/32"})
	c.Assert(result.Results[0].Error, gc.IsNil)
	c.Assert(result.Results[0].CACert, gc.Equals, coretesting.CACert)
}

func (s *RemoteFirewallerSuite) TestMacaroonForRelations(c *gc.C) {
	mac, err := macaroon.New(nil, "", "")
	c.Assert(err, jc.ErrorIsNil)
	entity := names.NewRelationTag("mysql:db wordpress:db")
	s.st.macaroons[entity] = mac
	result, err := s.api.MacaroonForRelations(
		params.Entities{Entities: []params.Entity{{
			Tag: entity.String(),
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)
	c.Assert(result.Results[0].Result, jc.DeepEquals, mac)
}
