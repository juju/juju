// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mock

import (
	"fmt"

	"launchpad.net/golxc"
)

// This file provides a mock implementation of the golxc interfaces
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
	golxc.ContainerFactory

	AddListener(chan<- Event)
	RemoveListener(chan<- Event)
}

type mockFactory struct {
	instances map[string]golxc.Container
	listeners []chan<- Event
}

func MockFactory() ContainerFactory {
	return &mockFactory{
		instances: make(map[string]golxc.Container),
	}
}

type mockContainer struct {
	factory  *mockFactory
	name     string
	state    golxc.State
	logFile  string
	logLevel golxc.LogLevel
}

// Name returns the name of the container.
func (mock *mockContainer) Name() string {
	return mock.name
}

// Create creates a new container based on the given template.
func (mock *mockContainer) Create(configFile, template string, templateArgs ...string) error {
	if mock.state != golxc.StateUnknown {
		return fmt.Errorf("container is already created")
	}
	mock.state = golxc.StateStopped
	mock.factory.instances[mock.name] = mock
	return nil
}

// Start runs the container as a daemon.
func (mock *mockContainer) Start(configFile, consoleFile string) error {
	if mock.state == golxc.StateUnknown {
		return fmt.Errorf("container has not been created")
	} else if mock.state == golxc.StateRunning {
		return fmt.Errorf("container is already running")
	}
	mock.state = golxc.StateRunning
	mock.factory.notify(Started, mock.name)
	return nil
}

// Stop terminates the running container.
func (mock *mockContainer) Stop() error {
	if mock.state == golxc.StateUnknown {
		return fmt.Errorf("container has not been created")
	} else if mock.state == golxc.StateStopped {
		return fmt.Errorf("container is already stopped")
	}
	mock.state = golxc.StateStopped
	mock.factory.notify(Stopped, mock.name)
	return nil
}

// Clone creates a copy of the container, giving the copy the specified name.
func (mock *mockContainer) Clone(name string) (golxc.Container, error) {
	container := &mockContainer{
		factory:  mock.factory,
		name:     name,
		state:    golxc.StateStopped,
		logLevel: golxc.LogWarning,
	}
	mock.factory.instances[name] = container
	return container, nil
}

// Freeze freezes all the container's processes.
func (mock *mockContainer) Freeze() error {
	return nil
}

// Unfreeze thaws all frozen container's processes.
func (mock *mockContainer) Unfreeze() error {
	return nil
}

// Destroy stops and removes the container.
func (mock *mockContainer) Destroy() error {
	// golxc destroy will stop the machine if it is running.
	if mock.state == golxc.StateRunning {
		mock.Stop()
	}
	if mock.state == golxc.StateUnknown {
		return fmt.Errorf("container has not been created")
	}
	mock.state = golxc.StateUnknown
	delete(mock.factory.instances, mock.name)
	return nil
}

// Wait waits for one of the specified container states.
func (mock *mockContainer) Wait(states ...golxc.State) error {
	return nil
}

// Info returns the status and the process id of the container.
func (mock *mockContainer) Info() (golxc.State, int, error) {
	pid := -1
	if mock.state == golxc.StateRunning {
		pid = 42
	}
	return mock.state, pid, nil
}

// IsConstructed checks if the container image exists.
func (mock *mockContainer) IsConstructed() bool {
	return mock.state != golxc.StateUnknown
}

// IsRunning checks if the state of the container is 'RUNNING'.
func (mock *mockContainer) IsRunning() bool {
	return mock.state == golxc.StateRunning
}

// String returns information about the container, like the name, state,
// and process id.
func (mock *mockContainer) String() string {
	_, pid, _ := mock.Info()
	return fmt.Sprintf("<MockContainer %q, state: %s, pid %d>", mock.name, string(mock.state), pid)
}

// LogFile returns the current filename used for the LogFile.
func (mock *mockContainer) LogFile() string {
	return mock.logFile
}

// LogLevel returns the current logging level (only used if the
// LogFile is not "").
func (mock *mockContainer) LogLevel() golxc.LogLevel {
	return mock.logLevel
}

// SetLogFile sets both the LogFile and LogLevel.
func (mock *mockContainer) SetLogFile(filename string, level golxc.LogLevel) {
	mock.logFile = filename
	mock.logLevel = level
}

func (mock *mockFactory) String() string {
	return fmt.Sprintf("mock lxc factory")
}

func (mock *mockFactory) New(name string) golxc.Container {
	container, ok := mock.instances[name]
	if ok {
		return container
	}
	container = &mockContainer{
		factory:  mock,
		name:     name,
		state:    golxc.StateUnknown,
		logLevel: golxc.LogWarning,
	}
	return container
}

func (mock *mockFactory) List() (result []golxc.Container, err error) {
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
