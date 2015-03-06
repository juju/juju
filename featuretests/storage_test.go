// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"github.com/juju/cmd"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/storage"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	cmdstorage "github.com/juju/juju/cmd/juju/storage"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/juju"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/storage/provider"
	"github.com/juju/juju/storage/provider/registry"
	"github.com/juju/juju/testing"
)

var (
	testPool = "block"
)

type apiStorageSuite struct {
	jujutesting.JujuConnSuite
	storageClient *storage.Client
}

var _ = gc.Suite(&apiStorageSuite{})

func (s *apiStorageSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.SetFeatureFlags(feature.Storage)
	conn, err := juju.NewAPIState(s.AdminUserTag(c), s.Environ, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(*gc.C) { conn.Close() })

	setupTestStorageSupport(c, s.State)

	cfg, err := s.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)

	st, err := juju.NewAPIFromName(cfg.Name())
	c.Assert(err, jc.ErrorIsNil)
	s.storageClient = storage.NewClient(st)
	c.Assert(s.storageClient, gc.NotNil)
}

func setupTestStorageSupport(c *gc.C, s *state.State) {
	stsetts := state.NewStateSettings(s)
	poolManager := poolmanager.New(stsetts)
	_, err := poolManager.Create(testPool, provider.LoopProviderType, map[string]interface{}{"it": "works"})
	c.Assert(err, jc.ErrorIsNil)

	registry.RegisterEnvironStorageProviders("someprovider", provider.LoopProviderType)
}

func (s *apiStorageSuite) TearDownTest(c *gc.C) {
	s.storageClient.ClientFacade.Close()
	s.JujuConnSuite.TearDownTest(c)
}

func (s *apiStorageSuite) TestStorageShow(c *gc.C) {
	createUnitForTest(c, &s.JujuConnSuite)

	storageTag, err := names.ParseStorageTag("storage-data-0")
	c.Assert(err, jc.ErrorIsNil)
	found, err := s.storageClient.Show([]names.StorageTag{storageTag})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.HasLen, 1)
	one := found[0]
	c.Assert(one.StorageTag, gc.DeepEquals, "storage-data-0")
	c.Assert(one.OwnerTag, gc.DeepEquals, "unit-storage-block-0")
	c.Assert(one.UnitTag, gc.DeepEquals, "unit-storage-block-0")
	c.Assert(one.Location, gc.DeepEquals, "")
	c.Assert(one.Provisioned, jc.IsFalse)
	c.Assert(one.Attached, jc.IsTrue)
	c.Assert(one.Kind, gc.DeepEquals, params.StorageKindBlock)
}

func (s *apiStorageSuite) TestStorageShowEmpty(c *gc.C) {
	found, err := s.storageClient.Show(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.HasLen, 0)
}

func (s *apiStorageSuite) TestStorageList(c *gc.C) {
	createUnitForTest(c, &s.JujuConnSuite)

	found, err := s.storageClient.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.HasLen, 1)
	one := found[0]
	c.Assert(one.StorageTag, gc.DeepEquals, "storage-data-0")
	c.Assert(one.OwnerTag, gc.DeepEquals, "unit-storage-block-0")
	c.Assert(one.Kind, gc.DeepEquals, params.StorageKindBlock)
	c.Assert(one.Location, gc.DeepEquals, "")
	c.Assert(one.Provisioned, jc.IsFalse)
	c.Assert(one.Attached, jc.IsTrue)
}

func (s *apiStorageSuite) TestStorageListEmpty(c *gc.C) {
	found, err := s.storageClient.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.HasLen, 0)
}

func makeStorageCons(pool string, size, count uint64) state.StorageConstraints {
	return state.StorageConstraints{Pool: pool, Size: size, Count: count}
}

func createUnitForTest(c *gc.C, s *jujutesting.JujuConnSuite) string {
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

func (s *cmdStorageSuite) TestStorageShowCmdStack(c *gc.C) {
	createUnitForTest(c, &s.JujuConnSuite)

	context := runShow(c, []string{"data/0"})
	expected := `
storage-block/0:
  data/0:
    storage: data
    kind: block
    unit_id: storage-block/0
    attached: true
`[1:]
	c.Assert(testing.Stdout(context), gc.Equals, expected)
}

func runList(c *gc.C) *cmd.Context {
	context, err := testing.RunCommand(c, envcmd.Wrap(&cmdstorage.ListCommand{}))
	c.Assert(err, jc.ErrorIsNil)
	return context
}
func (s *cmdStorageSuite) TestStorageListCmdStack(c *gc.C) {
	createUnitForTest(c, &s.JujuConnSuite)

	context := runList(c)
	expected := `
[Storage]       
OWNER           ID     NAME ATTACHED        LOCATION KIND  
storage-block/0 data/0 data storage-block/0          block 

`[1:]
	c.Assert(testing.Stdout(context), gc.Equals, expected)
}
