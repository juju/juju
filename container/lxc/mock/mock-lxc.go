// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mock

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"

	"github.com/juju/loggo"
	"launchpad.net/golxc"

	"launchpad.net/juju-core/container"
	"launchpad.net/juju-core/utils"
)

// This file provides a mock implementation of the golxc interfaces
// ContainerFactory and Container.

var logger = loggo.GetLogger("juju.container.lxc.mock")

type Action int

const (
	// A container has been started.
	Started Action = iota
	// A container has been stopped.
	Stopped
	// A container has been created.
	Created
	// A container has been destroyed.
	Destroyed
	// A container has been cloned.
	Cloned
)

func (action Action) String() string {
	switch action {
	case Started:
		return "Started"
	case Stopped:
		return "Stopped"
	case Created:
		return "Created"
	case Destroyed:
		return "Destroyed"
	case Cloned:
		return "Cloned"
	}
	return "unknown"
}

type Event struct {
	Action       Action
	InstanceId   string
	Args         []string
	TemplateArgs []string
}

type ContainerFactory interface {
	golxc.ContainerFactory

	AddListener(chan<- Event)
	RemoveListener(chan<- Event)
}

type mockFactory struct {
	containerDir string
	instances    map[string]golxc.Container
	listeners    []chan<- Event
	mutex        sync.Mutex
}

func MockFactory(containerDir string) ContainerFactory {
	return &mockFactory{
		containerDir: containerDir,
		instances:    make(map[string]golxc.Container),
	}
}

type mockContainer struct {
	factory  *mockFactory
	name     string
	state    golxc.State
	logFile  string
	logLevel golxc.LogLevel
}

func (mock *mockContainer) getState() golxc.State {
	mock.factory.mutex.Lock()
	defer mock.factory.mutex.Unlock()
	return mock.state
}

func (mock *mockContainer) setState(newState golxc.State) {
	mock.factory.mutex.Lock()
	defer mock.factory.mutex.Unlock()
	mock.state = newState
	logger.Debugf("container %q state change to %s", mock.name, string(newState))
}

// Name returns the name of the container.
func (mock *mockContainer) Name() string {
	return mock.name
}

func (mock *mockContainer) configFilename() string {
	return filepath.Join(mock.factory.containerDir, mock.name, "config")
}

// Create creates a new container based on the given template.
func (mock *mockContainer) Create(configFile, template string, extraArgs []string, templateArgs []string) error {
	if mock.getState() != golxc.StateUnknown {
		return fmt.Errorf("container is already created")
	}
	mock.factory.instances[mock.name] = mock
	// Create the container directory.
	containerDir := filepath.Join(mock.factory.containerDir, mock.name)
	if err := os.MkdirAll(containerDir, 0755); err != nil {
		return err
	}
	if err := utils.CopyFile(mock.configFilename(), configFile); err != nil {
		return err
	}
	mock.setState(golxc.StateStopped)
	mock.factory.notify(eventArgs(Created, mock.name, extraArgs, templateArgs))
	return nil
}

// Start runs the container as a daemon.
func (mock *mockContainer) Start(configFile, consoleFile string) error {
	state := mock.getState()
	if state == golxc.StateUnknown {
		return fmt.Errorf("container has not been created")
	} else if state == golxc.StateRunning {
		return fmt.Errorf("container is already running")
	}
	ioutil.WriteFile(
		filepath.Join(container.ContainerDir, mock.name, "console.log"),
		[]byte("fake console.log"), 0644)
	mock.setState(golxc.StateRunning)
	mock.factory.notify(event(Started, mock.name))
	return nil
}

// Stop terminates the running container.
func (mock *mockContainer) Stop() error {
	state := mock.getState()
	if state == golxc.StateUnknown {
		return fmt.Errorf("container has not been created")
	} else if state == golxc.StateStopped {
		return fmt.Errorf("container is already stopped")
	}
	mock.setState(golxc.StateStopped)
	mock.factory.notify(event(Stopped, mock.name))
	return nil
}

// Clone creates a copy of the container, giving the copy the specified name.
func (mock *mockContainer) Clone(name string, extraArgs []string, templateArgs []string) (golxc.Container, error) {
	state := mock.getState()
	if state == golxc.StateUnknown {
		return nil, fmt.Errorf("container has not been created")
	} else if state == golxc.StateRunning {
		return nil, fmt.Errorf("container is running, clone not possible")
	}

	container := &mockContainer{
		factory:  mock.factory,
		name:     name,
		state:    golxc.StateStopped,
		logLevel: golxc.LogWarning,
	}
	mock.factory.instances[name] = container

	// Create the container directory.
	containerDir := filepath.Join(mock.factory.containerDir, name)
	if err := os.MkdirAll(containerDir, 0755); err != nil {
		return nil, err
	}
	if err := utils.CopyFile(container.configFilename(), mock.configFilename()); err != nil {
		return nil, err
	}

	mock.factory.notify(eventArgs(Cloned, mock.name, extraArgs, templateArgs))
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
	state := mock.getState()
	// golxc destroy will stop the machine if it is running.
	if state == golxc.StateRunning {
		mock.Stop()
	}
	if state == golxc.StateUnknown {
		return fmt.Errorf("container has not been created")
	}
	delete(mock.factory.instances, mock.name)
	mock.setState(golxc.StateUnknown)
	mock.factory.notify(event(Destroyed, mock.name))
	return nil
}

// Wait waits for one of the specified container states.
func (mock *mockContainer) Wait(states ...golxc.State) error {
	return nil
}

// Info returns the status and the process id of the container.
func (mock *mockContainer) Info() (golxc.State, int, error) {
	pid := -1
	state := mock.getState()
	if state == golxc.StateRunning {
		pid = 42
	}
	return state, pid, nil
}

// IsConstructed checks if the container image exists.
func (mock *mockContainer) IsConstructed() bool {
	return mock.getState() != golxc.StateUnknown
}

// IsRunning checks if the state of the container is 'RUNNING'.
func (mock *mockContainer) IsRunning() bool {
	return mock.getState() == golxc.StateRunning
}

// String returns information about the container, like the name, state,
// and process id.
func (mock *mockContainer) String() string {
	state, pid, _ := mock.Info()
	return fmt.Sprintf("<MockContainer %q, state: %s, pid %d>", mock.name, string(state), pid)
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
	mock.mutex.Lock()
	defer mock.mutex.Unlock()
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
	mock.instances[name] = container
	return container
}

func (mock *mockFactory) List() (result []golxc.Container, err error) {
	for _, container := range mock.instances {
		result = append(result, container)
	}
	return
}

func event(action Action, instanceId string) Event {
	return Event{action, instanceId, nil, nil}
}

func eventArgs(action Action, instanceId string, args []string, template []string) Event {
	return Event{action, instanceId, args, template}
}

func (mock *mockFactory) notify(event Event) {
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
