// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasbroker_test

import (
	"sync"

	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/environs"
)

type fixture struct {
	watcherErr   error
	observerErrs []error
	cloud        environs.CloudSpec
}

func (fix *fixture) Run(c *gc.C, test func(*runContext)) {
	context := &runContext{
		cloud: fix.cloud,
	}
	context.stub.SetErrors(fix.observerErrs...)
	test(context)
}

type runContext struct {
	mu     sync.Mutex
	stub   testing.Stub
	cloud  environs.CloudSpec
	config map[string]interface{}
}

func (context *runContext) CloudSpec() (environs.CloudSpec, error) {
	context.mu.Lock()
	defer context.mu.Unlock()
	context.stub.AddCall("CloudSpec")
	if err := context.stub.NextErr(); err != nil {
		return environs.CloudSpec{}, err
	}
	return context.cloud, nil
}

func (context *runContext) CheckCallNames(c *gc.C, names ...string) {
	context.mu.Lock()
	defer context.mu.Unlock()
	context.stub.CheckCallNames(c, names...)
}

type mockBroker struct {
	caas.Broker
	testing.Stub
	spec environs.CloudSpec
	mu   sync.Mutex
}

func newMockBroker(spec environs.CloudSpec) (caas.Broker, error) {
	return &mockBroker{spec: spec}, nil
}
