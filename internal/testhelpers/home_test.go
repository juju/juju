// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package testhelpers_test

import (
	"io/ioutil"
	"path/filepath"
	"runtime"

	"github.com/juju/tc"
	"github.com/juju/utils/v4"

	testing "github.com/juju/juju/internal/testhelpers"
)

type fakeHomeSuite struct {
	testing.IsolationSuite
	fakeHomeSuite testing.FakeHomeSuite
}

var _ = tc.Suite(&fakeHomeSuite{})

func (s *fakeHomeSuite) SetUpSuite(c *tc.C) {
	s.IsolationSuite.SetUpSuite(c)
	s.fakeHomeSuite = testing.FakeHomeSuite{}
	s.fakeHomeSuite.SetUpSuite(c)
}

func (s *fakeHomeSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	err := utils.SetHome("/tmp/tests")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *fakeHomeSuite) TearDownSuite(c *tc.C) {
	s.fakeHomeSuite.TearDownSuite(c)
	s.IsolationSuite.TearDownSuite(c)
}

func (s *fakeHomeSuite) TestHomeCreated(c *tc.C) {
	// A fake home is created and set.
	s.fakeHomeSuite.SetUpTest(c)
	home := utils.Home()
	c.Assert(home, tc.Not(tc.Equals), "/tmp/tests")
	c.Assert(home, tc.IsDirectory)
	s.fakeHomeSuite.TearDownTest(c)
	// The original home has been restored.
	switch runtime.GOOS {
	case "windows":
		c.Assert(utils.Home(), tc.SamePath, "C:/tmp/tests")
	default:
		c.Assert(utils.Home(), tc.SamePath, "/tmp/tests")
	}
}

func (s *fakeHomeSuite) TestSshDirSetUp(c *tc.C) {
	// The SSH directory is properly created and set up.
	s.fakeHomeSuite.SetUpTest(c)
	sshDir := testing.HomePath(".ssh")
	c.Assert(sshDir, tc.IsDirectory)
	PrivKeyFile := filepath.Join(sshDir, "id_rsa")
	c.Assert(PrivKeyFile, tc.IsNonEmptyFile)
	PubKeyFile := filepath.Join(sshDir, "id_rsa.pub")
	c.Assert(PubKeyFile, tc.IsNonEmptyFile)
	s.fakeHomeSuite.TearDownTest(c)
}

type makeFakeHomeSuite struct {
	testing.IsolationSuite
	home *testing.FakeHome
}

var _ = tc.Suite(&makeFakeHomeSuite{})

func (s *makeFakeHomeSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.home = testing.MakeFakeHome(c)
	testFile := testing.TestFile{
		Name: "testfile-name",
		Data: "testfile-data",
	}
	s.home.AddFiles(c, testFile)
}

func (s *makeFakeHomeSuite) TestAddFiles(c *tc.C) {
	// Files are correctly added to the fake home.
	expectedPath := filepath.Join(utils.Home(), "testfile-name")
	contents, err := ioutil.ReadFile(expectedPath)
	c.Assert(err, tc.IsNil)
	c.Assert(string(contents), tc.Equals, "testfile-data")
}

func (s *makeFakeHomeSuite) TestFileContents(c *tc.C) {
	// Files contents are returned as strings.
	contents := s.home.FileContents(c, "testfile-name")
	c.Assert(contents, tc.Equals, "testfile-data")
}

func (s *makeFakeHomeSuite) TestFileExists(c *tc.C) {
	// It is possible to check whether a file exists in the fake home.
	c.Assert(s.home.FileExists("testfile-name"), tc.IsTrue)
	c.Assert(s.home.FileExists("no-such-file"), tc.IsFalse)
}
