// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/credentialvalidator"
)

// mockFacade implements credentialvalidator.Facade for use in the tests.
type mockFacade struct {
	*testing.Stub
	credential *base.StoredCredential
	exists     bool

	modelWatcher *watchertest.MockNotifyWatcher
}

func (m *mockFacade) setupModelHasNoCredential() {
	m.credential.CloudCredential = ""
	m.exists = false
}

// ModelCredential is part of the credentialvalidator.Facade interface.
func (m *mockFacade) ModelCredential(context.Context) (base.StoredCredential, bool, error) {
	m.AddCall("ModelCredential")
	if err := m.NextErr(); err != nil {
		return base.StoredCredential{}, false, err
	}
	return *m.credential, m.exists, nil
}

// WatchModelCredential is part of the credentialvalidator.Facade interface.
func (mock *mockFacade) WatchModelCredential(context.Context) (watcher.NotifyWatcher, error) {
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
func panicWorker(context.Context, credentialvalidator.Config) (worker.Worker, error) {
	panic("panicWorker")
}

// validConfig returns a minimal config stuffed with dummy objects that
// will explode when used.
func validConfig(c *gc.C) credentialvalidator.Config {
	return credentialvalidator.Config{
		Facade: struct{ credentialvalidator.Facade }{},
		Logger: loggertesting.WrapCheckLog(c),
	}
}

// checkNotValid checks that the supplied credentialvalidator.Config fails to
// Validate, and cannot be used to construct a credentialvalidator.Worker.
func checkNotValid(c *gc.C, config credentialvalidator.Config, expect string) {
	check := func(err error) {
		c.Check(err, gc.ErrorMatches, expect)
		c.Check(err, jc.ErrorIs, errors.NotValid)
	}

	err := config.Validate()
	check(err)

	worker, err := credentialvalidator.NewWorker(context.Background(), config)
	c.Check(worker, gc.IsNil)
	check(err)
}

// validManifoldConfig returns a minimal config stuffed with dummy objects
// that will explode when used.
func validManifoldConfig(c *gc.C) credentialvalidator.ManifoldConfig {
	return credentialvalidator.ManifoldConfig{
		APICallerName: "api-caller",
		NewFacade:     panicFacade,
		NewWorker:     panicWorker,
		Logger:        loggertesting.WrapCheckLog(c),
	}
}

// checkManifoldNotValid checks that the supplied ManifoldConfig creates
// a manifold that cannot be started.
func checkManifoldNotValid(c *gc.C, config credentialvalidator.ManifoldConfig, expect string) {
	err := config.Validate()
	c.Check(err, gc.ErrorMatches, expect)
	c.Check(err, jc.ErrorIs, errors.NotValid)
}

// stubCaller is a base.APICaller that only implements ModelTag.
type stubCaller struct {
	base.APICaller
}

// ModelTag is part of the base.APICaller interface.
func (*stubCaller) ModelTag() (names.ModelTag, bool) {
	return coretesting.ModelTag, true
}
