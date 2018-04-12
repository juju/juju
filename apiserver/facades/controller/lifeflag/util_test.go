// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lifeflag_test

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

// mockAuth implements facade.Authorizer for the tests' convenience.
type mockAuth struct {
	facade.Authorizer
	modelManager bool
}

func (mock mockAuth) AuthController() bool {
	return mock.modelManager
}

// auth is a convenience constructor for a mockAuth.
func auth(modelManager bool) facade.Authorizer {
	return mockAuth{modelManager: modelManager}
}

// mockBackend implements lifeflag.Backend for the tests' convenience.
type mockBackend struct {
	exist bool
	watch bool
}

func (mock *mockBackend) ModelUUID() string {
	return coretesting.ModelTag.Id()
}

func (mock *mockBackend) FindEntity(tag names.Tag) (state.Entity, error) {
	if tag != coretesting.ModelTag {
		panic("should never happen -- bad auth somewhere")
	}
	if !mock.exist {
		return nil, errors.NotFoundf("model")
	}
	return &mockEntity{
		watch: mock.watch,
	}, nil
}

// mockEntity implements state.Entity for the tests' convenience.
type mockEntity struct {
	watch bool
}

func (mock *mockEntity) Tag() names.Tag {
	return coretesting.ModelTag
}

func (mock *mockEntity) Life() state.Life {
	return state.Dying
}

func (mock *mockEntity) Watch() state.NotifyWatcher {
	changes := make(chan struct{}, 1)
	if mock.watch {
		changes <- struct{}{}
	} else {
		close(changes)
	}
	return &mockWatcher{changes: changes}
}

// mockWatcher implements state.NotifyWatcher for the tests' convenience.
type mockWatcher struct {
	state.NotifyWatcher
	changes chan struct{}
}

func (mock *mockWatcher) Changes() <-chan struct{} {
	return mock.changes
}

func (mock *mockWatcher) Err() error {
	return errors.New("blammo")
}

// entities is a convenience constructor for params.Entities.
func entities(tags ...string) params.Entities {
	entities := params.Entities{Entities: make([]params.Entity, len(tags))}
	for i, tag := range tags {
		entities.Entities[i].Tag = tag
	}
	return entities
}

func modelEntity() params.Entities {
	return entities(coretesting.ModelTag.String())
}
