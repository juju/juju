// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"context"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"
	dt "github.com/juju/worker/v5/dependency/testing"

	"github.com/juju/juju/agent/engine"
	credentialservice "github.com/juju/juju/domain/credential/service"
	modelservice "github.com/juju/juju/domain/model/service"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/credentialvalidator"
	"github.com/juju/juju/internal/worker/credentialvalidator/model"
)

type ModelManifoldSuite struct {
	testhelpers.IsolationSuite
}

func TestModelManifoldSuite(t *testing.T) {
	tc.Run(t, &ModelManifoldSuite{})
}

// panicWorker is a NewWorker that should not be called.
func panicWorker(_ context.Context, _ credentialvalidator.Config) (worker.Worker, error) {
	panic("panicWorker")
}

// stubDomainServices is a minimal stub that satisfies services.DomainServices
// for tests that only need Credential() and Model() methods.
type stubDomainServices struct {
	services.ControllerDomainServices
	services.ModelDomainServices
}

func (s *stubDomainServices) Credential() *credentialservice.WatchableService {
	return nil
}

func (s *stubDomainServices) Model() *modelservice.WatchableService {
	return nil
}

// validModelManifoldConfig returns a minimal ModelManifoldConfig stuffed with
// dummy objects that will explode when used.
func validModelManifoldConfig(c *tc.C) model.ModelManifoldConfig {
	return model.ModelManifoldConfig{
		DomainServicesName: "domain-services",
		ModelUUID:          "mock-model-uuid",
		NewWorker:          panicWorker,
		Logger:             loggertesting.WrapCheckLog(c),
	}
}

// checkModelManifoldNotValid checks that the supplied ModelManifoldConfig
// creates a manifold that cannot be started.
func checkModelManifoldNotValid(c *tc.C, config model.ModelManifoldConfig, expect string) {
	err := config.Validate()
	c.Check(err, tc.ErrorMatches, expect)
	c.Check(err, tc.ErrorIs, errors.NotValid)
}

func (*ModelManifoldSuite) TestModelManifoldInputs(c *tc.C) {
	manifold := model.ModelManifold(validModelManifoldConfig(c))
	c.Check(manifold.Inputs, tc.DeepEquals, []string{"domain-services"})
}

func (*ModelManifoldSuite) TestModelManifoldOutputBadWorker(c *tc.C) {
	manifold := model.ModelManifold(model.ModelManifoldConfig{})
	in := &struct{ worker.Worker }{}
	var out engine.Flag
	err := manifold.Output(in, &out)
	c.Check(err, tc.ErrorMatches, "expected in to implement Flag; got a .*")
}

func (*ModelManifoldSuite) TestModelManifoldFilterNil(c *tc.C) {
	manifold := model.ModelManifold(model.ModelManifoldConfig{})
	err := manifold.Filter(nil)
	c.Check(err, tc.ErrorIsNil)
}

func (*ModelManifoldSuite) TestModelManifoldFilterErrChanged(c *tc.C) {
	manifold := model.ModelManifold(model.ModelManifoldConfig{})
	err := manifold.Filter(credentialvalidator.ErrValidityChanged)
	c.Check(err, tc.Equals, dependency.ErrBounce)
}

func (*ModelManifoldSuite) TestModelManifoldFilterErrModelCredentialChanged(c *tc.C) {
	manifold := model.ModelManifold(model.ModelManifoldConfig{})
	err := manifold.Filter(credentialvalidator.ErrModelCredentialChanged)
	c.Check(err, tc.Equals, dependency.ErrBounce)
}

func (*ModelManifoldSuite) TestModelManifoldFilterOther(c *tc.C) {
	manifold := model.ModelManifold(model.ModelManifoldConfig{})
	expect := errors.New("whatever")
	actual := manifold.Filter(expect)
	c.Check(actual, tc.Equals, expect)
}

func (*ModelManifoldSuite) TestModelManifoldStartMissingDomainServicesName(c *tc.C) {
	config := validModelManifoldConfig(c)
	config.DomainServicesName = ""
	checkModelManifoldNotValid(c, config, "empty DomainServicesName not valid")
}

func (*ModelManifoldSuite) TestModelManifoldStartMissingModelUUID(c *tc.C) {
	config := validModelManifoldConfig(c)
	config.ModelUUID = ""
	checkModelManifoldNotValid(c, config, "empty ModelUUID not valid")
}

func (*ModelManifoldSuite) TestModelManifoldStartMissingNewWorker(c *tc.C) {
	config := validModelManifoldConfig(c)
	config.NewWorker = nil
	checkModelManifoldNotValid(c, config, "nil NewWorker not valid")
}

func (*ModelManifoldSuite) TestModelManifoldStartMissingLogger(c *tc.C) {
	config := validModelManifoldConfig(c)
	config.Logger = nil
	checkModelManifoldNotValid(c, config, "nil Logger not valid")
}

func (*ModelManifoldSuite) TestModelManifoldStartMissingDomainServices(c *tc.C) {
	getter := dt.StubGetter(map[string]any{
		"domain-services": dependency.ErrMissing,
	})
	manifold := model.ModelManifold(validModelManifoldConfig(c))

	w, err := manifold.Start(c.Context(), getter)
	c.Check(w, tc.IsNil)
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
}

func (*ModelManifoldSuite) TestModelManifoldStartNewWorkerError(c *tc.C) {
	getter := dt.StubGetter(map[string]any{
		"domain-services": &stubDomainServices{},
	})
	config := validModelManifoldConfig(c)
	config.NewWorker = func(_ context.Context, workerConfig credentialvalidator.Config) (worker.Worker, error) {
		c.Check(workerConfig.Facade, tc.NotNil)
		return nil, errors.New("snerk")
	}
	manifold := model.ModelManifold(config)

	w, err := manifold.Start(c.Context(), getter)
	c.Check(w, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "snerk")
}

func (*ModelManifoldSuite) TestModelManifoldStartSuccess(c *tc.C) {
	getter := dt.StubGetter(map[string]any{
		"domain-services": &stubDomainServices{},
	})
	expectWorker := &struct{ worker.Worker }{}
	config := validModelManifoldConfig(c)
	config.NewWorker = func(context.Context, credentialvalidator.Config) (worker.Worker, error) {
		return expectWorker, nil
	}
	manifold := model.ModelManifold(config)

	w, err := manifold.Start(c.Context(), getter)
	c.Check(err, tc.ErrorIsNil)
	c.Check(w, tc.Equals, expectWorker)
}

func (*ModelManifoldSuite) TestModelManifoldStartDoesNotRequestAPICaller(c *tc.C) {
	getter := dt.StubGetter(map[string]any{
		"domain-services": &stubDomainServices{},
	})
	config := validModelManifoldConfig(c)
	config.NewWorker = func(_ context.Context, workerConfig credentialvalidator.Config) (worker.Worker, error) {
		c.Check(workerConfig.Facade, tc.NotNil)
		return &struct{ worker.Worker }{}, nil
	}
	manifold := model.ModelManifold(config)

	_, err := manifold.Start(c.Context(), getter)
	c.Check(err, tc.ErrorIsNil)
}
