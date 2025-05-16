// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	"context"
	stdtesting "testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state/stateenvirons"
)

type environSuite struct {
}

func TestEnvironSuite(t *stdtesting.T) { tc.Run(t, &environSuite{}) }

type mockModel struct {
	stateenvirons.Model
	cfg *config.Config
}

func (m *mockModel) Config() (*config.Config, error) {
	return m.cfg, nil
}

func (m *mockModel) ModelConfig(ctx context.Context) (*config.Config, error) {
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

func (s *environSuite) TestGetEnvironment(c *tc.C) {
	cfg := testing.CustomModelConfig(c, testing.Attrs{"name": "testmodel-foo"})
	m := &mockModel{cfg: cfg}
	env, err := stateenvirons.GetNewEnvironFunc(environs.New)(m, apiservertesting.ConstCloudGetter(&jujutesting.DefaultCloud), nil, m)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(env.Config().UUID(), tc.DeepEquals, cfg.UUID())
}
