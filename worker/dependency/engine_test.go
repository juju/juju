package dependency_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"launchpad.net/tomb"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

type DependencySuite struct {
	testing.IsolationSuite
	engine dependency.Engine
}

var _ = gc.Suite(&DependencySuite{})

func (s *DependencySuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.engine = dependency.NewEngine(nothingFatal, coretesting.ShortWait, coretesting.ShortWait/10)
}

func (s *DependencySuite) TearDownTest(c *gc.C) {
	if s.engine != nil {
		err := worker.Stop(s.engine)
		s.engine = nil
		c.Check(err, jc.ErrorIsNil)
	}
	s.IsolationSuite.TearDownTest(c)
}

func (s *DependencySuite) TestInstallNoInputs(c *gc.C) {
	err := s.engine.Install("some-task", dependency.Manifold{Start: degenerateStart})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *DependencySuite) TestInstallUnknownInputs(c *gc.C) {
	c.Fatalf("xxx")
}

func (s *DependencySuite) TestDoubleInstall(c *gc.C) {
	err := s.engine.Install("some-task", dependency.Manifold{Start: degenerateStart})
	c.Assert(err, jc.ErrorIsNil)
	err = s.engine.Install("some-task", dependency.Manifold{Start: degenerateStart})
	c.Assert(err, gc.ErrorMatches, "some-task manifold already installed")
}

func (s *DependencySuite) TestInstallAlreadyStopped(c *gc.C) {
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

func nothingFatal(_ error) bool {
	return false
}

type degenerateWorker struct {
	tomb tomb.Tomb
}

func (w *degenerateWorker) Kill() {
	w.tomb.Kill(nil)
}

func (w *degenerateWorker) Wait() error {
	return w.tomb.Wait()
}

func degenerateStart(_ dependency.GetResourceFunc) (worker.Worker, error) {
	w := &degenerateWorker{}
	go func() {
		<-w.tomb.Dying()
		w.tomb.Done()
	}()
	return w, nil
}
