// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelworkermanager_test

import (
	stdtesting "testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"launchpad.net/tomb"

	cmdutil "github.com/juju/juju/cmd/jujud/util"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/modelworkermanager"
)

func TestPackage(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

var _ = gc.Suite(&suite{})

type suite struct {
	statetesting.StateSuite
	factory  *factory.Factory
	runnerC  chan *fakeRunner
	startErr error
}

func (s *suite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)
	s.factory = factory.NewFactory(s.State)
	s.runnerC = make(chan *fakeRunner, 1)
	s.startErr = nil
}

func (s *suite) TearDownTest(c *gc.C) {
	close(s.runnerC)
	s.StateSuite.TearDownTest(c)
}

func (s *suite) MakeModel(c *gc.C) *state.State {
	st := s.factory.MakeModel(c, nil)
	s.AddCleanup(func(*gc.C) { st.Close() })
	return st
}

func destroyEnvironment(c *gc.C, st *state.State) {
	env, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = env.Destroy()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *suite) TestStartsWorkersForPreExistingEnvs(c *gc.C) {
	moreState := s.MakeModel(c)

	var seenEnvs []string
	m := modelworkermanager.NewModelWorkerManager(s.State, s.startEnvWorker, s.dyingEnvWorker, time.Millisecond)
	defer m.Kill()
	for _, r := range s.seeRunnersStart(c, 2) {
		seenEnvs = append(seenEnvs, r.modelUUID)
	}

	c.Assert(seenEnvs, jc.SameContents,
		[]string{s.State.ModelUUID(), moreState.ModelUUID()},
	)

	destroyEnvironment(c, moreState)
	dyingRunner := s.seeRunnersStart(c, 1)[0]
	c.Assert(dyingRunner.modelUUID, gc.Equals, moreState.ModelUUID())
}

func (s *suite) TestStartsWorkersForNewEnv(c *gc.C) {
	m := modelworkermanager.NewModelWorkerManager(s.State, s.startEnvWorker, s.dyingEnvWorker, time.Millisecond)
	defer m.Kill()
	s.seeRunnersStart(c, 1) // Runner for controller env

	// Create another environment and watch a runner be created for it.
	st2 := s.MakeModel(c)
	runner := s.seeRunnersStart(c, 1)[0]
	c.Assert(runner.modelUUID, gc.Equals, st2.ModelUUID())
}

func (s *suite) TestStopsWorkersWhenEnvGoesAway(c *gc.C) {
	m := modelworkermanager.NewModelWorkerManager(s.State, s.startEnvWorker, s.dyingEnvWorker, time.Millisecond)
	defer m.Kill()
	runner0 := s.seeRunnersStart(c, 1)[0]

	// Create an environment and grab the runner for it.
	otherState := s.MakeModel(c)
	runner1 := s.seeRunnersStart(c, 1)[0]

	// Set environment to dying.
	destroyEnvironment(c, otherState)
	s.seeRunnersStart(c, 1) // dying env runner

	// Set environment to dead.
	err := otherState.ProcessDyingModel()
	c.Assert(err, jc.ErrorIsNil)

	// See that the first runner is still running but the runner for
	// the new environment is stopped.
	s.State.StartSync()
	select {
	case <-runner0.tomb.Dead():
		c.Fatal("first runner should not die here")
	case <-runner1.tomb.Dead():
		break
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for runner to die")
	}

	// Make sure the first runner doesn't get stopped.
	s.State.StartSync()
	select {
	case <-runner0.tomb.Dead():
		c.Fatal("first runner should not die here")
	case <-time.After(testing.ShortWait):
		break
	}
}

func (s *suite) TestKillPropagates(c *gc.C) {
	otherSt := s.MakeModel(c)

	m := modelworkermanager.NewModelWorkerManager(s.State, s.startEnvWorker, s.dyingEnvWorker, time.Millisecond)
	runners := s.seeRunnersStart(c, 2)
	c.Assert(runners[0].killed, jc.IsFalse)
	c.Assert(runners[1].killed, jc.IsFalse)

	destroyEnvironment(c, otherSt)
	dyingRunner := s.seeRunnersStart(c, 1)[0]
	c.Assert(dyingRunner.killed, jc.IsFalse)

	m.Kill()
	err := waitOrFatal(c, m.Wait)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(runners[0].killed, jc.IsTrue)
	c.Assert(runners[1].killed, jc.IsTrue)
	c.Assert(dyingRunner.killed, jc.IsTrue)
}

// stateWithFailingGetEnvironment wraps a *state.State, overriding the
// GetModel to generate an error.
type stateWithFailingGetEnvironment struct {
	*stateWithFakeWatcher
	shouldFail bool
}

func newStateWithFailingGetEnvironment(realSt *state.State) *stateWithFailingGetEnvironment {
	return &stateWithFailingGetEnvironment{
		stateWithFakeWatcher: newStateWithFakeWatcher(realSt),
		shouldFail:           false,
	}
}

func (s *stateWithFailingGetEnvironment) GetModel(tag names.ModelTag) (*state.Model, error) {
	if s.shouldFail {
		return nil, errors.New("unable to GetModel")
	}
	return s.State.GetModel(tag)
}

func (s *suite) TestLoopExitKillsRunner(c *gc.C) {
	// If something causes EnvWorkerManager.loop to exit that isn't Kill() then it should stop the runner.
	// Currently the best way to cause this is to make
	// m.st.GetModel(tag) fail with any error other than NotFound
	otherSt := s.MakeModel(c)
	st := newStateWithFailingGetEnvironment(s.State)
	uuid := st.ModelUUID()
	m := modelworkermanager.NewModelWorkerManager(st, s.startEnvWorker, s.dyingEnvWorker, time.Millisecond)
	defer m.Kill()

	// First time: runners started
	st.sendEnvChange(uuid)
	runners := s.seeRunnersStart(c, 1)
	c.Assert(runners[0].killed, jc.IsFalse)

	destroyEnvironment(c, otherSt)
	st.sendEnvChange(otherSt.ModelUUID())
	dyingRunner := s.seeRunnersStart(c, 1)[0]
	c.Assert(dyingRunner.killed, jc.IsFalse)

	// Now we start failing
	st.shouldFail = true
	st.sendEnvChange(uuid)

	// This should kill the manager
	err := waitOrFatal(c, m.Wait)
	c.Assert(err, gc.ErrorMatches, "error loading model .*: unable to GetModel")

	// And that should kill all the runners
	c.Assert(runners[0].killed, jc.IsTrue)
	c.Assert(dyingRunner.killed, jc.IsTrue)
}

func (s *suite) TestWorkerErrorIsPropagatedWhenKilled(c *gc.C) {
	st := newStateWithFakeWatcher(s.State)
	started := make(chan struct{}, 1)
	m := modelworkermanager.NewModelWorkerManager(st, func(modelworkermanager.InitialState, *state.State) (worker.Worker, error) {
		c.Logf("starting worker")
		started <- struct{}{}
		return &errorWhenKilledWorker{
			err: &cmdutil.FatalError{"an error"},
		}, nil
	}, s.dyingEnvWorker, time.Millisecond)
	st.sendEnvChange(st.ModelUUID())
	s.State.StartSync()
	<-started
	m.Kill()
	err := m.Wait()
	c.Assert(err, gc.ErrorMatches, "an error")
}

type errorWhenKilledWorker struct {
	tomb tomb.Tomb
	err  error
}

var logger = loggo.GetLogger("juju.worker.modelworkermanager")

func (w *errorWhenKilledWorker) Kill() {
	w.tomb.Kill(w.err)
	logger.Errorf("errorWhenKilledWorker dying with error %v", w.err)
	w.tomb.Done()
}

func (w *errorWhenKilledWorker) Wait() error {
	err := w.tomb.Wait()
	logger.Errorf("errorWhenKilledWorker wait -> error %v", err)
	return err
}

func (s *suite) TestNothingHappensWhenEnvIsSeenAgain(c *gc.C) {
	// This could happen if there's a change to an environment doc but
	// it's otherwise still alive (unlikely but possible).
	st := newStateWithFakeWatcher(s.State)
	uuid := st.ModelUUID()
	otherSt := s.MakeModel(c)
	otherUUID := otherSt.ModelUUID()

	m := modelworkermanager.NewModelWorkerManager(st, s.startEnvWorker, s.dyingEnvWorker, time.Millisecond)
	defer m.Kill()

	// First time: runners started
	st.sendEnvChange(uuid)
	st.sendEnvChange(otherUUID)
	s.seeRunnersStart(c, 2)

	// Second time: no runners started
	st.sendEnvChange(uuid)
	s.checkNoRunnersStart(c)

	destroyEnvironment(c, otherSt)
	st.sendEnvChange(otherUUID)
	s.seeRunnersStart(c, 1)

	st.sendEnvChange(otherUUID)
	s.checkNoRunnersStart(c)
}

func (s *suite) TestNothingHappensWhenUnknownEnvReported(c *gc.C) {
	// This could perhaps happen when an environment is dying just as
	// the EnvWorkerManager is coming up (unlikely but possible).
	st := newStateWithFakeWatcher(s.State)

	m := modelworkermanager.NewModelWorkerManager(st, s.startEnvWorker, s.dyingEnvWorker, time.Millisecond)
	defer m.Kill()

	st.sendEnvChange("unknown-model-uuid")
	s.checkNoRunnersStart(c)

	// Existing environment still works.
	st.sendEnvChange(st.ModelUUID())
	s.seeRunnersStart(c, 1)
}

func (s *suite) TestFatalErrorKillsEnvWorkerManager(c *gc.C) {
	m := modelworkermanager.NewModelWorkerManager(s.State, s.startEnvWorker, s.dyingEnvWorker, time.Millisecond)
	runner := s.seeRunnersStart(c, 1)[0]

	runner.tomb.Kill(worker.ErrTerminateAgent)
	runner.tomb.Done()

	err := waitOrFatal(c, m.Wait)
	c.Assert(errors.Cause(err), gc.Equals, worker.ErrTerminateAgent)
}

func (s *suite) TestNonFatalErrorCausesRunnerRestart(c *gc.C) {
	m := modelworkermanager.NewModelWorkerManager(s.State, s.startEnvWorker, s.dyingEnvWorker, time.Millisecond)
	defer m.Kill()
	runner0 := s.seeRunnersStart(c, 1)[0]

	runner0.tomb.Kill(errors.New("trivial"))
	runner0.tomb.Done()

	s.seeRunnersStart(c, 1)
}

func (s *suite) TestStateIsClosedIfStartEnvWorkersFails(c *gc.C) {
	// If State is not closed when startEnvWorker errors, MgoSuite's
	// dirty socket detection will pick up the leaked socket and
	// panic.
	s.startErr = worker.ErrTerminateAgent // This will make envWorkerManager exit.
	m := modelworkermanager.NewModelWorkerManager(s.State, s.startEnvWorker, s.dyingEnvWorker, time.Millisecond)
	waitOrFatal(c, m.Wait)
}

func (s *suite) TestdyingEnvWorkerId(c *gc.C) {
	c.Assert(modelworkermanager.DyingModelWorkerId("uuid"), gc.Equals, "dying:uuid")
}

func (s *suite) seeRunnersStart(c *gc.C, expectedCount int) []*fakeRunner {
	if expectedCount < 1 {
		c.Fatal("expectedCount must be >= 1")
	}
	s.State.StartSync()
	runners := make([]*fakeRunner, 0, expectedCount)
	for {
		select {
		case r := <-s.runnerC:
			c.Assert(r.ssModelUUID, gc.Equals, s.State.ModelUUID())

			runners = append(runners, r)
			if len(runners) == expectedCount {
				s.checkNoRunnersStart(c) // Check no more runners start
				return runners
			}
		case <-time.After(testing.LongWait):
			c.Fatal("timed out waiting for runners to be started")
		}
	}
}

func (s *suite) checkNoRunnersStart(c *gc.C) {
	s.State.StartSync()
	for {
		select {
		case <-s.runnerC:
			c.Fatal("saw runner creation when expecting none")
		case <-time.After(testing.ShortWait):
			return
		}
	}
}

// startEnvWorker is passed to NewModelWorkerManager in these tests. It
// creates fake Runner instances when envWorkerManager starts workers
// for an alive environment.
func (s *suite) startEnvWorker(ssSt modelworkermanager.InitialState, st *state.State) (worker.Worker, error) {
	if s.startErr != nil {
		return nil, s.startErr
	}
	runner := &fakeRunner{
		ssModelUUID: ssSt.ModelUUID(),
		modelUUID:   st.ModelUUID(),
	}
	s.runnerC <- runner
	return runner, nil
}

// dyingEnvWorker is passed to NewModelWorkerManager in these tests. It
// creates a fake Runner instance when envWorkerManager starts workers for a
// dying or dead environment.
func (s *suite) dyingEnvWorker(ssSt modelworkermanager.InitialState, st *state.State) (worker.Worker, error) {
	if s.startErr != nil {
		return nil, s.startErr
	}
	runner := &fakeRunner{
		ssModelUUID: ssSt.ModelUUID(),
		modelUUID:   st.ModelUUID(),
	}
	s.runnerC <- runner
	return runner, nil
}

func waitOrFatal(c *gc.C, wait func() error) error {
	errC := make(chan error)
	go func() {
		errC <- wait()
	}()

	select {
	case err := <-errC:
		return err
	case <-time.After(testing.LongWait):
		c.Fatal("waited too long")
	}
	return nil
}

// fakeRunner minimally implements the worker.Worker interface. It
// doesn't actually run anything, recording some execution details for
// testing.
type fakeRunner struct {
	tomb        tomb.Tomb
	ssModelUUID string
	modelUUID   string
	killed      bool
}

func (r *fakeRunner) Kill() {
	r.killed = true
	r.tomb.Done()
}

func (r *fakeRunner) Wait() error {
	return r.tomb.Wait()
}

func newStateWithFakeWatcher(realSt *state.State) *stateWithFakeWatcher {
	return &stateWithFakeWatcher{
		State: realSt,
		envWatcher: &fakeEnvWatcher{
			changes: make(chan []string),
		},
	}
}

// stateWithFakeWatcher wraps a *state.State, overriding the
// WatchModels method to allow control over the reported
// environment lifecycle events for testing.
//
// Use sendEnvChange to cause an environment event to be emitted by
// the watcher returned by WatchModels.
type stateWithFakeWatcher struct {
	*state.State
	envWatcher *fakeEnvWatcher
}

func (s *stateWithFakeWatcher) WatchModels() state.StringsWatcher {
	return s.envWatcher
}

func (s *stateWithFakeWatcher) sendEnvChange(uuids ...string) {
	s.envWatcher.changes <- uuids
}

type fakeEnvWatcher struct {
	state.StringsWatcher
	changes chan []string
}

func (w *fakeEnvWatcher) Stop() error {
	return nil
}

func (w *fakeEnvWatcher) Changes() <-chan []string {
	return w.changes
}
