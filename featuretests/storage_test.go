// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/envcmd"
	cmdstorage "github.com/juju/juju/cmd/juju/storage"
	"github.com/juju/juju/feature"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/storage/provider"
	"github.com/juju/juju/storage/provider/registry"
	"github.com/juju/juju/testing"
)

const testPool = "block"

func setupTestStorageSupport(c *gc.C, s *state.State) {
	stsetts := state.NewStateSettings(s)
	poolManager := poolmanager.New(stsetts)
	_, err := poolManager.Create(testPool, provider.LoopProviderType, map[string]interface{}{"it": "works"})
	c.Assert(err, jc.ErrorIsNil)

	registry.RegisterEnvironStorageProviders("someprovider", provider.LoopProviderType)
}

func makeStorageCons(pool string, size, count uint64) state.StorageConstraints {
	return state.StorageConstraints{Pool: pool, Size: size, Count: count}
}

func createUnitWithStorage(c *gc.C, s *jujutesting.JujuConnSuite) string {
	ch := s.AddTestingCharm(c, "storage-block")
	storage := map[string]state.StorageConstraints{
		"data": makeStorageCons(testPool, 1024, 1),
	}
	service := s.AddTestingServiceWithStorage(c, "storage-block", ch, storage)
	unit, err := service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.AssignUnit(unit, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)

	machineId, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)

	return machineId
}

type cmdStorageSuite struct {
	jujutesting.RepoSuite
}

var _ = gc.Suite(&cmdStorageSuite{})

func (s *cmdStorageSuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)
	s.SetFeatureFlags(feature.Storage)

	setupTestStorageSupport(c, s.State)
}

func runShow(c *gc.C, args []string) *cmd.Context {
	context, err := testing.RunCommand(c, envcmd.Wrap(&cmdstorage.ShowCommand{}), args...)
	c.Assert(err, jc.ErrorIsNil)
	return context
}

func (s *cmdStorageSuite) TestStorageShowEmpty(c *gc.C) {
	_, err := testing.RunCommand(c, envcmd.Wrap(&cmdstorage.ShowCommand{}))
	c.Assert(errors.Cause(err), gc.ErrorMatches, ".*must specify storage id.*")
}

func (s *cmdStorageSuite) TestStorageShowInvalidId(c *gc.C) {
	_, err := testing.RunCommand(c, envcmd.Wrap(&cmdstorage.ShowCommand{}), "fluff")
	c.Assert(errors.Cause(err), gc.ErrorMatches, ".*invalid storage id.*")
}

func (s *cmdStorageSuite) TestStorageShow(c *gc.C) {
	createUnitWithStorage(c, &s.JujuConnSuite)

	context := runShow(c, []string{"data/0"})
	expected := `
storage-block/0:
  data/0:
    storage: data
    kind: block
    unit_id: storage-block/0
    attached_status: attached
    provisioned_status: pending
`[1:]
	c.Assert(testing.Stdout(context), gc.Equals, expected)
}

func (s *cmdStorageSuite) TestStorageShowOneMatchingFilter(c *gc.C) {
	createUnitWithStorage(c, &s.JujuConnSuite)

	context := runShow(c, []string{"data/0", "fluff/0"})
	expected := `
storage-block/0:
  data/0:
    storage: data
    kind: block
    unit_id: storage-block/0
    attached_status: attached
    provisioned_status: pending
`[1:]
	c.Assert(testing.Stdout(context), gc.Equals, expected)
}

func (s *cmdStorageSuite) TestStorageShowNoMatch(c *gc.C) {
	createUnitWithStorage(c, &s.JujuConnSuite)
	context := runShow(c, []string{"fluff/0"})
	c.Assert(testing.Stdout(context), gc.Equals, "{}\n")
}

func runList(c *gc.C) *cmd.Context {
	context, err := testing.RunCommand(c, envcmd.Wrap(&cmdstorage.ListCommand{}))
	c.Assert(err, jc.ErrorIsNil)
	return context
}

func (s *cmdStorageSuite) TestStorageListEmpty(c *gc.C) {
	context := runList(c)
	c.Assert(testing.Stdout(context), gc.Equals, "")
}

func (s *cmdStorageSuite) TestStorageList(c *gc.C) {
	createUnitWithStorage(c, &s.JujuConnSuite)

	context := runList(c)
	expected := `
[Storage]       
UNIT            ID     LOCATION 
storage-block/0 data/0          

`[1:]
	c.Assert(testing.Stdout(context), gc.Equals, expected)
}
