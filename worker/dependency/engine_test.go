package dependency_test

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

type DependencySuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&DependencySuite{})

func (s *DependencySuite) TestInstallNoInputs(c *gc.C) {
	c.Fatalf("xxx")
}

func (s *DependencySuite) TestInstallUnknownInputs(c *gc.C) {
	c.Fatalf("xxx")
}

func (s *DependencySuite) TestDoubleInstall(c *gc.C) {
	c.Fatalf("xxx")
}

func (s *DependencySuite) TestStartGetResourceBadName(c *gc.C) {
	c.Fatalf("xxx")
}

func (s *DependencySuite) TestStartGetResourceBadType(c *gc.C) {
	c.Fatalf("xxx")
}

func (s *DependencySuite) TestStartGetResourceGoodType(c *gc.C) {
	c.Fatalf("xxx")
}

func (s *DependencySuite) TestStartGetResourceExistenceOnly(c *gc.C) {
	c.Fatalf("xxx")
}

func (s *DependencySuite) TestErrorRestartsDependents(c *gc.C) {
	c.Fatalf("xxx")
}

func (s *DependencySuite) TestErrorPreservesDependencies(c *gc.C) {
	c.Fatalf("xxx")
}

func (s *DependencySuite) TestRestartRestartsDependents(c *gc.C) {
	c.Fatalf("xxx")
}

func (s *DependencySuite) TestRestartPreservesDependencies(c *gc.C) {
	c.Fatalf("xxx")
}

func (s *DependencySuite) TestMore(c *gc.C) {
	c.Fatalf("xxx")
}
