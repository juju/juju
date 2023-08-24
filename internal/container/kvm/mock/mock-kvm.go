// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mock

import (
	"fmt"
	"sync"

	"github.com/juju/juju/internal/container/kvm"
)

// This file provides a mock implementation of the kvm interfaces
// ContainerFactory and Container.

type Action int

const (
	// A container has been started.
	Started Action = iota
	// A container has been stopped.
	Stopped
)

func (action Action) String() string {
	switch action {
	case Started:
		return "Started"
	case Stopped:
		return "Stopped"
	}
	return "unknown"
}

type Event struct {
	Action     Action
	InstanceId string
}

type ContainerFactory interface {
	kvm.ContainerFactory

	AddListener(chan<- Event)
	RemoveListener(chan<- Event)
	HasListener(chan<- Event) bool
}

type mockFactory struct {
	instances map[string]kvm.Container
	listeners []chan<- Event
	mu        sync.Mutex
}

func MockFactory() ContainerFactory {
	return &mockFactory{
		instances: make(map[string]kvm.Container),
	}
}

type MockContainer struct {
	StartParams kvm.StartParams

	factory *mockFactory
	name    string
	started bool
}

// Name returns the name of the container.
func (mock *MockContainer) Name() string {
	return mock.name
}

// imageCacheCalls does nothing more than get incremented by calls to the
// mocked EnsureCachedImage method below.
// However, it will cause the race checker to fail if such calls are not made
// in a Goroutine safe manner.
var imageCacheCalls int //nolint:unused

// EnsureCachedImage is the first supply of start-params to the container.
// We set it here for subsequent test assertions.
// Start is called by the manager immediately after, with the same argument.
func (mock *MockContainer) EnsureCachedImage(params kvm.StartParams) error {
	imageCacheCalls++
	mock.StartParams = params
	return nil
}

func (mock *MockContainer) Start(params kvm.StartParams) error {
	if mock.started {
		return fmt.Errorf("container is already running")
	}
	mock.started = true
	mock.factory.notify(Started, mock.name)
	return nil
}

// Stop terminates the running container.
func (mock *MockContainer) Stop() error {
	if !mock.started {
		return fmt.Errorf("container is not running")
	}
	mock.started = false
	mock.factory.notify(Stopped, mock.name)
	return nil
}

func (mock *MockContainer) IsRunning() bool {
	return mock.started
}

// String returns information about the container.
func (mock *MockContainer) String() string {
	return fmt.Sprintf("<MockContainer %q>", mock.name)
}

func (mock *mockFactory) String() string {
	return fmt.Sprintf("<Mock KVM Factory>")
}

func (mock *mockFactory) New(name string) kvm.Container {
	mock.mu.Lock()
	defer mock.mu.Unlock()

	container, ok := mock.instances[name]
	if ok {
		return container
	}
	container = &MockContainer{
		factory: mock,
		name:    name,
	}
	mock.instances[name] = container
	return container
}

func (mock *mockFactory) List() (result []kvm.Container, err error) {
	for _, container := range mock.instances {
		result = append(result, container)
	}
	return
}

func (mock *mockFactory) notify(action Action, instanceId string) {
	event := Event{action, instanceId}
	for _, c := range mock.listeners {
		c <- event
	}
}

func (mock *mockFactory) AddListener(listener chan<- Event) {
	mock.listeners = append(mock.listeners, listener)
}

func (mock *mockFactory) RemoveListener(listener chan<- Event) {
	pos := 0
	for i, c := range mock.listeners {
		if c == listener {
			pos = i
		}
	}
	mock.listeners = append(mock.listeners[:pos], mock.listeners[pos+1:]...)
}

func (mock *mockFactory) HasListener(listener chan<- Event) bool {
	for _, c := range mock.listeners {
		if c == listener {
			return true
		}
	}
	return false
}
