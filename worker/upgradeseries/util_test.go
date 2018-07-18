// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package upgradeseries_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/api/base"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/upgradeseries"
)

// panicFacade is a NewFacade that should not be called.
func panicFacade(base.APICaller, names.Tag) upgradeseries.Facade {
	panic("panicFacade")
}

// panicWorker is a NewWorker that should not be called.
func panicWorker(upgradeseries.Config) (worker.Worker, error) {
	panic("panicWorker")
}

// validManifoldConfig returns a minimal config stuffed with dummy objects
// that will explode when used.
func validManifoldConfig() upgradeseries.ManifoldConfig {
	return upgradeseries.ManifoldConfig{
		APICallerName: "api-caller",
		NewFacade:     panicFacade,
		NewWorker:     panicWorker,
	}
}

// checkManifoldNotValid checks that the supplied ManifoldConfig creates
// a manifold that cannot be started.
func checkManifoldNotValid(c *gc.C, config upgradeseries.ManifoldConfig, expect string) {
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
