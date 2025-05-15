// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationflag_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	dt "github.com/juju/worker/v4/dependency/testing"
	"github.com/juju/worker/v4/workertest"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/migrationflag"
)

// newMockFacade returns a mock Facade that will add calls to the
// supplied testing.Stub, and return errors in the sequences it
// specifies; if any Phase call does not return an error, it will
// return a phase consumed from the head of the supplied list (or
// panic if it's empty).
func newMockFacade(stub *testhelpers.Stub, phases ...migration.Phase) *mockFacade {
	return &mockFacade{
		stub:   stub,
		phases: phases,
	}
}

// mockFacade implements migrationflag.Facade for use in the tests.
type mockFacade struct {
	stub   *testhelpers.Stub
	phases []migration.Phase
}

// Phase is part of the migrationflag.Facade interface.
func (mock *mockFacade) Phase(_ context.Context, uuid string) (migration.Phase, error) {
	mock.stub.AddCall("Phase", uuid)
	if err := mock.stub.NextErr(); err != nil {
		return 0, err
	}
	return mock.nextPhase(), nil
}

// nextPhase consumes a phase and returns it, or panics.
func (mock *mockFacade) nextPhase() migration.Phase {
	phase := mock.phases[0]
	mock.phases = mock.phases[1:]
	return phase
}

// Watch is part of the migrationflag.Facade interface.
func (mock *mockFacade) Watch(_ context.Context, uuid string) (watcher.NotifyWatcher, error) {
	mock.stub.AddCall("Watch", uuid)
	if err := mock.stub.NextErr(); err != nil {
		return nil, err
	}
	return newMockWatcher(), nil
}

// newMockWatcher returns a watcher.NotifyWatcher that always
// sends 3 changes and then sits quietly until killed.
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

// mockWatcher implements watcher.NotifyWatcher for use in the tests.
type mockWatcher struct {
	worker.Worker
	changes chan struct{}
}

// Changes is part of the watcher.NotifyWatcher interface.
func (mock *mockWatcher) Changes() watcher.NotifyChannel {
	return mock.changes
}

// checkCalls checks that all the supplied call names were invoked
// in the supplied order, and that every one was passed [validUUID].
func checkCalls(c *tc.C, stub *testhelpers.Stub, names ...string) {
	stub.CheckCallNames(c, names...)
	for _, call := range stub.Calls() {
		c.Check(call.Args, tc.DeepEquals, []interface{}{validUUID})
	}
}

// validUUID is the model UUID we're using in the tests.
var validUUID = "01234567-89ab-cdef-0123-456789abcdef"

// panicCheck is a Config.Check value that should not be called.
func panicCheck(migration.Phase) bool { panic("unexpected") }

// neverCheck is a Config.Check value that always returns false.
func neverCheck(migration.Phase) bool { return false }

// panicFacade is a NewFacade that should not be called.
func panicFacade(base.APICaller) (migrationflag.Facade, error) {
	panic("panicFacade")
}

// panicWorker is a NewWorker that should not be called.
func panicWorker(context.Context, migrationflag.Config) (worker.Worker, error) {
	panic("panicWorker")
}

// isQuiesce is a Config.Check value that returns whether the phase is QUIESCE.
func isQuiesce(p migration.Phase) bool { return p == migration.QUIESCE }

// validConfig returns a minimal config stuffed with dummy objects that
// will explode when used.
func validConfig() migrationflag.Config {
	return migrationflag.Config{
		Facade: struct{ migrationflag.Facade }{},
		Model:  validUUID,
		Check:  panicCheck,
	}
}

// checkNotValid checks that the supplied migrationflag.Config fails to
// Validate, and cannot be used to construct a migrationflag.Worker.
func checkNotValid(c *tc.C, config migrationflag.Config, expect string) {
	check := func(err error) {
		c.Check(err, tc.ErrorMatches, expect)
		c.Check(err, tc.ErrorIs, errors.NotValid)
	}

	err := config.Validate()
	check(err)

	worker, err := migrationflag.New(c.Context(), config)
	c.Check(worker, tc.IsNil)
	check(err)
}

// validManifoldConfig returns a minimal config stuffed with dummy objects
// that will explode when used.
func validManifoldConfig() migrationflag.ManifoldConfig {
	return migrationflag.ManifoldConfig{
		APICallerName: "api-caller",
		Check:         panicCheck,
		NewFacade:     panicFacade,
		NewWorker:     panicWorker,
	}
}

// checkManifoldNotValid checks that the supplied ManifoldConfig creates
// a manifold that cannot be started.
func checkManifoldNotValid(c *tc.C, config migrationflag.ManifoldConfig, expect string) {
	manifold := migrationflag.Manifold(config)
	worker, err := manifold.Start(c.Context(), dt.StubGetter(nil))
	c.Check(worker, tc.IsNil)
	c.Check(err, tc.ErrorMatches, expect)
	c.Check(err, tc.ErrorIs, errors.NotValid)
}

// stubCaller is a base.APICaller that only implements ModelTag.
type stubCaller struct {
	base.APICaller
}

// ModelTag is part of the base.APICaller interface.
func (*stubCaller) ModelTag() (names.ModelTag, bool) {
	return names.NewModelTag(validUUID), true
}
