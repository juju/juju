// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxc_test

import (
	"fmt"

	"launchpad.net/golxc"
)

type mockFactory struct {
	instances map[string]golxc.Container
}

func MockFactory() golxc.ContainerFactory {
	return &mockFactory{make(map[string]golxc.Container)}
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
	mock.state = golxc.StateStopped
	mock.factory.instances[mock.name] = mock
	return nil
}

// Start runs the container as a daemon.
func (mock *mockContainer) Start(configFile, consoleFile string) error {
	mock.state = golxc.StateRunning
	return nil
}

// Stop terminates the running container.
func (mock *mockContainer) Stop() error {
	mock.state = golxc.StateStopped
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

func (mock *mockFactory) New(name string) golxc.Container {
	container := &mockContainer{
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
