// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"bytes"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v2/api"
	apiclient "github.com/juju/juju/v2/api/client/client"
	"github.com/juju/juju/v2/api/client/modelmanager"
	"github.com/juju/juju/v2/api/client/usermanager"
	"github.com/juju/juju/v2/core/permission"
	jujunames "github.com/juju/juju/v2/juju/names"
	"github.com/juju/juju/v2/juju/testing"
	"github.com/juju/juju/v2/rpc/params"
	"github.com/juju/juju/v2/state"
	coretesting "github.com/juju/juju/v2/testing"
	"github.com/juju/juju/v2/testing/factory"
)

type apiModelSuite struct {
	testing.JujuConnSuite
}

func (s *apiModelSuite) TestGrantModel(c *gc.C) {
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
	c.Assert(modelUser.UserName, gc.Equals, user.Id())
	lastConn, err := s.Model.LastModelConnection(modelUser.UserTag)
	c.Assert(err, jc.Satisfies, state.IsNeverConnectedError)
	c.Assert(lastConn.IsZero(), jc.IsTrue)
}

func (s *apiModelSuite) TestRevokeModel(c *gc.C) {
	// First share an model with a user.
	user := s.Factory.MakeModelUser(c, &factory.ModelUserParams{User: "foo@ubuntuone"})
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	mm := modelmanager.NewClient(s.OpenControllerAPI(c))
	defer mm.Close()

	modelUser, err := s.State.UserAccess(user.UserTag, s.Model.ModelTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelUser, gc.Not(gc.DeepEquals), permission.UserAccess{})

	// Then test unsharing the model.
	err = mm.RevokeModel(user.UserName, "read", model.UUID())
	c.Assert(err, jc.ErrorIsNil)

	modelUser, err = s.State.UserAccess(user.UserTag, s.Model.ModelTag())
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
	c.Assert(modelUser, gc.DeepEquals, permission.UserAccess{})
}

func (s *apiModelSuite) TestModelUserInfo(c *gc.C) {
	modelUser := s.Factory.MakeModelUser(c, &factory.ModelUserParams{User: "bobjohns@ubuntuone", DisplayName: "Bob Johns"})
	mod, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	owner, err := s.State.UserAccess(mod.Owner(), mod.ModelTag())
	c.Assert(err, jc.ErrorIsNil)

	um := usermanager.NewClient(s.OpenControllerAPI(c))
	defer um.Close()
	obtained, err := um.ModelUserInfo(mod.ModelTag().Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, []params.ModelUserInfo{
		{
			ModelTag:       mod.ModelTag().String(),
			UserName:       owner.UserName,
			DisplayName:    owner.DisplayName,
			Access:         "admin",
			LastConnection: lastConnPointer(c, mod, owner),
		}, {
			ModelTag:       mod.ModelTag().String(),
			UserName:       "bobjohns@ubuntuone",
			DisplayName:    "Bob Johns",
			Access:         "admin",
			LastConnection: lastConnPointer(c, mod, modelUser),
		},
	})
}

func lastConnPointer(c *gc.C, mod *state.Model, modelUser permission.UserAccess) *time.Time {
	lastConn, err := mod.LastModelConnection(modelUser.UserTag)
	if err != nil {
		if state.IsNeverConnectedError(err) {
			return nil
		}
		c.Fatal(err)
	}
	return &lastConn
}

func (s *apiModelSuite) TestUploadToolsOtherModel(c *gc.C) {
	// setup other model
	otherState := s.Factory.MakeModel(c, nil)
	defer otherState.Close()

	otherModel, err := otherState.Model()
	c.Assert(err, jc.ErrorIsNil)

	info := s.APIInfo(c)
	info.ModelTag = otherModel.ModelTag()
	otherAPIState, err := api.Open(info, api.DefaultDialOpts())
	c.Assert(err, jc.ErrorIsNil)
	defer otherAPIState.Close()
	otherClient := apiclient.NewClient(otherAPIState)
	defer otherClient.ClientFacade.Close()

	newVersion := version.MustParseBinary("5.4.3-ubuntu-amd64")
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
