// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"strings"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/cmd/juju/controller"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/testing"
)

const (
	serverUUID = "0dbfe161-de6c-47ad-9283-5e3ea64e1dd3"
	model1UUID = "ebf03329-cdad-44a5-9f10-fe318efda3ce"
	model2UUID = "b366cdd5-82da-49a1-ac18-001f26bb59a3"
	model3UUID = "fd0f57a3-eb94-4095-9ab0-d1f6042f942a"
	model4UUID = "1e45141b-85cb-4a0a-96ef-0aa6bbeac45a"
)

type UseModelSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	api      *fakeModelMgrAPIClient
	creds    configstore.APICredentials
	endpoint configstore.APIEndpoint
}

var _ = gc.Suite(&UseModelSuite{})

func (s *UseModelSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)

	err := modelcmd.WriteCurrentController("fake")
	c.Assert(err, jc.ErrorIsNil)

	models := []base.UserModel{{
		Name:  "unique",
		Owner: "tester@local",
		UUID:  "some-uuid",
	}, {
		Name:  "test",
		Owner: "tester@local",
		UUID:  model1UUID,
	}, {
		Name:  "test",
		Owner: "bob@local",
		UUID:  model2UUID,
	}, {
		Name:  "other",
		Owner: "bob@local",
		UUID:  model3UUID,
	}, {
		Name:  "other",
		Owner: "bob@remote",
		UUID:  model4UUID,
	}}
	s.api = &fakeModelMgrAPIClient{models: models}
	s.creds = configstore.APICredentials{User: "tester", Password: "password"}
	s.endpoint = configstore.APIEndpoint{
		Addresses:  []string{"127.0.0.1:12345"},
		Hostnames:  []string{"localhost:12345"},
		CACert:     testing.CACert,
		ServerUUID: serverUUID,
	}
}

func (s *UseModelSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	wrappedCommand, _ := controller.NewUseModelCommandForTest(s.api, &s.creds, &s.endpoint)
	return testing.RunCommand(c, wrappedCommand, args...)
}

func (s *UseModelSuite) TestInit(c *gc.C) {
	for i, test := range []struct {
		args        []string
		errorString string
		localName   string
		owner       string
		modelName   string
		modelUUID   string
	}{{
		errorString: "no model supplied",
	}, {
		args:        []string{""},
		errorString: "no model supplied",
	}, {
		args:      []string{"model-name"},
		modelName: "model-name",
	}, {
		args:      []string{"model-name", "--name", "foo"},
		localName: "foo",
		modelName: "model-name",
	}, {
		args:      []string{"user/foobar"},
		modelName: "foobar",
		owner:     "user",
	}, {
		args:      []string{"user@local/foobar"},
		modelName: "foobar",
		owner:     "user@local",
	}, {
		args:      []string{"user@remote/foobar"},
		modelName: "foobar",
		owner:     "user@remote",
	}, {
		args:        []string{"+user+name/foobar"},
		errorString: `"\+user\+name" is not a valid user`,
	}, {
		args:      []string{model1UUID},
		modelUUID: model1UUID,
	}, {
		args:      []string{"user/" + model1UUID},
		owner:     "user",
		modelUUID: model1UUID,
	}} {
		c.Logf("test %d", i)
		wrappedCommand, command := controller.NewUseModelCommandForTest(nil, nil, nil)
		err := testing.InitCommand(wrappedCommand, test.args)
		if test.errorString == "" {
			c.Check(command.LocalName, gc.Equals, test.localName)
			c.Check(command.ModelName, gc.Equals, test.modelName)
			c.Check(command.ModelUUID, gc.Equals, test.modelUUID)
			c.Check(command.Owner, gc.Equals, test.owner)
		} else {
			c.Check(err, gc.ErrorMatches, test.errorString)
		}
	}
}

func (s *UseModelSuite) TestEnvironmentsError(c *gc.C) {
	s.api.err = common.ErrPerm
	_, err := s.run(c, "ignored-but-needed")
	c.Assert(err, gc.ErrorMatches, "cannot list models: permission denied")
}

func (s *UseModelSuite) TestNameNotFound(c *gc.C) {
	_, err := s.run(c, "missing")
	c.Assert(err, gc.ErrorMatches, "matching model not found")
}

func (s *UseModelSuite) TestUUID(c *gc.C) {
	_, err := s.run(c, model3UUID)
	c.Assert(err, gc.IsNil)

	s.assertCurrentModel(c, "bob-other", model3UUID)
}

func (s *UseModelSuite) TestUUIDCorrectOwner(c *gc.C) {
	_, err := s.run(c, "bob/"+model3UUID)
	c.Assert(err, gc.IsNil)

	s.assertCurrentModel(c, "bob-other", model3UUID)
}

func (s *UseModelSuite) TestUUIDWrongOwner(c *gc.C) {
	ctx, err := s.run(c, "charles/"+model3UUID)
	c.Assert(err, gc.IsNil)
	expected := "Specified model owned by bob@local, not charles@local"
	c.Assert(testing.Stderr(ctx), jc.Contains, expected)

	s.assertCurrentModel(c, "bob-other", model3UUID)
}

func (s *UseModelSuite) TestUniqueName(c *gc.C) {
	_, err := s.run(c, "unique")
	c.Assert(err, gc.IsNil)

	s.assertCurrentModel(c, "unique", "some-uuid")
}

func (s *UseModelSuite) TestMultipleNameMatches(c *gc.C) {
	ctx, err := s.run(c, "test")
	c.Assert(err, gc.ErrorMatches, "multiple models matched")

	message := strings.TrimSpace(testing.Stderr(ctx))
	lines := strings.Split(message, "\n")
	c.Assert(lines, gc.HasLen, 4)
	c.Assert(lines[0], gc.Equals, `Multiple models matched name "test":`)
	c.Assert(lines[1], gc.Equals, "  "+model1UUID+", owned by tester@local")
	c.Assert(lines[2], gc.Equals, "  "+model2UUID+", owned by bob@local")
	c.Assert(lines[3], gc.Equals, `Please specify either the model UUID or the owner to disambiguate.`)
}

func (s *UseModelSuite) TestUserOwnerOfEnvironment(c *gc.C) {
	_, err := s.run(c, "tester/test")
	c.Assert(err, gc.IsNil)

	s.assertCurrentModel(c, "test", model1UUID)
}

func (s *UseModelSuite) TestOtherUsersEnvironment(c *gc.C) {
	_, err := s.run(c, "bob/test")
	c.Assert(err, gc.IsNil)

	s.assertCurrentModel(c, "bob-test", model2UUID)
}

func (s *UseModelSuite) TestRemoteUsersEnvironmentName(c *gc.C) {
	_, err := s.run(c, "bob@remote/other")
	c.Assert(err, gc.IsNil)

	s.assertCurrentModel(c, "bob-other", model4UUID)
}

func (s *UseModelSuite) TestDisambiguateWrongOwner(c *gc.C) {
	_, err := s.run(c, "wrong/test")
	c.Assert(err, gc.ErrorMatches, "matching model not found")
}

func (s *UseModelSuite) TestUseEnvAlreadyExisting(c *gc.C) {
	s.makeLocalEnvironment(c, "unique", "", "")
	ctx, err := s.run(c, "unique")
	c.Assert(err, gc.ErrorMatches, "existing model")
	expected := `You have an existing model called "unique", use --name to specify a different local name.`
	c.Assert(testing.Stderr(ctx), jc.Contains, expected)
}

func (s *UseModelSuite) TestUseEnvAlreadyExistingSameEnv(c *gc.C) {
	s.makeLocalEnvironment(c, "unique", "some-uuid", "tester")
	ctx, err := s.run(c, "unique")
	c.Assert(err, gc.IsNil)

	message := strings.TrimSpace(testing.Stderr(ctx))
	lines := strings.Split(message, "\n")
	c.Assert(lines, gc.HasLen, 2)

	expected := `You already have model details for "unique" cached locally.`
	c.Assert(lines[0], gc.Equals, expected)
	c.Assert(lines[1], gc.Equals, `fake (controller) -> unique`)

	current, err := modelcmd.ReadCurrentModel()
	c.Assert(err, gc.IsNil)
	c.Assert(current, gc.Equals, "unique")
}

func (s *UseModelSuite) assertCurrentModel(c *gc.C, name, uuid string) {
	current, err := modelcmd.ReadCurrentModel()
	c.Assert(err, gc.IsNil)
	c.Assert(current, gc.Equals, name)

	store, err := configstore.Default()
	c.Assert(err, gc.IsNil)

	info, err := store.ReadInfo(name)
	c.Assert(err, gc.IsNil)
	c.Assert(info.APIEndpoint(), jc.DeepEquals, configstore.APIEndpoint{
		Addresses:  []string{"127.0.0.1:12345"},
		Hostnames:  []string{"localhost:12345"},
		CACert:     testing.CACert,
		ModelUUID:  uuid,
		ServerUUID: serverUUID,
	})
	c.Assert(info.APICredentials(), jc.DeepEquals, s.creds)
}

func (s *UseModelSuite) makeLocalEnvironment(c *gc.C, name, uuid, owner string) {
	store, err := configstore.Default()
	c.Assert(err, gc.IsNil)

	if uuid == "" {
		uuid = utils.MustNewUUID().String()
	}
	if owner == "" {
		owner = "random@person"
	}
	info := store.CreateInfo(name)
	info.SetAPIEndpoint(configstore.APIEndpoint{
		ModelUUID: uuid,
	})
	info.SetAPICredentials(configstore.APICredentials{
		User: owner,
	})
	err = info.Write()
	c.Assert(err, gc.IsNil)
}
