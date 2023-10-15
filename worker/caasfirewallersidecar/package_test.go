// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewallersidecar

import (
	"testing"

	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"
	gc "gopkg.in/check.v1"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

type (
	ApplicationWorkerCreator = applicationWorkerCreator
)

var (
	NewApplicationWorker = newApplicationWorker
)

func NewWorkerForTest(config Config, f ApplicationWorkerCreator) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	p := newFirewaller(config, f)
	err := catacomb.Invoke(catacomb.Plan{
		Site: &p.catacomb,
		Work: p.loop,
	})
	return p, err
}
