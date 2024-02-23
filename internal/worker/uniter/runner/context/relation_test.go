// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	stdcontext "context"

	"github.com/juju/juju/charm"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	apiuniter "github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/core/relation"
	uniterapi "github.com/juju/juju/internal/worker/uniter/api"
	"github.com/juju/juju/internal/worker/uniter/runner/context"
	"github.com/juju/juju/rpc/params"
)

type ContextRelationSuite struct {
	jujutesting.IsolationSuite
	rel     *uniterapi.MockRelation
	relUnit *uniterapi.MockRelationUnit
}

var _ = gc.Suite(&ContextRelationSuite{})

func (s *ContextRelationSuite) setUp(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.rel = uniterapi.NewMockRelation(ctrl)
	s.rel.EXPECT().Id().Return(666)
	s.relUnit = uniterapi.NewMockRelationUnit(ctrl)
	s.relUnit.EXPECT().Relation().Return(s.rel).AnyTimes()
	s.relUnit.EXPECT().Endpoint().Return(apiuniter.Endpoint{Relation: charm.Relation{Name: "server"}})
	return ctrl
}

func (s *ContextRelationSuite) assertSettingsCaching(c *gc.C, members ...string) {
	defer s.setUp(c).Finish()

	s.relUnit.EXPECT().ReadSettings("u/1").Return(params.Settings{"blib": "blob"}, nil)

	cache := context.NewRelationCache(s.relUnit.ReadSettings, members)
	ctx := context.NewContextRelation(s.relUnit, cache)

	// Check that uncached settings are read once.
	m, err := ctx.ReadSettings("u/1")
	c.Assert(err, jc.ErrorIsNil)
	expectSettings := convertMap(map[string]interface{}{"blib": "blob"})
	c.Assert(m, gc.DeepEquals, expectSettings)

	// Check that another call does not hit the api.
	m, err = ctx.ReadSettings("u/1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m, gc.DeepEquals, expectSettings)
}

func (s *ContextRelationSuite) TestMemberCaching(c *gc.C) {
	s.assertSettingsCaching(c, "u/1")
}

func (s *ContextRelationSuite) TestNonMemberCaching(c *gc.C) {
	s.assertSettingsCaching(c, []string(nil)...)
}

func convertMap(settingsMap map[string]interface{}) params.Settings {
	result := make(params.Settings)
	for k, v := range settingsMap {
		result[k] = v.(string)
	}
	return result
}

func (s *ContextRelationSuite) TestSuspended(c *gc.C) {
	defer s.setUp(c).Finish()

	s.rel.EXPECT().Suspended().Return(true)
	ctx := context.NewContextRelation(s.relUnit, nil)
	c.Assert(ctx.Suspended(), jc.IsTrue)
}

func (s *ContextRelationSuite) TestSetStatus(c *gc.C) {
	defer s.setUp(c).Finish()

	s.rel.EXPECT().SetStatus(gomock.Any(), relation.Suspended).Return(nil)

	ctx := context.NewContextRelation(s.relUnit, nil)
	err := ctx.SetStatus(stdcontext.Background(), relation.Suspended)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ContextRelationSuite) TestRemoteApplicationName(c *gc.C) {
	defer s.setUp(c).Finish()

	s.rel.EXPECT().OtherApplication().Return("u")

	ctx := context.NewContextRelation(s.relUnit, nil)
	c.Assert(ctx.RemoteApplicationName(), gc.Equals, "u")
}
