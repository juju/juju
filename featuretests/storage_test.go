// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/envcmd"
	cmdstorage "github.com/juju/juju/cmd/juju/storage"
	"github.com/juju/juju/feature"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/provider/ec2"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/storage/provider"
	"github.com/juju/juju/storage/provider/registry"
	"github.com/juju/juju/testing"
	"github.com/juju/utils/set"
)

const (
	testPool           = "block"
	testPersistentPool = "block-persistent"
)

func setupTestStorageSupport(c *gc.C, s *state.State) {
	stsetts := state.NewStateSettings(s)
	poolManager := poolmanager.New(stsetts)
	_, err := poolManager.Create(testPool, provider.LoopProviderType, map[string]interface{}{"it": "works"})
	c.Assert(err, jc.ErrorIsNil)
	_, err = poolManager.Create(testPersistentPool, ec2.EBS_ProviderType, map[string]interface{}{"persistent": true})
	c.Assert(err, jc.ErrorIsNil)

	registry.RegisterEnvironStorageProviders("someprovider", provider.LoopProviderType)
	registry.RegisterEnvironStorageProviders("dummy", ec2.EBS_ProviderType)
}

func makeStorageCons(pool string, size, count uint64) state.StorageConstraints {
	return state.StorageConstraints{Pool: pool, Size: size, Count: count}
}

func createUnitWithStorage(c *gc.C, s *jujutesting.JujuConnSuite, poolName string) string {
	ch := s.AddTestingCharm(c, "storage-block")
	storage := map[string]state.StorageConstraints{
		"data": makeStorageCons(poolName, 1024, 1),
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
	createUnitWithStorage(c, &s.JujuConnSuite, testPool)

	context := runShow(c, []string{"data/0"})
	expected := `
storage-block/0:
  data/0:
    storage: data
    kind: block
    status: pending
    persistent: false
`[1:]
	c.Assert(testing.Stdout(context), gc.Equals, expected)
}

func (s *cmdStorageSuite) TestStorageShowOneMatchingFilter(c *gc.C) {
	createUnitWithStorage(c, &s.JujuConnSuite, testPool)

	context := runShow(c, []string{"data/0", "fluff/0"})
	expected := `
storage-block/0:
  data/0:
    storage: data
    kind: block
    status: pending
    persistent: false
`[1:]
	c.Assert(testing.Stdout(context), gc.Equals, expected)
}

func (s *cmdStorageSuite) TestStorageShowNoMatch(c *gc.C) {
	createUnitWithStorage(c, &s.JujuConnSuite, testPool)
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
	createUnitWithStorage(c, &s.JujuConnSuite, testPool)

	context := runList(c)
	expected := `
[Storage]       
UNIT            ID     LOCATION STATUS  PERSISTENT 
storage-block/0 data/0          pending false      

`[1:]
	c.Assert(testing.Stdout(context), gc.Equals, expected)
}

func (s *cmdStorageSuite) TestStorageListPersistent(c *gc.C) {
	createUnitWithStorage(c, &s.JujuConnSuite, testPersistentPool)

	context := runList(c)
	expected := `
[Storage]       
UNIT            ID     LOCATION STATUS  PERSISTENT 
storage-block/0 data/0          pending true       

`[1:]
	c.Assert(testing.Stdout(context), gc.Equals, expected)
}

func (s *cmdStorageSuite) TestStoragePersistentProvisioned(c *gc.C) {
	createUnitWithStorage(c, &s.JujuConnSuite, testPool)
	vol, err := s.State.StorageInstanceVolume(names.NewStorageTag("data/0"))
	c.Assert(err, jc.ErrorIsNil)
	s.State.SetVolumeInfo(vol.VolumeTag(), state.VolumeInfo{
		Size:       1024,
		Persistent: true,
	})

	context := runShow(c, []string{"data/0"})
	expected := `
storage-block/0:
  data/0:
    storage: data
    kind: block
    status: pending
    persistent: true
`[1:]
	c.Assert(testing.Stdout(context), gc.Equals, expected)
}

func (s *cmdStorageSuite) TestStoragePersistentUnprovisioned(c *gc.C) {
	createUnitWithStorage(c, &s.JujuConnSuite, testPersistentPool)

	context := runShow(c, []string{"data/0"})
	expected := `
storage-block/0:
  data/0:
    storage: data
    kind: block
    status: pending
    persistent: true
`[1:]
	c.Assert(testing.Stdout(context), gc.Equals, expected)
}

func runPoolList(c *gc.C, args []string) *cmd.Context {
	context, err := testing.RunCommand(c, envcmd.Wrap(&cmdstorage.PoolListCommand{}), args...)
	c.Assert(err, jc.ErrorIsNil)
	return context
}

func (s *cmdStorageSuite) TestListPools(c *gc.C) {
	context := runPoolList(c, []string{""})
	expected := `
block:
  provider: loop
  attrs:
    it: works
block-persistent:
  provider: ebs
  attrs:
    persistent: true
`[1:]
	c.Assert(testing.Stdout(context), gc.Equals, expected)
}

func (s *cmdStorageSuite) TestListPoolsName(c *gc.C) {
	context := runPoolList(c, []string{"--name", "block"})
	expected := `
block:
  provider: loop
  attrs:
    it: works
`[1:]
	c.Assert(testing.Stdout(context), gc.Equals, expected)
}

func (s *cmdStorageSuite) TestListPoolsNameNoMatch(c *gc.C) {
	context := runPoolList(c, []string{"--name", "cranky"})
	c.Assert(testing.Stdout(context), gc.Equals, "")
}

func (s *cmdStorageSuite) TestListPoolsProvider(c *gc.C) {
	context := runPoolList(c, []string{"--type", "ebs"})
	expected := `
block-persistent:
  provider: ebs
  attrs:
    persistent: true
`[1:]
	c.Assert(testing.Stdout(context), gc.Equals, expected)
}

func (s *cmdStorageSuite) TestListPoolsProviderNoMatch(c *gc.C) {
	context := runPoolList(c, []string{"--type", "oops"})
	c.Assert(testing.Stdout(context), gc.Equals, "")
}

func (s *cmdStorageSuite) TestListPoolsNameAndProvider(c *gc.C) {
	context := runPoolList(c, []string{"--name", "block", "--type", "ebs"})
	expected := `
block:
  provider: loop
  attrs:
    it: works
block-persistent:
  provider: ebs
  attrs:
    persistent: true
`[1:]
	c.Assert(testing.Stdout(context), gc.Equals, expected)
}

func (s *cmdStorageSuite) TestListPoolsProviderAndNotName(c *gc.C) {
	context := runPoolList(c, []string{"--name", "fluff", "--type", "ebs"})
	expected := `
block-persistent:
  provider: ebs
  attrs:
    persistent: true
`[1:]
	c.Assert(testing.Stdout(context), gc.Equals, expected)
}

func (s *cmdStorageSuite) TestListPoolsNameAndNotProvider(c *gc.C) {
	context := runPoolList(c, []string{"--name", "block", "--type", "oops"})
	expected := `
block:
  provider: loop
  attrs:
    it: works
`[1:]
	c.Assert(testing.Stdout(context), gc.Equals, expected)
}

func (s *cmdStorageSuite) TestListPoolsNotNameAndNotProvider(c *gc.C) {
	context := runPoolList(c, []string{"--name", "fluff", "--type", "oops"})
	c.Assert(testing.Stdout(context), gc.Equals, "")
}

func runPoolCreate(c *gc.C, args []string) *cmd.Context {
	context, err := testing.RunCommand(c, envcmd.Wrap(&cmdstorage.PoolCreateCommand{}), args...)
	c.Assert(err, jc.ErrorIsNil)
	return context
}

func (s *cmdStorageSuite) TestCreatePool(c *gc.C) {
	pname := "ftPool"
	context := runPoolCreate(c, []string{pname, "loop", "smth=one"})
	c.Assert(testing.Stdout(context), gc.Equals, "")
	assertPoolByName(c, s.State, pname)
}

func (s *cmdStorageSuite) TestCreatePoolErrorNoAttrs(c *gc.C) {
	_, err := testing.RunCommand(c, envcmd.Wrap(&cmdstorage.PoolCreateCommand{}), "loop", "ftPool")
	c.Assert(errors.Cause(err), gc.ErrorMatches, ".*pool creation requires names, provider type and attrs for configuration.*")
}

func (s *cmdStorageSuite) TestCreatePoolErrorNoProvider(c *gc.C) {
	_, err := testing.RunCommand(c, envcmd.Wrap(&cmdstorage.PoolCreateCommand{}), "oops provider", "smth=one")
	c.Assert(errors.Cause(err), gc.ErrorMatches, ".*pool creation requires names, provider type and attrs for configuration.*")
}

func (s *cmdStorageSuite) TestCreatePoolErrorProviderType(c *gc.C) {
	_, err := testing.RunCommand(c, envcmd.Wrap(&cmdstorage.PoolCreateCommand{}), "loop", "ftPool", "smth=one")
	c.Assert(errors.Cause(err), gc.ErrorMatches, ".*not found.*")
}

func assertPoolByName(c *gc.C, st *state.State, pname string) {
	stsetts := state.NewStateSettings(st)
	poolManager := poolmanager.New(stsetts)

	found, err := poolManager.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(found) > 0, jc.IsTrue)

	namesSet := make(set.Strings)
	for _, one := range found {
		namesSet.Add(one.Name())
	}
	c.Assert(namesSet.Contains(pname), jc.IsTrue)
}
