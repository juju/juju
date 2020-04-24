// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator_test

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/credentialvalidator"
)

// mockFacade implements credentialvalidator.Facade for use in the tests.
type mockFacade struct {
	*testing.Stub
	credential *base.StoredCredential
	exists     bool

	watcher      *watchertest.MockNotifyWatcher
	modelWatcher *watchertest.MockNotifyWatcher
}

func (m *mockFacade) setupModelHasNoCredential() {
	m.credential.CloudCredential = ""
	m.exists = false
	m.watcher = nil
}

// ModelCredential is part of the credentialvalidator.Facade interface.
func (m *mockFacade) ModelCredential() (base.StoredCredential, bool, error) {
	m.AddCall("ModelCredential")
	if err := m.NextErr(); err != nil {
		return base.StoredCredential{}, false, err
	}
	return *m.credential, m.exists, nil
}

// WatchCredential is part of the credentialvalidator.Facade interface.
func (mock *mockFacade) WatchCredential(id string) (watcher.NotifyWatcher, error) {
	mock.AddCall("WatchCredential", id)
	if err := mock.NextErr(); err != nil {
		return nil, err
	}
	return mock.watcher, nil
}

// WatchModelCredential is part of the credentialvalidator.Facade interface.
func (mock *mockFacade) WatchModelCredential() (watcher.NotifyWatcher, error) {
	mock.AddCall("WatchModelCredential")
	if err := mock.NextErr(); err != nil {
		return nil, err
	}
	return mock.modelWatcher, nil
}

// credentialTag is the credential tag we're using in the tests.
// needs to fit fmt.Sprintf("%s/%s/%s", cloudName, userName, credentialName)
var credentialTag = names.NewCloudCredentialTag("cloud/user/credential").String()

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
		Logger: loggo.GetLogger("test"),
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

	worker, err := credentialvalidator.NewWorker(config)
	c.Check(worker, gc.IsNil)
	check(err)
}

// validManifoldConfig returns a minimal config stuffed with dummy objects
// that will explode when used.
func validManifoldConfig() credentialvalidator.ManifoldConfig {
	return credentialvalidator.ManifoldConfig{
		APICallerName: "api-caller",
		NewFacade:     panicFacade,
		NewWorker:     panicWorker,
		Logger:        loggo.GetLogger("test"),
	}
}

// checkManifoldNotValid checks that the supplied ManifoldConfig creates
// a manifold that cannot be started.
func checkManifoldNotValid(c *gc.C, config credentialvalidator.ManifoldConfig, expect string) {
	err := config.Validate()
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
