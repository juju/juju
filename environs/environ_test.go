// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state/stateenvirons"
	"github.com/juju/juju/testing"
)

type environSuite struct {
}

var _ = gc.Suite(&environSuite{})

type mockModel struct {
	stateenvirons.Model
	cfg *config.Config
}

func (m *mockModel) Config() (*config.Config, error) {
	return m.cfg, nil
}

func (m *mockModel) Cloud() (cloud.Cloud, error) {
	return jujutesting.DefaultCloud, nil
}

func (m *mockModel) CloudRegion() string {
	return jujutesting.DefaultCloudRegion
}

func (m *mockModel) CloudCredential() (cloud.Credential, bool, error) {
	return cloud.Credential{}, false, nil
}

func (s *environSuite) TestGetEnvironment(c *gc.C) {
	cfg := testing.CustomModelConfig(c, testing.Attrs{"name": "testmodel-foo"})
	m := &mockModel{cfg: cfg}
	env, err := stateenvirons.GetNewEnvironFunc(environs.New)(m)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Config().UUID(), jc.DeepEquals, cfg.UUID())
}
