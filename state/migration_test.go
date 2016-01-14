// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state/migration"
	"github.com/juju/juju/version"
)

type MigrationSuite struct {
	ConnSuite
}

var _ = gc.Suite(&MigrationSuite{})

func (s *MigrationSuite) setLatestTools(c *gc.C, latestTools version.Number) {
	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	err = env.UpdateLatestToolsVersion(latestTools)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MigrationSuite) TestExportEnvironmentInfo(c *gc.C) {
	latestTools := version.MustParse("2.0.1")
	s.setLatestTools(c, latestTools)
	out, err := s.State.Export()
	c.Assert(err, jc.ErrorIsNil)

	model := out.Model()

	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Tag(), gc.Equals, env.EnvironTag())
	c.Assert(model.Owner(), gc.Equals, env.Owner())
	config, err := env.Config()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Config(), jc.DeepEquals, config.AllAttrs())
	c.Assert(model.LatestToolsVersion(), gc.Equals, latestTools)
}

func (s *MigrationSuite) TestImportExisting(c *gc.C) {
	out, err := s.State.Export()
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.Import(out)
	c.Assert(err, jc.Satisfies, errors.IsAlreadyExists)
}

func (s *MigrationSuite) TestImportNewEnv(c *gc.C) {
	latestTools := version.MustParse("2.0.1")
	s.setLatestTools(c, latestTools)
	out, err := s.State.Export()
	c.Assert(err, jc.ErrorIsNil)

	uuid := utils.MustNewUUID().String()
	in := newDescription(out, uuid, "new")

	err = s.State.Import(in)
	c.Assert(err, jc.ErrorIsNil)

	original, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)

	newEnv, err := s.State.GetEnvironment(names.NewEnvironTag(uuid))
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(newEnv.Owner(), gc.Equals, original.Owner())
	c.Assert(newEnv.LatestToolsVersion(), gc.Equals, latestTools)
	originalConfig, err := original.Config()
	c.Assert(err, jc.ErrorIsNil)
	originalAttrs := originalConfig.AllAttrs()

	newConfig, err := newEnv.Config()
	c.Assert(err, jc.ErrorIsNil)
	newAttrs := newConfig.AllAttrs()

	c.Assert(newAttrs["uuid"], gc.Equals, uuid)
	c.Assert(newAttrs["name"], gc.Equals, "new")

	// Now drop the uuid and name and the rest of the attributes should match.
	delete(newAttrs, "uuid")
	delete(newAttrs, "name")
	delete(originalAttrs, "uuid")
	delete(originalAttrs, "name")
	c.Assert(newAttrs, jc.DeepEquals, originalAttrs)
}

// newDescription replaces the uuid and name of the config attributes so we
// can use all the other data to validate imports. An owner and name of the
// environment / model are unique together in a controller.
func newDescription(d migration.Description, uuid, name string) migration.Description {
	return &mockDescription{d, uuid, name}
}

type mockDescription struct {
	d    migration.Description
	uuid string
	name string
}

func (m *mockDescription) Model() migration.Model {
	return &mockModel{m.d.Model(), m.uuid, m.name}
}

type mockModel struct {
	migration.Model
	uuid string
	name string
}

func (m *mockModel) Tag() names.EnvironTag {
	return names.NewEnvironTag(m.uuid)
}

func (m *mockModel) Config() map[string]interface{} {
	c := m.Model.Config()
	c["uuid"] = m.uuid
	c["name"] = m.name
	return c
}
