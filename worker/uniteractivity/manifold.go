// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package uniteractivity provides the manifold that maintains information
// about whether the uniter has started or not.
package uniteractivity

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

type readFunc func(file string) (bool, error)
type writeFunc func(file string, value bool) error

type uniterState struct {
	Started bool `yaml:started`
}

// ManifoldConfig specifies the names a uniteractivity manifold should use to
// address its dependencies.
type ManifoldConfig util.AgentManifoldConfig

// Manifold returns a dependency.Manifold that keeps track of whether the uniter
// is started.
func Manifold(config ManifoldConfig) dependency.Manifold {
	manifold := util.AgentManifold(
		util.AgentManifoldConfig(config),
		newWorker(readStateFile, writeStateFile))
	manifold.Output = outputFunc
	return manifold
}

// UniterActivityState interface defines the functions for setting and getting
// the state of the uniter (a boolean signifying whether it is started or not).
type UniterActivityState interface {
	SetStarted(bool) error
	Started() bool
}

// newWorker returns a function that  creates a degenerate worker that provides access to the state of the uniter
// with the specified functions for reading and writing the state file.
func newWorker(read readFunc, write writeFunc) func(a agent.Agent) (worker.Worker, error) {
	return func(a agent.Agent) (worker.Worker, error) {
		id := a.CurrentConfig().Tag().String()
		persistenceFile := filepath.Join(
			a.CurrentConfig().UniterStateDir(),
			id)
		started, err := read(persistenceFile)
		if err != nil {
			return nil, errors.Trace(err)
		}
		w := &uniterStateWorker{file: persistenceFile, started: started, writeStateFile: write}
		go func() {
			defer w.tomb.Done()
			<-w.tomb.Dying()
		}()
		return w, nil
	}
}

// outputFunc extracts a *fslock.Lock from a *machineLockWorker.
func outputFunc(in worker.Worker, out interface{}) error {
	inWorker, _ := in.(*uniterStateWorker)
	outPointer, _ := out.(*UniterActivityState)
	if inWorker == nil || outPointer == nil {
		return errors.Errorf("expected %T->%T; got %T->%T", inWorker, outPointer, in, out)
	}
	*outPointer = inWorker
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
	return s.Started, nil
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
	sync.RWMutex

	file           string
	started        bool
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

// Started is part of the UniterState interface.
func (u *uniterStateWorker) Started() bool {
	u.RLock()
	defer u.RUnlock()
	return u.started
}

// SetStarted is part of the UniterState interface. If writing
// to the persistence file fails, the worker will shut down.
func (u *uniterStateWorker) SetStarted(started bool) (retErr error) {
	u.Lock()
	defer u.Unlock()
	// Set the in-memory variable if persisting succeeds,
	// shut down worker otherwise.
	defer func() {
		if retErr == nil {
			u.started = started
		} else {
			u.tomb.Kill(retErr)
		}
	}()
	err := u.writeStateFile(u.file, started)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}
