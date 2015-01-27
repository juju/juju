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
	"github.com/juju/juju/testing"
	"github.com/juju/names"
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
	// TODO(anastasiamac) update when s.Factory.MakeStorage or similar is available
	storageTag := names.NewStorageTag("shared-fs/0")
	found, err := s.storageClient.Show([]names.StorageTag{storageTag})
	c.Assert(err.Error(), gc.Matches, ".*permission denied.*")
	c.Assert(found, gc.HasLen, 0)
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
	c.Assert(err.Error(), gc.Matches, ".*permission denied.*")
	return context
}

func (s *cmdStorageSuite) TestStorageShowCmdStack(c *gc.C) {
	// TODO(anastasiamac) update when s.Factory.MakeStorage or similar is available
	context := runShow(c, []string{"shared-fs/0"})
	obtained := strings.Replace(testing.Stdout(context), "\n", "", -1)
	expected := ""
	c.Assert(obtained, gc.Equals, expected)
}
