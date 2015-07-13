// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environmentmanager_test

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/environmentmanager"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/juju"
	jujutesting "github.com/juju/juju/juju/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type environmentmanagerSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&environmentmanagerSuite{})

func (s *environmentmanagerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
}

func (s *environmentmanagerSuite) OpenAPI(c *gc.C) *environmentmanager.Client {
	conn, err := juju.NewAPIState(s.AdminUserTag(c), s.Environ, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(*gc.C) { conn.Close() })
	return environmentmanager.NewClient(conn)
}

func (s *environmentmanagerSuite) TestConfigSkeleton(c *gc.C) {
	s.SetFeatureFlags(feature.JES)
	envManager := s.OpenAPI(c)
	result, err := envManager.ConfigSkeleton("", "")
	c.Assert(err, jc.ErrorIsNil)

	// The apiPort changes every test run as the dummy provider
	// looks for a random open port.
	apiPort := s.Environ.Config().APIPort()

	// Numbers coming over the api are floats, not ints.
	c.Assert(result, jc.DeepEquals, params.EnvironConfig{
		"type":        "dummy",
		"ca-cert":     coretesting.CACert,
		"state-port":  float64(1234),
		"api-port":    float64(apiPort),
		"syslog-port": float64(2345),
	})

}

func (s *environmentmanagerSuite) TestCreateEnvironmentBadUser(c *gc.C) {
	envManager := s.OpenAPI(c)
	_, err := envManager.CreateEnvironment("not a user", nil, nil)
	c.Assert(err, gc.ErrorMatches, `invalid owner name "not a user"`)
}

func (s *environmentmanagerSuite) TestCreateEnvironmentFeatureNotEnabled(c *gc.C) {
	envManager := s.OpenAPI(c)
	_, err := envManager.CreateEnvironment("owner", nil, nil)
	c.Assert(err, gc.ErrorMatches, `unknown object type "EnvironmentManager"`)
}

func (s *environmentmanagerSuite) TestCreateEnvironmentMissingConfig(c *gc.C) {
	s.SetFeatureFlags(feature.JES)
	envManager := s.OpenAPI(c)
	_, err := envManager.CreateEnvironment("owner", nil, nil)
	c.Assert(err, gc.ErrorMatches, `creating config from values failed: name: expected string, got nothing`)
}

func (s *environmentmanagerSuite) TestCreateEnvironment(c *gc.C) {
	s.SetFeatureFlags(feature.JES)
	envManager := s.OpenAPI(c)
	user := s.Factory.MakeUser(c, nil)
	owner := user.UserTag().Username()
	newEnv, err := envManager.CreateEnvironment(owner, nil, map[string]interface{}{
		"name":            "new-env",
		"authorized-keys": "ssh-key",
		// dummy needs state-server
		"state-server": false,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newEnv.Name, gc.Equals, "new-env")
	c.Assert(newEnv.OwnerTag, gc.Equals, user.Tag().String())
	c.Assert(utils.IsValidUUIDString(newEnv.UUID), jc.IsTrue)
}

func (s *environmentmanagerSuite) TestListEnvironmentsBadUser(c *gc.C) {
	envManager := s.OpenAPI(c)
	_, err := envManager.ListEnvironments("not a user")
	c.Assert(err, gc.ErrorMatches, `invalid user name "not a user"`)
}

func (s *environmentmanagerSuite) TestListEnvironments(c *gc.C) {
	s.SetFeatureFlags(feature.JES)
	owner := names.NewUserTag("user@remote")
	s.Factory.MakeEnvironment(c, &factory.EnvParams{
		Name: "first", Owner: owner}).Close()
	s.Factory.MakeEnvironment(c, &factory.EnvParams{
		Name: "second", Owner: owner}).Close()

	envManager := s.OpenAPI(c)
	envs, err := envManager.ListEnvironments("user@remote")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(envs, gc.HasLen, 2)

	envNames := []string{envs[0].Name, envs[1].Name}
	c.Assert(envNames, jc.DeepEquals, []string{"first", "second"})
	ownerNames := []string{envs[0].Owner, envs[1].Owner}
	c.Assert(ownerNames, jc.DeepEquals, []string{"user@remote", "user@remote"})
}
