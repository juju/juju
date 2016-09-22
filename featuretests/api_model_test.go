// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"bytes"
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/modelmanager"
	"github.com/juju/juju/apiserver/params"
	jujunames "github.com/juju/juju/juju/names"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type apiEnvironmentSuite struct {
	testing.JujuConnSuite
	client *api.Client
}

func (s *apiEnvironmentSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.client = s.APIState.Client()
	c.Assert(s.client, gc.NotNil)
	s.AddCleanup(func(*gc.C) {
		s.client.ClientFacade.Close()
	})
}

func (s *apiEnvironmentSuite) TestGrantModel(c *gc.C) {
	username := "foo@ubuntuone"
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	mm := modelmanager.NewClient(s.OpenControllerAPI(c))
	defer mm.Close()
	err = mm.GrantModel(username, "read", model.UUID())
	c.Assert(err, jc.ErrorIsNil)

	user := names.NewUserTag(username)
	modelUser, err := s.State.UserAccess(user, model.ModelTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelUser.UserName, gc.Equals, user.Canonical())
	lastConn, err := s.State.LastModelConnection(modelUser.UserTag)
	c.Assert(err, jc.Satisfies, state.IsNeverConnectedError)
	c.Assert(lastConn.IsZero(), jc.IsTrue)
}

func (s *apiEnvironmentSuite) TestRevokeModel(c *gc.C) {
	// First share an environment with a user.
	user := s.Factory.MakeModelUser(c, &factory.ModelUserParams{User: "foo@ubuntuone"})
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	mm := modelmanager.NewClient(s.OpenControllerAPI(c))
	defer mm.Close()

	modelUser, err := s.State.UserAccess(user.UserTag, s.State.ModelTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelUser, gc.Not(gc.DeepEquals), permission.UserAccess{})

	// Then test unsharing the environment.
	err = mm.RevokeModel(user.UserName, "read", model.UUID())
	c.Assert(err, jc.ErrorIsNil)

	modelUser, err = s.State.UserAccess(user.UserTag, s.State.ModelTag())
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
	c.Assert(modelUser, gc.DeepEquals, permission.UserAccess{})
}

func (s *apiEnvironmentSuite) TestEnvironmentUserInfo(c *gc.C) {
	modelUser := s.Factory.MakeModelUser(c, &factory.ModelUserParams{User: "bobjohns@ubuntuone", DisplayName: "Bob Johns"})
	mod, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	owner, err := s.State.UserAccess(mod.Owner(), mod.ModelTag())
	c.Assert(err, jc.ErrorIsNil)

	obtained, err := s.client.ModelUserInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, []params.ModelUserInfo{
		{
			UserName:       owner.UserName,
			DisplayName:    owner.DisplayName,
			Access:         "admin",
			LastConnection: lastConnPointer(c, s.State, owner),
		}, {
			UserName:       "bobjohns@ubuntuone",
			DisplayName:    "Bob Johns",
			Access:         "admin",
			LastConnection: lastConnPointer(c, s.State, modelUser),
		},
	})
}

func lastConnPointer(c *gc.C, st *state.State, modelUser permission.UserAccess) *time.Time {
	lastConn, err := st.LastModelConnection(modelUser.UserTag)
	if err != nil {
		if state.IsNeverConnectedError(err) {
			return nil
		}
		c.Fatal(err)
	}
	return &lastConn
}

func (s *apiEnvironmentSuite) TestUploadToolsOtherEnvironment(c *gc.C) {
	// setup other environment
	otherState := s.Factory.MakeModel(c, nil)
	defer otherState.Close()
	info := s.APIInfo(c)
	info.ModelTag = otherState.ModelTag()
	otherAPIState, err := api.Open(info, api.DefaultDialOpts())
	c.Assert(err, jc.ErrorIsNil)
	defer otherAPIState.Close()
	otherClient := otherAPIState.Client()
	defer otherClient.ClientFacade.Close()

	newVersion := version.MustParseBinary("5.4.3-quantal-amd64")
	vers := newVersion.String()

	// build fake tools
	tgz, checksum := coretesting.TarGz(
		coretesting.NewTarFile(jujunames.Jujud, 0777, "jujud contents "+vers))

	toolsList, err := otherClient.UploadTools(bytes.NewReader(tgz), newVersion)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(toolsList, gc.HasLen, 1)
	c.Assert(toolsList[0].SHA256, gc.Equals, checksum)

	toolStrg, err := otherState.ToolsStorage()
	defer toolStrg.Close()
	c.Assert(err, jc.ErrorIsNil)
	meta, closer, err := toolStrg.Open(vers)
	defer closer.Close()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(meta.SHA256, gc.Equals, checksum)
	c.Assert(meta.Version, gc.Equals, vers)
}
