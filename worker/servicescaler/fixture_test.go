// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package servicescaler_test

import (
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/servicescaler"
	"github.com/juju/juju/worker/workertest"
)

// fixture is used to test the operation of a servicescaler worker.
type fixture struct {
	testing.Stub
}

func newFixture(c *gc.C, callErrors ...error) *fixture {
	fix := &fixture{}
	fix.SetErrors(callErrors...)
	return fix
}

// Run will create a servicescaler worker; start recording the calls
// it makes; and pass it to the supplied test func, which will be invoked
// on a new goroutine. If Run returns, it is safe to inspect the recorded
// calls via the embedded testing.Stub.
func (fix *fixture) Run(c *gc.C, test func(worker.Worker)) {
	stubFacade := newFacade(&fix.Stub)
	scaler, err := servicescaler.New(servicescaler.Config{
		Facade: stubFacade,
	})
	c.Assert(err, jc.ErrorIsNil)

	done := make(chan struct{})
	go func() {
		defer close(done)
		defer worker.Stop(scaler)
		test(scaler)
	}()
	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("test func timed out")
	}
}

// stubFacade implements servicescaler.Facade and records calls to its
// interface methods.
type stubFacade struct {
	stub    *testing.Stub
	watcher *stubWatcher
}

func newFacade(stub *testing.Stub) *stubFacade {
	return &stubFacade{
		stub:    stub,
		watcher: newStubWatcher(),
	}
}

// Watch is part of the servicescaler.Facade interface.
func (facade *stubFacade) Watch() (watcher.StringsWatcher, error) {
	facade.stub.AddCall("Watch")
	err := facade.stub.NextErr()
	if err != nil {
		return nil, err
	}
	return facade.watcher, nil
}

// Rescale is part of the servicescaler.Facade interface.
func (facade *stubFacade) Rescale(serviceNames []string) error {
	facade.stub.AddCall("Rescale", serviceNames)
	return facade.stub.NextErr()
}

// stubWatcher implements watcher.StringsWatcher and supplied canned
// data over the Changes() channel.
type stubWatcher struct {
	worker.Worker
	changes chan []string
}

func newStubWatcher() *stubWatcher {
	changes := make(chan []string, 3)
	changes <- []string{"expected", "first"}
	changes <- []string{"expected", "second"}
	changes <- []string{"unexpected?"}
	return &stubWatcher{
		Worker:  workertest.NewErrorWorker(nil),
		changes: changes,
	}
}

// Changes is part of the watcher.StringsWatcher interface.
func (stubWatcher *stubWatcher) Changes() watcher.StringsChannel {
	return stubWatcher.changes
}
