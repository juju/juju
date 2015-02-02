// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package envworkermanager_test

import (
	stdtesting "testing"
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"launchpad.net/tomb"

	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/envworkermanager"
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

func (s *suite) makeEnvironment(c *gc.C) *state.State {
	st := s.factory.MakeEnvironment(c, nil)
	s.AddCleanup(func(*gc.C) { st.Close() })
	return st
}

func (s *suite) TestStartsWorkersForPreExistingEnvs(c *gc.C) {
	moreState := s.makeEnvironment(c)

	var seenEnvs []string
	m := envworkermanager.NewEnvWorkerManager(s.State, s.startEnvWorkers)
	defer m.Kill()
	for _, r := range s.seeRunnersStart(c, 2) {
		seenEnvs = append(seenEnvs, r.envUUID)
	}
	c.Assert(seenEnvs, jc.SameContents,
		[]string{s.State.EnvironUUID(), moreState.EnvironUUID()})
}

func (s *suite) TestStartsWorkersForNewEnv(c *gc.C) {
	m := envworkermanager.NewEnvWorkerManager(s.State, s.startEnvWorkers)
	defer m.Kill()
	s.seeRunnersStart(c, 1) // Runner for state server env

	// Create another environment and watch a runner be created for it.
	st2 := s.makeEnvironment(c)
	runner := s.seeRunnersStart(c, 1)[0]
	c.Assert(runner.envUUID, gc.Equals, st2.EnvironUUID())
}

func (s *suite) TestStopsWorkersWhenEnvGoesAway(c *gc.C) {
	m := envworkermanager.NewEnvWorkerManager(s.State, s.startEnvWorkers)
	defer m.Kill()
	runner0 := s.seeRunnersStart(c, 1)[0]

	// Create an environment and grab the runner for it.
	otherState := s.makeEnvironment(c)
	runner1 := s.seeRunnersStart(c, 1)[0]

	// Destroy the new environment.
	env, err := otherState.Environment()
	c.Assert(err, jc.ErrorIsNil)
	err = env.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	// See that the first runner is still running but the runner for
	// the new environment is stopped.
	s.State.StartSync()
	select {
	case <-runner0.tomb.Dying():
		c.Fatal("first runner should not die here")
	case <-runner1.tomb.Dying():
		break
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for runner to die")
	}

	// Make sure the first runner doesn't get stopped.
	s.State.StartSync()
	select {
	case <-runner0.tomb.Dying():
		c.Fatal("first runner should not die here")
	case <-time.After(testing.ShortWait):
		break
	}
}

func (s *suite) TestKillPropogates(c *gc.C) {
	s.makeEnvironment(c)

	m := envworkermanager.NewEnvWorkerManager(s.State, s.startEnvWorkers)
	runners := s.seeRunnersStart(c, 2)
	c.Assert(runners[0].killed, jc.IsFalse)
	c.Assert(runners[1].killed, jc.IsFalse)

	m.Kill()
	err := waitOrPanic(m.Wait)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(runners[0].killed, jc.IsTrue)
	c.Assert(runners[1].killed, jc.IsTrue)
}

func (s *suite) TestNothingHappensWhenEnvIsSeenAgain(c *gc.C) {
	// This could happen if there's a change to an environment doc but
	// it's otherwise still alive (unlikely but possible).
	st := newStateWithFakeWatcher(s.State)
	uuid := st.EnvironUUID()

	m := envworkermanager.NewEnvWorkerManager(st, s.startEnvWorkers)
	defer m.Kill()

	// First time: runners started
	st.sendEnvChange(uuid)
	s.seeRunnersStart(c, 1)

	// Second time: no runners started
	st.sendEnvChange(uuid)
	s.checkNoRunnersStart(c)
}

func (s *suite) TestNothingHappensWhenUnknownEnvReported(c *gc.C) {
	// This could perhaps happen when an environment is dying just as
	// the EnvWorkerManager is coming up (unlikely but possible).
	st := newStateWithFakeWatcher(s.State)

	m := envworkermanager.NewEnvWorkerManager(st, s.startEnvWorkers)
	defer m.Kill()

	st.sendEnvChange("unknown-env-uuid")
	s.checkNoRunnersStart(c)

	// Existing environment still works.
	st.sendEnvChange(st.EnvironUUID())
	s.seeRunnersStart(c, 1)
}

func (s *suite) TestFatalErrorKillsEnvWorkerManager(c *gc.C) {
	m := envworkermanager.NewEnvWorkerManager(s.State, s.startEnvWorkers)
	runner := s.seeRunnersStart(c, 1)[0]

	runner.tomb.Kill(worker.ErrTerminateAgent)
	runner.tomb.Done()

	err := waitOrPanic(m.Wait)
	c.Assert(errors.Cause(err), gc.Equals, worker.ErrTerminateAgent)
}

func (s *suite) TestNonFatalErrorCausesRunnerRestart(c *gc.C) {
	s.PatchValue(&worker.RestartDelay, time.Millisecond)

	m := envworkermanager.NewEnvWorkerManager(s.State, s.startEnvWorkers)
	defer m.Kill()
	runner0 := s.seeRunnersStart(c, 1)[0]

	runner0.tomb.Kill(errors.New("trivial"))
	runner0.tomb.Done()

	s.seeRunnersStart(c, 1)
}

func (s *suite) TestStateIsClosedIfStartEnvWorkersFails(c *gc.C) {
	// If State is not closed when startEnvWorkers errors, MgoSuite's
	// dirty socket detection will pick up the leaked socket and
	// panic.
	s.startErr = worker.ErrTerminateAgent // This will make envWorkerManager exit.
	m := envworkermanager.NewEnvWorkerManager(s.State, s.startEnvWorkers)
	waitOrPanic(m.Wait)
}

func (s *suite) seeRunnersStart(c *gc.C, expectedCount int) []*fakeRunner {
	if expectedCount < 1 {
		panic("expectedCount must be >= 1")
	}
	s.State.StartSync()
	runners := make([]*fakeRunner, 0, expectedCount)
	for {
		select {
		case r := <-s.runnerC:
			c.Assert(r.ssEnvUUID, gc.Equals, s.State.EnvironUUID())

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

// startEnvWorkers is passed to NewEnvWorkerManager in these tests. It
// creates fake Runner instances when envWorkerManager starts workers
// for an environment.
func (s *suite) startEnvWorkers(ssSt envworkermanager.InitialState, st *state.State) (worker.Runner, error) {
	if s.startErr != nil {
		return nil, s.startErr
	}
	runner := &fakeRunner{
		ssEnvUUID: ssSt.EnvironUUID(),
		envUUID:   st.EnvironUUID(),
	}
	s.runnerC <- runner
	return runner, nil
}

func waitOrPanic(wait func() error) error {
	errC := make(chan error)
	go func() {
		errC <- wait()
	}()

	select {
	case err := <-errC:
		return err
	case <-time.After(testing.LongWait):
		panic("waited too long")
	}
}

// fakeRunner minimally implements the worker.Runner interface. It
// doesn't actually run anything, recording some execution details for
// testing.
type fakeRunner struct {
	worker.Runner
	tomb      tomb.Tomb
	ssEnvUUID string
	envUUID   string
	killed    bool
}

func (r *fakeRunner) Kill() {
	r.killed = true
	r.tomb.Done()
}

func (r *fakeRunner) Wait() error {
	e := r.tomb.Wait()
	return e
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
// WatchEnvironments method to allow control over the reported
// environment lifecycle events for testing.
//
// Use sendEnvChange to cause an environment event to be emitted by
// the watcher returned by WatchEnvironments.
type stateWithFakeWatcher struct {
	*state.State
	envWatcher *fakeEnvWatcher
}

func (s *stateWithFakeWatcher) WatchEnvironments() state.StringsWatcher {
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
