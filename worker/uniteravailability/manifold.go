// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package uniteravailability provides the manifold that maintains information
// about whether the uniter is available or not.
package uniteravailability

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/utils"
	goyaml "gopkg.in/yaml.v1"
	"launchpad.net/tomb"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/util"
)

// UniterAvailabilitySetter interface defines the function for setting
// the state of the uniter (a boolean signifying whether it is available or not).
type UniterAvailabilitySetter interface {
	// Lock locks the uniter availability manifold worker and sets it to unavailable.
	Lock() error
	// Unlock unlocks the uniter availability manifold worker and sets it to available.
	Unlock() error
}

// UniterAvailabilityGetter interface defines the function getting
// the state of the uniter (a boolean signifying whether it is available or not).
// When the state of the uniter is retrieved, it is locked, preventing it from
// being changed (though multiple getters may lock it simultaneously).
// Require also returns a function used to unlock it.
type UniterAvailabilityGetter interface {
	Require() (bool, func())
}

type readFunc func(file string) (bool, error)
type writeFunc func(file string, value bool) error

type uniterState struct {
	Available bool `yaml:available`
}

// ManifoldConfig specifies the names a uniteravailability manifold should use to
// address its dependencies.
type ManifoldConfig util.AgentManifoldConfig

// Manifold returns a dependency.Manifold that keeps track of whether the uniter
// is available.
func Manifold(config ManifoldConfig) dependency.Manifold {
	manifold := util.AgentManifold(
		util.AgentManifoldConfig(config),
		newWorker(readStateFile, writeStateFile))
	manifold.Output = outputFunc
	return manifold
}

// newWorker returns a function that  creates a degenerate worker that provides access to the state of the uniter
// with the specified functions for reading and writing the state file.
func newWorker(read readFunc, write writeFunc) func(a agent.Agent) (worker.Worker, error) {
	return func(a agent.Agent) (worker.Worker, error) {
		id := a.CurrentConfig().Tag().String()
		persistenceFile := filepath.Join(
			a.CurrentConfig().UniterStateDir(),
			id)
		avail, err := read(persistenceFile)
		if err != nil {
			return nil, errors.Trace(err)
		}
		w := &uniterStateWorker{file: persistenceFile, available: avail, writeStateFile: write}
		go func() {
			defer w.tomb.Done()
			<-w.tomb.Dying()
		}()
		return w, nil
	}
}

// outputFunc extracts an UniterAvailabilityGetter or a UniterAvailabilitySetter from the worker.
func outputFunc(in worker.Worker, out interface{}) error {
	inWorker, _ := in.(*uniterStateWorker)
	if inWorker == nil {
		return errors.Errorf("expected %T; got %T", inWorker, in)
	}
	switch outPointer := out.(type) {
	case *UniterAvailabilitySetter:
		*outPointer = inWorker
	case *UniterAvailabilityGetter:
		*outPointer = inWorker
	default:
		return errors.Errorf("out should be a pointer to a UniterAvailabilityGetter or a UniterAvailabilitySetter; is %T", out)
	}
	return nil
}

// readStateFile is used to read the file the uniter's state is persisted in.
func readStateFile(file string) (bool, error) {
	raw, err := ioutil.ReadFile(file)
	if err != nil && os.IsNotExist(err) {
		// If the state file does not exist, assume the state was never
		// written, and therefore the uniter has not been started.
		return false, nil
	} else if err != nil {
		return false, errors.Annotatef(err, "could not open uniter state file %q", file)
	}
	var s uniterState
	err = goyaml.Unmarshal(raw, &s)
	if err != nil {
		return false, errors.Annotatef(err, "could not decode uniter state file %q", file)
	}
	return s.Available, nil
}

// writeStateFile is used to save the file the uniter's state is persisted in.
func writeStateFile(file string, value bool) error {
	b, err := goyaml.Marshal(uniterState{value})
	if err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(utils.AtomicWriteFile(file, b, 0755))
}

// uniterStateWorker is a degenerate worker that keeps track o
// to its lock.
type uniterStateWorker struct {
	tomb tomb.Tomb

	mtx sync.RWMutex

	file           string
	available      bool
	writeStateFile writeFunc
}

// Kill is part of the worker.Worker interface.
func (w *uniterStateWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *uniterStateWorker) Wait() error {
	return w.tomb.Wait()
}

// Require is part of the UniterStateGetter interface.
func (u *uniterStateWorker) Require() (bool, func()) {
	u.mtx.RLock()
	return u.available, u.mtx.RUnlock
}

// Lock is part of the UniterStateSetter interface.
func (u *uniterStateWorker) Lock() error {
	u.mtx.Lock()
	return u.setAvailable(false)
}

// Unlock is part of the UniterStateSetter interface.
func (u *uniterStateWorker) Unlock() error {
	err := u.setAvailable(true)
	u.mtx.Unlock()
	return err
}

// SetAvailable is part of the UniterState interface. If writing
// to the persistence file fails, the worker will shut down.
func (u *uniterStateWorker) setAvailable(available bool) (retErr error) {
	// Set the in-memory variable if persisting succeeds,
	// shut down worker otherwise.
	defer func() {
		if retErr == nil {
			u.available = available
		} else {
			u.tomb.Kill(retErr)
		}
	}()
	err := u.writeStateFile(u.file, available)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}
