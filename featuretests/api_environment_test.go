// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"bytes"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/juju"
	jujunames "github.com/juju/juju/juju/names"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/juju/version"
)

type apiEnvironmentSuite struct {
	testing.JujuConnSuite
	client *api.Client
}

func (s *apiEnvironmentSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	var err error
	s.client, err = juju.NewAPIClientFromName("")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.client, gc.NotNil)
}

func (s *apiEnvironmentSuite) TearDownTest(c *gc.C) {
	s.client.ClientFacade.Close()
	s.JujuConnSuite.TearDownTest(c)
}

func (s *apiEnvironmentSuite) TestEnvironmentShare(c *gc.C) {
	user := names.NewUserTag("foo@ubuntuone")

	err := s.client.ShareEnvironment(user)
	c.Assert(err, jc.ErrorIsNil)

	envUser, err := s.State.EnvironmentUser(user)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(envUser.UserName(), gc.Equals, user.Username())
	c.Assert(envUser.CreatedBy(), gc.Equals, s.AdminUserTag(c).Username())
	lastConn, err := envUser.LastConnection()
	c.Assert(err, jc.Satisfies, state.IsNeverConnectedError)
	c.Assert(lastConn.IsZero(), jc.IsTrue)
}

func (s *apiEnvironmentSuite) TestEnvironmentUnshare(c *gc.C) {
	// Firt share an environment with a user.
	user := names.NewUserTag("foo@ubuntuone")
	err := s.client.ShareEnvironment(user)
	c.Assert(err, jc.ErrorIsNil)

	envUser, err := s.State.EnvironmentUser(user)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(envUser, gc.NotNil)

	// Then test unsharing the environment.
	err = s.client.UnshareEnvironment(user)
	c.Assert(err, jc.ErrorIsNil)

	envUser, err = s.State.EnvironmentUser(user)
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
	c.Assert(envUser, gc.IsNil)
}

func (s *apiEnvironmentSuite) TestEnvironmentUserInfo(c *gc.C) {
	envUser := s.Factory.MakeEnvUser(c, &factory.EnvUserParams{User: "bobjohns@ubuntuone", DisplayName: "Bob Johns"})
	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	owner, err := s.State.EnvironmentUser(env.Owner())
	c.Assert(err, jc.ErrorIsNil)

	obtained, err := s.client.EnvironmentUserInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, []params.EnvUserInfo{
		{
			UserName:       owner.UserName(),
			DisplayName:    owner.DisplayName(),
			CreatedBy:      owner.UserName(),
			DateCreated:    owner.DateCreated(),
			LastConnection: lastConnPointer(c, owner),
		}, {
			UserName:       "bobjohns@ubuntuone",
			DisplayName:    "Bob Johns",
			CreatedBy:      owner.UserName(),
			DateCreated:    envUser.DateCreated(),
			LastConnection: lastConnPointer(c, envUser),
		},
	})
}

func lastConnPointer(c *gc.C, envUser *state.EnvironmentUser) *time.Time {
	lastConn, err := envUser.LastConnection()
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
	otherState := s.Factory.MakeEnvironment(c, nil)
	defer otherState.Close()
	info := s.APIInfo(c)
	info.EnvironTag = otherState.EnvironTag()
	otherAPIState, err := api.Open(info, api.DefaultDialOpts())
	c.Assert(err, jc.ErrorIsNil)
	defer otherAPIState.Close()
	otherClient := otherAPIState.Client()
	defer otherClient.ClientFacade.Close()

	newVersion := version.MustParseBinary("5.4.3-quantal-amd64")

	// build fake tools
	tgz, checksum := coretesting.TarGz(
		coretesting.NewTarFile(jujunames.Jujud, 0777, "jujud contents "+newVersion.String()))

	tool, err := otherClient.UploadTools(bytes.NewReader(tgz), newVersion)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tool.SHA256, gc.Equals, checksum)

	toolStrg, err := otherState.ToolsStorage()
	defer toolStrg.Close()
	c.Assert(err, jc.ErrorIsNil)
	meta, closer, err := toolStrg.Tools(newVersion)
	defer closer.Close()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(meta.SHA256, gc.Equals, checksum)
	c.Assert(meta.Version, gc.Equals, newVersion)
}
