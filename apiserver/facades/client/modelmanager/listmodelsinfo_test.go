// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/params"
	_ "github.com/juju/juju/provider/azure"
	_ "github.com/juju/juju/provider/ec2"
	_ "github.com/juju/juju/provider/joyent"
	_ "github.com/juju/juju/provider/maas"
	_ "github.com/juju/juju/provider/openstack"
)

func (s *modelManagerStateSuite) TestListModelsWithInfoForSelf(c *gc.C) {
	user := names.NewUserTag("external@remote")
	s.setAPIUser(c, user)
	result, err := s.modelmanager.ListModelsWithInfo(params.Entity{Tag: user.String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 0)
}

func (s *modelManagerStateSuite) TestListModelsWithInfoForSelfLocalUser(c *gc.C) {
	// When the user's credentials cache stores the simple name, but the
	// api server converts it to a fully qualified name.
	user := names.NewUserTag("local-user")
	s.setAPIUser(c, names.NewUserTag("local-user"))
	result, err := s.modelmanager.ListModelsWithInfo(params.Entity{Tag: user.String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 0)
}

func (s *modelManagerStateSuite) TestListModelsWithInfoAdminSelf(c *gc.C) {
	user := s.AdminUserTag(c)
	s.setAPIUser(c, user)
	result, err := s.modelmanager.ListModelsWithInfo(params.Entity{Tag: user.String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	expected, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)

	model := result.Results[0].Result
	c.Check(model.Name, gc.Equals, expected.Name())
	c.Check(model.UUID, gc.Equals, expected.UUID())
	c.Check(model.OwnerTag, gc.Equals, expected.Owner().String())
}

func (s *modelManagerStateSuite) TestListModelsWithInfoAdminListsOther(c *gc.C) {
	user := s.AdminUserTag(c)
	s.setAPIUser(c, user)
	other := names.NewUserTag("admin")
	result, err := s.modelmanager.ListModelsWithInfo(params.Entity{Tag: other.String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
}

func (s *modelManagerStateSuite) TestListModelsWithInfoDenied(c *gc.C) {
	user := names.NewUserTag("external@remote")
	s.setAPIUser(c, user)
	other := names.NewUserTag("other@remote")
	_, err := s.modelmanager.ListModelsWithInfo(params.Entity{Tag: other.String()})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}
