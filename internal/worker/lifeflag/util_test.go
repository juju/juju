// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lifeflag_test

import (
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/workertest"

	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/watcher"
)

func newMockFacade(stub *testing.Stub, lifeResults ...func() life.Value) *mockFacade {
	return &mockFacade{
		stub:        stub,
		lifeResults: lifeResults,
	}
}

type mockFacade struct {
	stub        *testing.Stub
	lifeResults []func() life.Value
}

func (mock *mockFacade) Life(entity names.Tag) (life.Value, error) {
	mock.stub.AddCall("Life", entity)
	if err := mock.stub.NextErr(); err != nil {
		return "", err
	}
	return mock.nextLife(), nil
}

func (mock *mockFacade) nextLife() life.Value {
	result := mock.lifeResults[0]
	mock.lifeResults = mock.lifeResults[1:]
	return result()
}

func (mock *mockFacade) Watch(entity names.Tag) (watcher.NotifyWatcher, error) {
	mock.stub.AddCall("Watch", entity)
	if err := mock.stub.NextErr(); err != nil {
		return nil, err
	}
	const count = 2
	changes := make(chan struct{}, count)
	for i := 0; i < count; i++ {
		changes <- struct{}{}
	}
	return newMockWatcher(changes), nil
}

type mockWatcher struct {
	worker.Worker
	changes chan struct{}
}

func newMockWatcher(changes chan struct{}) *mockWatcher {
	return &mockWatcher{
		Worker:  workertest.NewErrorWorker(nil),
		changes: changes,
	}
}

func (mock *mockWatcher) Changes() watcher.NotifyChannel {
	return mock.changes
}
