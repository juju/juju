// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasbroker_test

import (
	"sync"

	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"

	jujutesting "github.com/juju/juju/testing"
)

type fixture struct {
	watcherErr   error
	observerErrs []error
	cloud        environs.CloudSpec
	config       map[string]interface{}
}

func (fix *fixture) Run(c *gc.C, test func(*runContext)) {
	context := &runContext{
		cloud:  fix.cloud,
		config: fix.config,
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

func (context *runContext) ModelConfig() (*config.Config, error) {
	context.mu.Lock()
	defer context.mu.Unlock()
	context.stub.AddCall("Model")
	if err := context.stub.NextErr(); err != nil {
		return nil, err
	}
	return config.New(config.UseDefaults, context.config)
}

func (context *runContext) ControllerConfig() (controller.Config, error) {
	context.mu.Lock()
	defer context.mu.Unlock()
	context.stub.AddCall("ControllerConfig")
	if err := context.stub.NextErr(); err != nil {
		return nil, err
	}
	return jujutesting.FakeControllerConfig(), nil
}

func (context *runContext) CheckCallNames(c *gc.C, names ...string) {
	context.mu.Lock()
	defer context.mu.Unlock()
	context.stub.CheckCallNames(c, names...)
}

type mockBroker struct {
	caas.Broker
	testing.Stub
	spec      environs.CloudSpec
	namespace string
	mu        sync.Mutex
}

func newMockBroker(args environs.OpenParams) (caas.Broker, error) {
	return &mockBroker{spec: args.Cloud, namespace: args.Config.Name()}, nil
}
