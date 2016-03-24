// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package migrationflag_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/migrationflag"
	"github.com/juju/juju/worker/workertest"
)

func newMockFacade(stub *testing.Stub, phases ...migration.Phase) *mockFacade {
	return &mockFacade{
		stub:   stub,
		phases: phases,
	}
}

type mockFacade struct {
	stub   *testing.Stub
	phases []migration.Phase
}

// Phase is part of the migrationflag.Facade interface.
func (mock *mockFacade) Phase(uuid string) (migration.Phase, error) {
	mock.stub.AddCall("Phase", uuid)
	if err := mock.stub.NextErr(); err != nil {
		return 0, err
	}
	return mock.nextPhase(), nil
}

func (mock *mockFacade) nextPhase() migration.Phase {
	phase := mock.phases[0]
	mock.phases = mock.phases[1:]
	return phase
}

// Watch is part of the migrationflag.Facade interface.
func (mock *mockFacade) Watch(uuid string) (watcher.NotifyWatcher, error) {
	mock.stub.AddCall("Watch", uuid)
	if err := mock.stub.NextErr(); err != nil {
		return nil, err
	}
	return newMockWatcher(), nil
}

func newMockWatcher() *mockWatcher {
	const count = 3
	changes := make(chan struct{}, count)
	for i := 0; i < count; i++ {
		changes <- struct{}{}
	}
	return &mockWatcher{
		Worker:  workertest.NewErrorWorker(nil),
		changes: changes,
	}
}

type mockWatcher struct {
	worker.Worker
	changes chan struct{}
}

// Changes is part of the watcher.NotifyWatcher interface.
func (mock *mockWatcher) Changes() watcher.NotifyChannel {
	return mock.changes
}

func checkCalls(c *gc.C, stub *testing.Stub, names ...string) {
	stub.CheckCallNames(c, names...)
	for _, call := range stub.Calls() {
		c.Check(call.Args, jc.DeepEquals, []interface{}{validUUID})
	}
}

var validUUID = "01234567-89ab-cdef-0123-456789abcdef"

func panicCheck(migration.Phase) bool  { panic("unexpected") }
func neverCheck(migration.Phase) bool  { return false }
func isQuiesce(p migration.Phase) bool { return p == migration.QUIESCE }

func validConfig() migrationflag.Config {
	return migrationflag.Config{
		Facade: struct{ migrationflag.Facade }{},
		Model:  validUUID,
		Check:  panicCheck,
	}
}

func checkNotValid(c *gc.C, config migrationflag.Config, expect string) {
	check := func(err error) {
		c.Check(err, gc.ErrorMatches, expect)
		c.Check(err, jc.Satisfies, errors.IsNotValid)
	}

	err := config.Validate()
	check(err)

	worker, err := migrationflag.New(config)
	c.Check(worker, gc.IsNil)
	check(err)
}
