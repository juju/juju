// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/loggo"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujucmd "github.com/juju/juju/cmd/juju/commands"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/provider/ec2"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/storage/provider"
	"github.com/juju/juju/storage/provider/registry"
	"github.com/juju/juju/testing"
)

const (
	testPool = "block"
)

func setupTestStorageSupport(c *gc.C, s *state.State) {
	stsetts := state.NewStateSettings(s)
	poolManager := poolmanager.New(stsetts)
	_, err := poolManager.Create(testPool, provider.LoopProviderType, map[string]interface{}{"it": "works"})
	c.Assert(err, jc.ErrorIsNil)

	registry.RegisterEnvironStorageProviders("dummy", ec2.EBS_ProviderType)
	registry.RegisterEnvironStorageProviders("dummyenv", ec2.EBS_ProviderType)
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

	return unit.Tag().Id()
}

type cmdStorageSuite struct {
	jujutesting.RepoSuite
}

func (s *cmdStorageSuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)
	setupTestStorageSupport(c, s.State)
}

func runShow(c *gc.C, expectedError string, args ...string) {
	cmdArgs := append([]string{"storage", "show"}, args...)
	context, err := runJujuCommand(c, cmdArgs...)
	if expectedError == "" {
		c.Assert(err, jc.ErrorIsNil)
	} else {
		c.Assert(err, gc.NotNil)
		c.Assert(testing.Stderr(context), jc.Contains, expectedError)
	}
}

func (s *cmdStorageSuite) TestStorageShowEmpty(c *gc.C) {
	runShow(c, "must specify storage id")
}

func (s *cmdStorageSuite) TestStorageShowInvalidId(c *gc.C) {
	runShow(c, "invalid storage id", "fluff")
}

func (s *cmdStorageSuite) TestStorageShow(c *gc.C) {
	createUnitWithStorage(c, &s.JujuConnSuite, testPool)

	expected := `
data/0:
  kind: block
  status:
    current: pending
    since: .*
  persistent: false
  attachments:
    units:
      storage-block/0:
        machine: "0"
`[1:]
	context, err := runJujuCommand(c, "storage", "show", "data/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Matches, expected)
}

func (s *cmdStorageSuite) TestStorageShowOneInvalid(c *gc.C) {
	createUnitWithStorage(c, &s.JujuConnSuite, testPool)

	runShow(c, "storage instance \"fluff/0\" not found", "data/0", "fluff/0")
}

func (s *cmdStorageSuite) TestStorageShowNoMatch(c *gc.C) {
	createUnitWithStorage(c, &s.JujuConnSuite, testPool)
	runShow(c, "storage instance \"fluff/0\" not found", "data/0", "fluff/0")
}

func runList(c *gc.C, expectedOutput string, args ...string) {
	cmdArgs := append([]string{"storage", "list"}, args...)
	context, err := runJujuCommand(c, cmdArgs...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, expectedOutput)
}

func (s *cmdStorageSuite) TestStorageListEmpty(c *gc.C) {
	runList(c, "")
}

func (s *cmdStorageSuite) TestStorageList(c *gc.C) {
	createUnitWithStorage(c, &s.JujuConnSuite, testPool)

	expected := `
[Storage]       
UNIT            ID     LOCATION STATUS  MESSAGE 
storage-block/0 data/0          pending         

`[1:]
	runList(c, expected)
}

func (s *cmdStorageSuite) TestStorageListPersistent(c *gc.C) {
	createUnitWithStorage(c, &s.JujuConnSuite, testPool)

	// There are currently no guarantees about whether storage
	// will be persistent until it has been provisioned.
	expected := `
[Storage]       
UNIT            ID     LOCATION STATUS  MESSAGE 
storage-block/0 data/0          pending         

`[1:]
	runList(c, expected)
}

func (s *cmdStorageSuite) TestStoragePersistentProvisioned(c *gc.C) {
	createUnitWithStorage(c, &s.JujuConnSuite, testPool)
	vol, err := s.State.StorageInstanceVolume(names.NewStorageTag("data/0"))
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.SetVolumeInfo(vol.VolumeTag(), state.VolumeInfo{
		Size:       1024,
		Persistent: true,
		VolumeId:   "vol-ume",
	})
	c.Assert(err, jc.ErrorIsNil)

	expected := `
data/0:
  kind: block
  status:
    current: pending
    since: .*
  persistent: true
  attachments:
    units:
      storage-block/0:
        machine: "0"
`[1:]
	context, err := runJujuCommand(c, "storage", "show", "data/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Matches, expected)
}

func (s *cmdStorageSuite) TestStoragePersistentUnprovisioned(c *gc.C) {
	createUnitWithStorage(c, &s.JujuConnSuite, testPool)

	// There are currently no guarantees about whether storage
	// will be persistent until it has been provisioned.
	expected := `
data/0:
  kind: block
  status:
    current: pending
    since: .*
  persistent: false
  attachments:
    units:
      storage-block/0:
        machine: "0"
`[1:]
	context, err := runJujuCommand(c, "storage", "show", "data/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Matches, expected)
}

func runJujuCommand(c *gc.C, args ...string) (*cmd.Context, error) {
	// NOTE (alesstimec): Writers need to be reset, because
	// they are set globally in the juju/cmd package and will
	// return an error if we attempt to run two commands in the
	// same test.
	loggo.RemoveWriter("warning")
	ctx, err := cmd.DefaultContext()
	c.Assert(err, jc.ErrorIsNil)
	command := jujucmd.NewJujuCommand(ctx)
	return testing.RunCommand(c, command, args...)
}

func runPoolList(c *gc.C, args ...string) (string, string, error) {
	cmdArgs := append([]string{"storage", "pool", "list"}, args...)
	ctx, err := runJujuCommand(c, cmdArgs...)
	stdout, stderr := "", ""
	if ctx != nil {
		stdout = testing.Stdout(ctx)
		stderr = testing.Stderr(ctx)
	}
	return stdout, stderr, err
}

func (s *cmdStorageSuite) TestListPools(c *gc.C) {
	stdout, _, err := runPoolList(c)
	c.Assert(err, jc.ErrorIsNil)
	expected := `
block:
  provider: loop
  attrs:
    it: works
ebs:
  provider: ebs
loop:
  provider: loop
rootfs:
  provider: rootfs
tmpfs:
  provider: tmpfs
`[1:]
	c.Assert(stdout, gc.Equals, expected)
}

func (s *cmdStorageSuite) TestListPoolsTabular(c *gc.C) {
	stdout, _, err := runPoolList(c, "--format", "tabular")
	c.Assert(err, jc.ErrorIsNil)
	expected := `
NAME    PROVIDER  ATTRS
block   loop      it=works
ebs     ebs       
loop    loop      
rootfs  rootfs    
tmpfs   tmpfs     

`[1:]
	c.Assert(stdout, gc.Equals, expected)
}

func (s *cmdStorageSuite) TestListPoolsName(c *gc.C) {
	stdout, _, err := runPoolList(c, "--name", "block")
	c.Assert(err, jc.ErrorIsNil)
	expected := `
block:
  provider: loop
  attrs:
    it: works
`[1:]
	c.Assert(stdout, gc.Equals, expected)
}

func (s *cmdStorageSuite) TestListPoolsNameNoMatch(c *gc.C) {
	stdout, stderr, err := runPoolList(c, "--name", "cranky")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stderr, gc.Equals, "")
	c.Assert(stdout, gc.Equals, "")
}

func (s *cmdStorageSuite) TestListPoolsNameInvalid(c *gc.C) {
	_, stderr, err := runPoolList(c, "--name", "9oops")
	c.Assert(err, gc.NotNil)
	c.Assert(stderr, jc.Contains, `ERROR pool name "9oops" not valid`)
}

func (s *cmdStorageSuite) TestListPoolsProvider(c *gc.C) {
	stdout, _, err := runPoolList(c, "--provider", "loop")
	c.Assert(err, jc.ErrorIsNil)
	expected := `
block:
  provider: loop
  attrs:
    it: works
loop:
  provider: loop
`[1:]
	c.Assert(stdout, gc.Equals, expected)
}

func (s *cmdStorageSuite) registerTmpProviderType(c *gc.C) {
	cfg, err := s.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	registry.RegisterEnvironStorageProviders(cfg.Name(), provider.TmpfsProviderType)
}

func (s *cmdStorageSuite) TestListPoolsProviderNoMatch(c *gc.C) {
	s.registerTmpProviderType(c)
	stdout, _, err := runPoolList(c, "--provider", string(provider.TmpfsProviderType))
	c.Assert(err, jc.ErrorIsNil)
	expected := `
tmpfs:
  provider: tmpfs
`[1:]
	c.Assert(stdout, gc.Equals, expected)
}

func (s *cmdStorageSuite) TestListPoolsProviderUnregistered(c *gc.C) {
	_, stderr, err := runPoolList(c, "--provider", "oops")
	c.Assert(err, gc.NotNil)
	c.Assert(stderr, jc.Contains, `"oops" for environment "dummyenv" not supported`)
}

func (s *cmdStorageSuite) TestListPoolsNameAndProvider(c *gc.C) {
	stdout, _, err := runPoolList(c, "--name", "block", "--provider", "loop")
	c.Assert(err, jc.ErrorIsNil)
	expected := `
block:
  provider: loop
  attrs:
    it: works
`[1:]
	c.Assert(stdout, gc.Equals, expected)
}

func (s *cmdStorageSuite) TestListPoolsProviderAndNotName(c *gc.C) {
	stdout, _, err := runPoolList(c, "--name", "fluff", "--provider", "ebs")
	c.Assert(err, jc.ErrorIsNil)
	// there is no pool that matches this name AND type
	c.Assert(stdout, gc.Equals, "")
}

func (s *cmdStorageSuite) TestListPoolsNameAndNotProvider(c *gc.C) {
	s.registerTmpProviderType(c)
	stdout, _, err := runPoolList(c, "--name", "block", "--provider", string(provider.TmpfsProviderType))
	c.Assert(err, jc.ErrorIsNil)
	// no pool matches this name and this provider
	c.Assert(stdout, gc.Equals, "")
}

func (s *cmdStorageSuite) TestListPoolsNotNameAndNotProvider(c *gc.C) {
	s.registerTmpProviderType(c)
	stdout, _, err := runPoolList(c, "--name", "fluff", "--provider", string(provider.TmpfsProviderType))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stdout, gc.Equals, "")
}

func runPoolCreate(c *gc.C, args ...string) (string, string, error) {
	cmdArgs := append([]string{"storage", "pool", "create"}, args...)
	ctx, err := runJujuCommand(c, cmdArgs...)
	stdout, stderr := "", ""
	if ctx != nil {
		stdout = testing.Stdout(ctx)
		stderr = testing.Stderr(ctx)
	}
	return stdout, stderr, err

}

func (s *cmdStorageSuite) TestCreatePool(c *gc.C) {
	pname := "ftPool"
	stdout, _, err := runPoolCreate(c, pname, "loop", "smth=one")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stdout, gc.Equals, "")
	assertPoolExists(c, s.State, pname, "loop", "smth=one")
}

func (s *cmdStorageSuite) assertCreatePoolError(c *gc.C, errString, expected string, args ...string) {
	_, stderr, err := runPoolCreate(c, args...)
	if errString != "" {
		c.Assert(err, gc.ErrorMatches, errString)
	} else {
		c.Assert(err, gc.NotNil)
	}

	c.Assert(stderr, jc.Contains, expected)
}

func (s *cmdStorageSuite) TestCreatePoolErrorNoAttrs(c *gc.C) {
	s.assertCreatePoolError(c, "pool creation requires names, provider type and attrs for configuration", "", "loop", "ftPool")
}

func (s *cmdStorageSuite) TestCreatePoolErrorNoProvider(c *gc.C) {
	s.assertCreatePoolError(c, "pool creation requires names, provider type and attrs for configuration", "", "oops provider", "smth=one")
}

func (s *cmdStorageSuite) TestCreatePoolErrorProviderType(c *gc.C) {
	s.assertCreatePoolError(c, "", "not found", "loop", "ftPool", "smth=one")
}

func (s *cmdStorageSuite) TestCreatePoolDuplicateName(c *gc.C) {
	pname := "ftPool"
	stdout, _, err := runPoolCreate(c, pname, "loop", "smth=one")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stdout, gc.Equals, "")
	assertPoolExists(c, s.State, pname, "loop", "smth=one")
	s.assertCreatePoolError(c, "", "cannot overwrite existing settings", pname, "loop", "smth=one")
}

func assertPoolExists(c *gc.C, st *state.State, pname, provider, attr string) {
	stsetts := state.NewStateSettings(st)
	poolManager := poolmanager.New(stsetts)

	found, err := poolManager.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(found) > 0, jc.IsTrue)

	exists := false
	for _, one := range found {
		if one.Name() == pname {
			exists = true
			c.Assert(string(one.Provider()), gc.Equals, provider)
			// At this stage, only 1 attr is expected and checked
			expectedAttrs := strings.Split(attr, "=")
			value, ok := one.Attrs()[expectedAttrs[0]]
			c.Assert(ok, jc.IsTrue)
			c.Assert(value, gc.Equals, expectedAttrs[1])
		}
	}
	c.Assert(exists, jc.IsTrue)
}

func runVolumeList(c *gc.C, args ...string) (string, string, error) {
	cmdArgs := append([]string{"storage", "volume", "list"}, args...)
	ctx, err := runJujuCommand(c, cmdArgs...)
	return testing.Stdout(ctx), testing.Stderr(ctx), err
}

func (s *cmdStorageSuite) TestListVolumeInvalidMachine(c *gc.C) {
	_, stderr, err := runVolumeList(c, "abc")
	c.Assert(err, gc.ErrorMatches, "cmd: error out silently")
	c.Assert(stderr, jc.Contains, `"machine-abc" is not a valid machine tag`)
}

func (s *cmdStorageSuite) TestListVolumeTabularFilterMatch(c *gc.C) {
	createUnitWithStorage(c, &s.JujuConnSuite, testPool)
	stdout, _, err := runVolumeList(c, "0")
	c.Assert(err, jc.ErrorIsNil)
	expected := `
MACHINE  UNIT             STORAGE  ID   PROVIDER-ID  DEVICE  SIZE  STATE    MESSAGE
0        storage-block/0  data/0   0/0                             pending  

`[1:]
	c.Assert(stdout, gc.Equals, expected)
}

func runAddToUnit(c *gc.C, args ...string) *cmd.Context {
	cmdArgs := append([]string{"storage", "add"}, args...)
	context, err := runJujuCommand(c, cmdArgs...)
	c.Assert(err, jc.ErrorIsNil)
	return context
}

func (s *cmdStorageSuite) TestStorageAddToUnitSuccess(c *gc.C) {
	u := createUnitWithStorage(c, &s.JujuConnSuite, testPool)
	instancesBefore, err := s.State.AllStorageInstances()
	c.Assert(err, jc.ErrorIsNil)
	volumesBefore, err := s.State.AllVolumes()
	c.Assert(err, jc.ErrorIsNil)
	s.assertStorageExist(c, instancesBefore, "data")

	context := runAddToUnit(c, u, "allecto=1")
	c.Assert(testing.Stdout(context), gc.Equals, "")
	c.Assert(testing.Stderr(context), gc.Equals, "")

	instancesAfter, err := s.State.AllStorageInstances()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(instancesAfter)-len(instancesBefore), gc.Equals, 1)
	volumesAfter, err := s.State.AllVolumes()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(volumesAfter)-len(volumesBefore), gc.Equals, 1)
	s.assertStorageExist(c, instancesAfter, "data", "allecto")
}

func (s *cmdStorageSuite) assertStorageExist(c *gc.C,
	all []state.StorageInstance,
	expected ...string) {

	names := make([]string, len(all))
	for i, one := range all {
		names[i] = one.StorageName()
	}
	c.Assert(names, jc.SameContents, expected)
}

func (s *cmdStorageSuite) TestStorageAddToUnitFailure(c *gc.C) {
	context := runAddToUnit(c, "fluffyunit/0", "allecto=1")
	c.Assert(testing.Stdout(context), gc.Equals, "")
	c.Assert(testing.Stderr(context), gc.Equals, "fail: storage \"allecto\": permission denied\n")
}

func (s *cmdStorageSuite) TestStorageAddToUnitHasVolumes(c *gc.C) {
	// Reproducing Bug1462146
	u := createUnitWithFileSystemStorage(c, &s.JujuConnSuite, "ebs")
	instancesBefore, err := s.State.AllStorageInstances()
	c.Assert(err, jc.ErrorIsNil)
	s.assertStorageExist(c, instancesBefore, "data")
	volumesBefore, err := s.State.AllVolumes()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volumesBefore, gc.HasLen, 1)

	context, err := runJujuCommand(c, "storage", "list")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, `
[Storage]            
UNIT                 ID     LOCATION STATUS  MESSAGE 
storage-filesystem/0 data/0          pending         

`[1:])
	c.Assert(testing.Stderr(context), gc.Equals, "")

	context = runAddToUnit(c, u, "data=ebs,1G")
	c.Assert(testing.Stdout(context), gc.Equals, "")
	c.Assert(testing.Stderr(context), gc.Equals, "")

	instancesAfter, err := s.State.AllStorageInstances()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(instancesAfter)-len(instancesBefore), gc.Equals, 1)
	s.assertStorageExist(c, instancesAfter, "data", "data")
	volumesAfter, err := s.State.AllVolumes()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volumesAfter, gc.HasLen, 2)

	context, err = runJujuCommand(c, "storage", "list")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, `
[Storage]            
UNIT                 ID     LOCATION STATUS  MESSAGE 
storage-filesystem/0 data/0          pending         
storage-filesystem/0 data/1          pending         

`[1:])
	c.Assert(testing.Stderr(context), gc.Equals, "")
}

func createUnitWithFileSystemStorage(c *gc.C, s *jujutesting.JujuConnSuite, poolName string) string {
	ch := s.AddTestingCharm(c, "storage-filesystem")
	storage := map[string]state.StorageConstraints{
		"data": makeStorageCons(poolName, 1024, 1),
	}
	service := s.AddTestingServiceWithStorage(c, "storage-filesystem", ch, storage)
	unit, err := service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.AssignUnit(unit, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)

	return unit.Tag().Id()
}
