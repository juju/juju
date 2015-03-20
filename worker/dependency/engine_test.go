package dependency_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"launchpad.net/tomb"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

type EngineSuite struct {
	testing.IsolationSuite
	engine dependency.Engine
}

var _ = gc.Suite(&EngineSuite{})

func (s *EngineSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.engine = dependency.NewEngine(nothingFatal, coretesting.ShortWait, coretesting.ShortWait/10)
}

func (s *EngineSuite) TearDownTest(c *gc.C) {
	if s.engine != nil {
		err := worker.Stop(s.engine)
		s.engine = nil
		c.Check(err, jc.ErrorIsNil)
	}
	s.IsolationSuite.TearDownTest(c)
}

func (s *EngineSuite) TestInstallNoInputs(c *gc.C) {
	err := s.engine.Install("some-task", dependency.Manifold{Start: degenerateStart})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *EngineSuite) TestInstallUnknownInputs(c *gc.C) {
	err := s.engine.Install("some-task", dependency.Manifold{
		Start:  degenerateStart,
		Inputs: []string{"unknown-task"},
	})
	c.Assert(err, gc.ErrorMatches, "some-task manifold depends on unknown unknown-task manifold")
}

func (s *EngineSuite) TestDoubleInstall(c *gc.C) {
	err := s.engine.Install("some-task", dependency.Manifold{Start: degenerateStart})
	c.Assert(err, jc.ErrorIsNil)

	err = s.engine.Install("some-task", dependency.Manifold{Start: degenerateStart})
	c.Assert(err, gc.ErrorMatches, "some-task manifold already installed")
}

func (s *EngineSuite) TestInstallAlreadyStopped(c *gc.C) {
	err := worker.Stop(s.engine)
	c.Assert(err, jc.ErrorIsNil)

	err = s.engine.Install("some-task", dependency.Manifold{Start: degenerateStart})
	c.Assert(err, gc.ErrorMatches, "engine is shutting down")
}

func (s *EngineSuite) TestStartGetResourceExistenceOnly(c *gc.C) {
	err := s.engine.Install("some-task", dependency.Manifold{Start: degenerateStart})
	c.Assert(err, jc.ErrorIsNil)

	done := make(chan struct{})
	err = s.engine.Install("other-task", dependency.Manifold{
		Inputs: []string{"some-task"},
		Start: func(getResource dependency.GetResourceFunc) (worker.Worker, error) {
			if !getResource("some-task", nil) {
				// If some-task is slow to start, we may bounce.
				return nil, errors.New("need some-task")
			}
			close(done)
			return degenerateStart(getResource)
		},
	})
	c.Check(err, jc.ErrorIsNil)
	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("other-task never started")
	}
}

func (s *EngineSuite) TestStartGetResourceUndeclaredName(c *gc.C) {
	err := s.engine.Install("some-task", dependency.Manifold{Start: degenerateStart})
	c.Assert(err, jc.ErrorIsNil)

	done := make(chan struct{})
	err = s.engine.Install("other-task", dependency.Manifold{
		Start: func(getResource dependency.GetResourceFunc) (worker.Worker, error) {
			success := getResource("some-task", nil)
			c.Check(success, jc.IsFalse)
			close(done)
			return degenerateStart(getResource)
		},
	})
	c.Check(err, jc.ErrorIsNil)
	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("other-task never started")
	}
}

func (s *EngineSuite) TestStartGetResourceBadType(c *gc.C) {
	var target interface{}
	expectTarget := &target
	err := s.engine.Install("some-task", dependency.Manifold{
		Start: degenerateStart,
		Output: func(worker worker.Worker, target interface{}) bool {
			c.Check(target, gc.DeepEquals, expectTarget)
			return false
		},
	})

	c.Assert(err, jc.ErrorIsNil)
	done := make(chan struct{})
	err = s.engine.Install("other-task", dependency.Manifold{
		Inputs: []string{"some-task"},
		Start: func(getResource dependency.GetResourceFunc) (worker.Worker, error) {
			success := getResource("some-task", &target)
			c.Check(success, jc.IsFalse)
			close(done)
			return degenerateStart(getResource)
		},
	})
	c.Check(err, jc.ErrorIsNil)
	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("other-task never started")
	}
}

func (s *EngineSuite) TestStartGetResourceGoodType(c *gc.C) {
	var target interface{}
	expectTarget := &target
	err := s.engine.Install("some-task", dependency.Manifold{
		Start: degenerateStart,
		Output: func(worker worker.Worker, target interface{}) bool {
			c.Check(target, gc.DeepEquals, expectTarget)
			return true
		},
	})

	c.Assert(err, jc.ErrorIsNil)
	done := make(chan struct{})
	err = s.engine.Install("other-task", dependency.Manifold{
		Inputs: []string{"some-task"},
		Start: func(getResource dependency.GetResourceFunc) (worker.Worker, error) {
			if !getResource("some-task", &target) {
				// If some-task is slow to start, we may bounce.
				return nil, errors.New("need some-task")
			}
			close(done)
			return degenerateStart(getResource)
		},
	})
	c.Check(err, jc.ErrorIsNil)
	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("other-task never started")
	}
}

func (s *EngineSuite) TestErrorRestartsDependents(c *gc.C) {

	// Start a task that will error out, once, when we tell it to.
	shouldError := make(chan struct{})
	hasErrored := make(chan struct{})
	err := s.engine.Install("error-task", dependency.Manifold{
		Start: func(_ dependency.GetResourceFunc) (worker.Worker, error) {
			w := &degenerateWorker{}
			go func() {
				<-w.tomb.Dying()
				w.tomb.Done()
			}()
			go func() {
				<-shouldError
				select {
				case <-hasErrored:
				default:
					w.tomb.Kill(errors.New("BLAM"))
					close(hasErrored)
				}
			}()
			return w, nil
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	// Start another task depending on that one.
	happyStarts := make(chan struct{})
	err = s.engine.Install("happy-task", dependency.Manifold{
		Inputs: []string{"error-task"},
		Start: func(getResource dependency.GetResourceFunc) (worker.Worker, error) {
			if !getResource("error-task", nil) {
				return nil, errors.New("need error-task")
			}
			happyStarts <- struct{}{}
			return degenerateStart(getResource)
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	// Wait for the dependent task to start, then induce the error.
	timeout := time.After(coretesting.LongWait)
	select {
	case <-happyStarts:
		close(shouldError)
	case <-timeout:
		c.Fatalf("dependent task never started")
	}

	// Wait for the dependent task to restart.
	select {
	case <-happyStarts:
	case <-timeout:
		c.Fatalf("dependent task never restarted")
	}

	// Check it doesn't restart again.
	select {
	case <-happyStarts:
		c.Fatalf("dependent task restarted unexpectedly")
	case <-time.After(coretesting.ShortWait):
	}
}

func (s *EngineSuite) TestErrorPreservesDependencies(c *gc.C) {
	c.Fatalf("xxx")
}

func (s *EngineSuite) TestRestartRestartsDependents(c *gc.C) {
	c.Fatalf("xxx")
}

func (s *EngineSuite) TestRestartPreservesDependencies(c *gc.C) {
	c.Fatalf("xxx")
}

func (s *EngineSuite) TestCompletedWorkerNotRestartedOnExit(c *gc.C) {
	c.Fatalf("xxx")
}

func (s *EngineSuite) TestCompletedWorkerRestartedByDependencyChange(c *gc.C) {
	c.Fatalf("xxx")
}

func (s *EngineSuite) TestMore(c *gc.C) {
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
