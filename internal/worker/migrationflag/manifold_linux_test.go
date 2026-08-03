//go:build dqlite

// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationflag_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"
	dt "github.com/juju/worker/v5/dependency/testing"

	"github.com/juju/juju/agent/engine"
	"github.com/juju/juju/internal/worker/migrationflag"
)

func (*ManifoldSuite) TestModelManifoldInputs(c *tc.C) {
	manifold := migrationflag.ModelManifold(validModelManifoldConfig())
	c.Check(manifold.Inputs, tc.DeepEquals, []string{"domain-services"})
}

func (*ManifoldSuite) TestModelManifoldOutputBadWorker(c *tc.C) {
	manifold := migrationflag.ModelManifold(migrationflag.ModelManifoldConfig{})
	in := &struct{ worker.Worker }{}
	var out engine.Flag
	err := manifold.Output(in, &out)
	c.Check(err, tc.ErrorMatches, "expected in to implement Flag; got a .*")
}

func (*ManifoldSuite) TestModelManifoldFilterErrChanged(c *tc.C) {
	manifold := migrationflag.ModelManifold(migrationflag.ModelManifoldConfig{})
	err := manifold.Filter(migrationflag.ErrChanged)
	c.Check(err, tc.Equals, dependency.ErrBounce)
}

func (*ManifoldSuite) TestModelManifoldStartMissingDomainServicesName(c *tc.C) {
	config := validModelManifoldConfig()
	config.DomainServicesName = ""
	checkModelManifoldNotValid(c, config, "empty DomainServicesName not valid")
}

func (*ManifoldSuite) TestModelManifoldStartMissingModelUUID(c *tc.C) {
	config := validModelManifoldConfig()
	config.ModelUUID = ""
	checkModelManifoldNotValid(c, config, "empty ModelUUID not valid")
}

func (*ManifoldSuite) TestModelManifoldStartMissingCheck(c *tc.C) {
	config := validModelManifoldConfig()
	config.Check = nil
	checkModelManifoldNotValid(c, config, "nil Check not valid")
}

func (*ManifoldSuite) TestModelManifoldStartMissingNewWorker(c *tc.C) {
	config := validModelManifoldConfig()
	config.NewWorker = nil
	checkModelManifoldNotValid(c, config, "nil NewWorker not valid")
}

func (*ManifoldSuite) TestModelManifoldStartMissingDomainServices(c *tc.C) {
	getter := dt.StubGetter(map[string]any{
		"domain-services": dependency.ErrMissing,
	})
	manifold := migrationflag.ModelManifold(validModelManifoldConfig())

	worker, err := manifold.Start(c.Context(), getter)
	c.Check(worker, tc.IsNil)
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
}

func (*ManifoldSuite) TestModelManifoldStartSuccess(c *tc.C) {
	getter := dt.StubGetter(map[string]any{
		"domain-services": &stubDomainServices{},
	})
	expectWorker := &struct{ worker.Worker }{}
	config := validModelManifoldConfig()
	config.NewWorker = func(ctx context.Context, workerConfig migrationflag.Config) (worker.Worker, error) {
		c.Check(workerConfig.Model, tc.Equals, validUUID)
		c.Check(workerConfig.Facade, tc.NotNil)
		c.Check(workerConfig.Check, tc.NotNil)
		return expectWorker, nil
	}
	manifold := migrationflag.ModelManifold(config)

	worker, err := manifold.Start(c.Context(), getter)
	c.Check(err, tc.ErrorIsNil)
	c.Check(worker, tc.Equals, expectWorker)
}
