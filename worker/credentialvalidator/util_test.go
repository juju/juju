// Copyright 2018 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package credentialvalidator_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	worker "gopkg.in/juju/worker.v1"

	"github.com/juju/juju/api/base"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker/credentialvalidator"
	dt "github.com/juju/juju/worker/dependency/testing"
	"github.com/juju/juju/worker/workertest"
)

// mockFacade implements credentialvalidator.Facade for use in the tests.
type mockFacade struct {
	*testing.Stub
	credentials []base.StoredCredential
	exists      bool
}

// ModelCredential is part of the credentialvalidator.Facade interface.
func (m *mockFacade) ModelCredential() (base.StoredCredential, bool, error) {
	m.AddCall("ModelCredential")
	if err := m.NextErr(); err != nil {
		return base.StoredCredential{}, false, err
	}
	return m.nextCredential(), m.exists, nil
}

// nextCredential consumes a credential and returns it, or panics.
func (m *mockFacade) nextCredential() base.StoredCredential {
	credential := m.credentials[0]
	m.credentials = m.credentials[1:]
	return credential
}

// WatchCredential is part of the credentialvalidator.Facade interface.
func (mock *mockFacade) WatchCredential(id string) (watcher.NotifyWatcher, error) {
	mock.AddCall("WatchCredential", id)
	if err := mock.NextErr(); err != nil {
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

// credentialTag is the credential tag we're using in the tests.
// needs to fit fmt.Sprintf("%s/%s/%s", cloudName, userName, credentialName)
var credentialTag = names.NewCloudCredentialTag("cloud/user/credential").String()

// panicCheck is a Config.Check value that should not be called.
func panicCheck(base.StoredCredential) bool { panic("unexpected") }

// neverCheck is a Config.Check value that always returns false.
func neverCheck(base.StoredCredential) bool { return false }

// panicFacade is a NewFacade that should not be called.
func panicFacade(base.APICaller) (credentialvalidator.Facade, error) {
	panic("panicFacade")
}

// panicWorker is a NewWorker that should not be called.
func panicWorker(credentialvalidator.Config) (worker.Worker, error) {
	panic("panicWorker")
}

// validConfig returns a minimal config stuffed with dummy objects that
// will explode when used.
func validConfig() credentialvalidator.Config {
	return credentialvalidator.Config{
		Facade: struct{ credentialvalidator.Facade }{},
		Check:  panicCheck,
	}
}

// checkNotValid checks that the supplied credentialvalidator.Config fails to
// Validate, and cannot be used to construct a credentialvalidator.Worker.
func checkNotValid(c *gc.C, config credentialvalidator.Config, expect string) {
	check := func(err error) {
		c.Check(err, gc.ErrorMatches, expect)
		c.Check(err, jc.Satisfies, errors.IsNotValid)
	}

	err := config.Validate()
	check(err)

	worker, err := credentialvalidator.New(config)
	c.Check(worker, gc.IsNil)
	check(err)
}

// validManifoldConfig returns a minimal config stuffed with dummy objects
// that will explode when used.
func validManifoldConfig() credentialvalidator.ManifoldConfig {
	return credentialvalidator.ManifoldConfig{
		APICallerName: "api-caller",
		Check:         panicCheck,
		NewFacade:     panicFacade,
		NewWorker:     panicWorker,
	}
}

// checkManifoldNotValid checks that the supplied ManifoldConfig creates
// a manifold that cannot be started.
func checkManifoldNotValid(c *gc.C, config credentialvalidator.ManifoldConfig, expect string) {
	manifold := credentialvalidator.Manifold(config)
	worker, err := manifold.Start(dt.StubContext(nil, nil))
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, expect)
	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

// stubCaller is a base.APICaller that only implements ModelTag.
type stubCaller struct {
	base.APICaller
}

// ModelTag is part of the base.APICaller interface.
func (*stubCaller) ModelTag() (names.ModelTag, bool) {
	return coretesting.ModelTag, true
}
