// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"testing"

	"github.com/canonical/lxd/shared/api"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/internal/container/lxd"
	lxdtesting "github.com/juju/juju/internal/container/lxd/testing"
)

type serverSuite struct {
	lxdtesting.BaseSuite
}

func TestServerSuite(t *testing.T) {
	tc.Run(t, &serverSuite{})
}

func (s *serverSuite) TestUpdateServerConfig(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := lxdtesting.NewMockInstanceServer(ctrl)

	updateReq := api.ServerPut{Config: map[string]interface{}{"key1": "val1"}}
	gomock.InOrder(
		cSvr.EXPECT().GetServer().Return(&api.Server{}, lxdtesting.ETag, nil).Times(2),
		cSvr.EXPECT().UpdateServer(updateReq, lxdtesting.ETag).Return(nil),
	)

	jujuSvr, err := lxd.NewServer(cSvr)
	c.Assert(err, tc.ErrorIsNil)

	err = jujuSvr.UpdateServerConfig(map[string]string{"key1": "val1"})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serverSuite) TestUpdateContainerConfig(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := lxdtesting.NewMockInstanceServer(ctrl)

	cName := "juju-lxd-1"
	newConfig := map[string]string{"key1": "val1"}
	updateReq := api.InstancePut{Config: newConfig}
	op := lxdtesting.NewMockOperation(ctrl)
	gomock.InOrder(
		cSvr.EXPECT().GetServer().Return(&api.Server{}, lxdtesting.ETag, nil),
		cSvr.EXPECT().GetInstance(cName).Return(&api.Instance{}, lxdtesting.ETag, nil),
		cSvr.EXPECT().UpdateInstance(cName, updateReq, lxdtesting.ETag).Return(op, nil),
		op.EXPECT().Wait().Return(nil),
	)
	jujuSvr, err := lxd.NewServer(cSvr)
	c.Assert(err, tc.ErrorIsNil)

	err = jujuSvr.UpdateContainerConfig("juju-lxd-1", newConfig)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serverSuite) TestHasProfile(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl)

	cSvr.EXPECT().GetProfileNames().Return([]string{"default", "custom"}, nil).Times(2)

	jujuSvr, err := lxd.NewServer(cSvr)
	c.Assert(err, tc.ErrorIsNil)

	has, err := jujuSvr.HasProfile("default")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(has, tc.IsTrue)

	has, err = jujuSvr.HasProfile("unknown")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(has, tc.IsFalse)
}

func (s *serverSuite) TestCreateProfileWithConfig(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl)

	req := api.ProfilesPost{
		Name: "custom",
		ProfilePut: api.ProfilePut{
			Config: map[string]string{
				"boot.autostart": "false",
			},
		},
	}
	cSvr.EXPECT().CreateProfile(req).Return(nil)

	jujuSvr, err := lxd.NewServer(cSvr)
	c.Assert(err, tc.ErrorIsNil)
	err = jujuSvr.CreateProfileWithConfig("custom", map[string]string{"boot.autostart": "false"})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serverSuite) TestGetServerName(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	serverName := "nuc8"
	mutate := func(s *api.Server) {
		s.Environment.ServerClustered = false
		s.Environment.ServerName = serverName
	}

	cSvr := s.NewMockServer(ctrl, mutate)
	jujuSvr, err := lxd.NewServer(cSvr)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jujuSvr.Name(), tc.Equals, serverName)
}

func (s *serverSuite) TestGetServerNameReturnsNoneIfServerNameIsEmpty(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mutate := func(s *api.Server) {
		s.Environment.ServerClustered = false
		s.Environment.ServerName = ""
	}

	cSvr := s.NewMockServer(ctrl, mutate)
	jujuSvr, err := lxd.NewServer(cSvr)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jujuSvr.Name(), tc.Equals, "none")
}

func (s *serverSuite) TestGetServerNameReturnsEmptyIfServerNameIsEmptyAndClustered(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mutate := func(s *api.Server) {
		s.Environment.ServerClustered = true
		s.Environment.ServerName = ""
	}

	cSvr := s.NewMockServer(ctrl, mutate)
	jujuSvr, err := lxd.NewServer(cSvr)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jujuSvr.Name(), tc.Equals, "")
}

func (s *serverSuite) TestReplaceOrAddContainerProfile(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl)

	updateOp := lxdtesting.NewMockOperation(ctrl)
	updateOp.EXPECT().Wait().Return(nil)
	updateOp.EXPECT().Get().Return(api.Operation{Description: "Updating container"})

	instId := "testme"
	old := "old-profile"
	oldProfiles := []string{"default", "juju-default", old}
	new := "new-profile"
	cSvr.EXPECT().GetInstance(instId).Return(
		&api.Instance{
			Profiles: oldProfiles,
		}, "", nil)
	cSvr.EXPECT().UpdateInstance(instId, gomock.Any(), gomock.Any()).Return(updateOp, nil)

	jujuSvr, err := lxd.NewServer(cSvr)
	c.Assert(err, tc.ErrorIsNil)
	err = jujuSvr.ReplaceOrAddContainerProfile(instId, old, new)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serverSuite) TestUseProject(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl)

	cSvr.EXPECT().UseProject("my-project").Return(cSvr)

	jujuSvr, err := lxd.NewServer(cSvr)
	c.Assert(err, tc.ErrorIsNil)

	jujuSvr.UseProject("my-project")
}
