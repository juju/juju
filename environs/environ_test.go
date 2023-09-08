// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apiservertesting "github.com/juju/juju/apiserver/testing"
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

func (m *mockModel) CloudName() string {
	return jujutesting.DefaultCloud.Name
}

func (m *mockModel) CloudRegion() string {
	return jujutesting.DefaultCloudRegion
}

func (m *mockModel) CloudCredentialTag() (names.CloudCredentialTag, bool) {
	return names.CloudCredentialTag{}, false
}

func (s *environSuite) TestGetEnvironment(c *gc.C) {
	cfg := testing.CustomModelConfig(c, testing.Attrs{"name": "testmodel-foo"})
	m := &mockModel{cfg: cfg}
	env, err := stateenvirons.GetNewEnvironFunc(environs.New)(m, apiservertesting.ConstCloudGetter(&jujutesting.DefaultCloud), nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Config().UUID(), jc.DeepEquals, cfg.UUID())
}
