package rebootstate_test

import (
	"os"
	"path/filepath"
	"testing"

	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/utils/rebootstate"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type rebootstateSuite struct {
	jujutesting.IsolationSuite
}

var _ = gc.Suite(&rebootstateSuite{})

func (s *rebootstateSuite) SetUpTest(c *gc.C) {
	dataDir := c.MkDir()
	s.PatchValue(&rebootstate.RebootStateFile, filepath.Join(dataDir, "reboot-state.txt"))
	s.IsolationSuite.SetUpTest(c)
}

func (s *rebootstateSuite) fileExists(f string) bool {
	if _, err := os.Stat(f); err != nil {
		return false
	}
	return true
}

func (s *rebootstateSuite) TestNewState(c *gc.C) {
	err := rebootstate.New()
	c.Assert(err, gc.IsNil)
	c.Assert(s.fileExists(rebootstate.RebootStateFile), jc.IsTrue)
}

func (s *rebootstateSuite) TestMultipleNewState(c *gc.C) {
	err := rebootstate.New()
	c.Assert(err, gc.IsNil)
	err = rebootstate.New()
	c.Assert(err, gc.ErrorMatches, "state file (.*) already exists")
}

func (s *rebootstateSuite) TestIsPresent(c *gc.C) {
	err := rebootstate.New()
	c.Assert(err, gc.IsNil)
	exists := rebootstate.IsPresent()
	c.Assert(exists, jc.IsTrue)
}

func (s *rebootstateSuite) TestRemoveState(c *gc.C) {
	err := rebootstate.New()
	c.Assert(err, gc.IsNil)
	err = rebootstate.Remove()
	c.Assert(err, gc.IsNil)
	c.Assert(s.fileExists(rebootstate.RebootStateFile), jc.IsFalse)
}
