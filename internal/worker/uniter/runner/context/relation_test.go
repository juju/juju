// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	apiuniter "github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/testhelpers"
	uniterapi "github.com/juju/juju/internal/worker/uniter/api"
	"github.com/juju/juju/internal/worker/uniter/runner/context"
	"github.com/juju/juju/rpc/params"
)

type ContextRelationSuite struct {
	testhelpers.IsolationSuite
	rel     *uniterapi.MockRelation
	relUnit *uniterapi.MockRelationUnit
}

func TestContextRelationSuite(t *testing.T) {
	tc.Run(t, &ContextRelationSuite{})
}

func (s *ContextRelationSuite) setUp(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.rel = uniterapi.NewMockRelation(ctrl)
	s.rel.EXPECT().Id().Return(666)
	s.relUnit = uniterapi.NewMockRelationUnit(ctrl)
	s.relUnit.EXPECT().Relation().Return(s.rel).AnyTimes()
	s.relUnit.EXPECT().Endpoint().Return(apiuniter.Endpoint{Relation: charm.Relation{Name: "server"}})
	return ctrl
}

func (s *ContextRelationSuite) assertSettingsCaching(c *tc.C, members ...string) {
	defer s.setUp(c).Finish()

	s.relUnit.EXPECT().ReadSettings(gomock.Any(), "u/1").Return(params.Settings{"blib": "blob"}, nil)

	cache := context.NewRelationCache(s.relUnit.ReadSettings, members)
	ctx := context.NewContextRelation(s.relUnit, cache, false)

	// Check that uncached settings are read once.
	m, err := ctx.ReadSettings(c.Context(), "u/1")
	c.Assert(err, tc.ErrorIsNil)
	expectSettings := convertMap(map[string]interface{}{"blib": "blob"})
	c.Assert(m, tc.DeepEquals, expectSettings)

	// Check that another call does not hit the api.
	m, err = ctx.ReadSettings(c.Context(), "u/1")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m, tc.DeepEquals, expectSettings)
}

func (s *ContextRelationSuite) TestMemberCaching(c *tc.C) {
	s.assertSettingsCaching(c, "u/1")
}

func (s *ContextRelationSuite) TestNonMemberCaching(c *tc.C) {
	s.assertSettingsCaching(c, []string(nil)...)
}

func convertMap(settingsMap map[string]interface{}) params.Settings {
	result := make(params.Settings)
	for k, v := range settingsMap {
		result[k] = v.(string)
	}
	return result
}

func (s *ContextRelationSuite) TestSuspended(c *tc.C) {
	defer s.setUp(c).Finish()

	s.rel.EXPECT().Suspended().Return(true)
	ctx := context.NewContextRelation(s.relUnit, nil, false)
	c.Assert(ctx.Suspended(), tc.IsTrue)
}

func (s *ContextRelationSuite) TestSetStatus(c *tc.C) {
	defer s.setUp(c).Finish()

	s.rel.EXPECT().SetStatus(gomock.Any(), relation.Suspended).Return(nil)

	ctx := context.NewContextRelation(s.relUnit, nil, false)
	err := ctx.SetStatus(c.Context(), relation.Suspended)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ContextRelationSuite) TestRemoteApplicationName(c *tc.C) {
	defer s.setUp(c).Finish()

	s.rel.EXPECT().OtherApplication().Return("u")

	ctx := context.NewContextRelation(s.relUnit, nil, false)
	c.Assert(ctx.RemoteApplicationName(), tc.Equals, "u")
}
