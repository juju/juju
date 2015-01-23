// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"strings"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/storage"
	"github.com/juju/juju/cmd/envcmd"
	cmdstorage "github.com/juju/juju/cmd/juju/storage"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/juju"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
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

	s.storageClient = storage.NewClient(conn)
	c.Assert(s.storageClient, gc.NotNil)
}

func (s *apiStorageSuite) TearDownTest(c *gc.C) {
	s.storageClient.ClientFacade.Close()
	s.JujuConnSuite.TearDownTest(c)
}

func (s *apiStorageSuite) TestStorageShow(c *gc.C) {
	unit := s.createTestUnit(c)
	found, err := s.storageClient.Show(unit.Name(), "test-storage")
	c.Assert(err.Error(), gc.Matches, ".*not implemented.*")
	c.Assert(found, gc.HasLen, 0)
}

func (s *apiStorageSuite) createTestUnit(c *gc.C) *state.Unit {
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Jobs: []state.MachineJob{state.JobHostUnits},
	})
	charm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "wordpress"})
	wordpress := s.Factory.MakeService(c, &factory.ServiceParams{
		Charm: charm,
	})
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{
		Service: wordpress,
		Machine: machine,
	})
	return unit
}

type cmdStorageSuite struct {
	jujutesting.RepoSuite
}

var _ = gc.Suite(&cmdStorageSuite{})

func (s *cmdStorageSuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)
	s.SetFeatureFlags(feature.Storage)
}

func runShow(c *gc.C, args []string) *cmd.Context {
	context, err := testing.RunCommand(c, envcmd.Wrap(&cmdstorage.ShowCommand{}), args...)
	c.Assert(err.Error(), gc.Matches, ".*This needs to plug into a real deal! not implemented.*")
	return context
}

func (s *cmdStorageSuite) TestStorageShowCmdStack(c *gc.C) {
	context := runShow(c, []string{"--unit", "test-unit", "--storage", "test-storage"})
	obtained := strings.Replace(testing.Stdout(context), "\n", "", -1)
	expected := ""
	c.Assert(obtained, gc.Equals, expected)
}
