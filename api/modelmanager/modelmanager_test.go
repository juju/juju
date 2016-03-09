// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager_test

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/modelmanager"
	"github.com/juju/juju/apiserver/params"
	jujutesting "github.com/juju/juju/juju/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type modelmanagerSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&modelmanagerSuite{})

func (s *modelmanagerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
}

func (s *modelmanagerSuite) OpenAPI(c *gc.C) *modelmanager.Client {
	return modelmanager.NewClient(s.APIState)
}

func (s *modelmanagerSuite) TestConfigSkeleton(c *gc.C) {
	modelManager := s.OpenAPI(c)
	result, err := modelManager.ConfigSkeleton("", "")
	c.Assert(err, jc.ErrorIsNil)

	// The apiPort changes every test run as the dummy provider
	// looks for a random open port.
	apiPort := s.Environ.Config().APIPort()

	// Numbers coming over the api are floats, not ints.
	c.Assert(result, jc.DeepEquals, params.ModelConfig{
		"type":       "dummy",
		"ca-cert":    coretesting.CACert,
		"state-port": float64(1234),
		"api-port":   float64(apiPort),
	})

}

func (s *modelmanagerSuite) TestCreateModelBadUser(c *gc.C) {
	modelManager := s.OpenAPI(c)
	_, err := modelManager.CreateModel("not a user", nil, nil)
	c.Assert(err, gc.ErrorMatches, `invalid owner name "not a user"`)
}

func (s *modelmanagerSuite) TestCreateModelMissingConfig(c *gc.C) {
	modelManager := s.OpenAPI(c)
	_, err := modelManager.CreateModel("owner", nil, nil)
	c.Assert(err, gc.ErrorMatches, `creating config from values failed: name: expected string, got nothing`)
}

func (s *modelmanagerSuite) TestCreateModel(c *gc.C) {
	modelManager := s.OpenAPI(c)
	user := s.Factory.MakeUser(c, nil)
	owner := user.UserTag().Canonical()
	newEnv, err := modelManager.CreateModel(owner, nil, map[string]interface{}{
		"name":            "new-model",
		"authorized-keys": "ssh-key",
		// dummy needs controller
		"controller": false,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newEnv.Name, gc.Equals, "new-model")
	c.Assert(newEnv.OwnerTag, gc.Equals, user.Tag().String())
	c.Assert(utils.IsValidUUIDString(newEnv.UUID), jc.IsTrue)
}

func (s *modelmanagerSuite) TestListModelsBadUser(c *gc.C) {
	modelManager := s.OpenAPI(c)
	_, err := modelManager.ListModels("not a user")
	c.Assert(err, gc.ErrorMatches, `invalid user name "not a user"`)
}

func (s *modelmanagerSuite) TestListModels(c *gc.C) {
	owner := names.NewUserTag("user@remote")
	s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "first", Owner: owner}).Close()
	s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "second", Owner: owner}).Close()

	modelManager := s.OpenAPI(c)
	models, err := modelManager.ListModels("user@remote")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(models, gc.HasLen, 2)

	envNames := []string{models[0].Name, models[1].Name}
	c.Assert(envNames, jc.DeepEquals, []string{"first", "second"})
	ownerNames := []string{models[0].Owner, models[1].Owner}
	c.Assert(ownerNames, jc.DeepEquals, []string{"user@remote", "user@remote"})
}
