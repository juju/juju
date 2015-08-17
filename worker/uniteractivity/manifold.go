// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package uniteractivity provides the manifold that maintains information
// about whether the uniter has started or not.
package uniteractivity

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/juju/errors"
	"launchpad.net/tomb"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/util"
)

// readStateFile is used to read the file the uniter's state is persisted in.
// It's a variable to make it easier to stub out in tests.
var readStateFile = readStateFileImpl

// writeStateFile is used to save the file the uniter's state is persisted in.
// It's a variable to make it easier to stub it out in tests.
var writeStateFile = writeStateFileImpl

// ManifoldConfig specifies the names a machinelock manifold should use to
// address its dependencies.
type ManifoldConfig util.AgentManifoldConfig

// Manifold returns a dependency.Manifold that keeps track of whether the uniter
// is started.
func Manifold(config ManifoldConfig) dependency.Manifold {
	manifold := util.AgentManifold(util.AgentManifoldConfig(config), newWorker)
	manifold.Output = outputFunc
	return manifold
}

// UniterState interface defines the functions for setting and getting
// the state of the uniter (a boolean signifying whether it is started or not).
type UniterState interface {
	SetStarted(bool) error
	GetStarted() bool
}

// newWorker creates a degenerate worker that provides access to the state of the uniter.
func newWorker(a agent.Agent) (worker.Worker, error) {
	id := strings.Replace(a.CurrentConfig().Tag().Id(), "/", "-", -1)
	persistenceFile := filepath.Join(
		a.CurrentConfig().UniterStateDir(),
		id)
	started, err := readStateFile(persistenceFile)
	if err != nil {
		return nil, errors.Trace(err)
	}
	w := &uniterStateWorker{file: persistenceFile, started: started}
	go func() {
		defer w.tomb.Done()
		<-w.tomb.Dying()
	}()
	return w, nil
}

// outputFunc extracts a *fslock.Lock from a *machineLockWorker.
func outputFunc(in worker.Worker, out interface{}) error {
	inWorker, _ := in.(*uniterStateWorker)
	outPointer, _ := out.(*UniterState)
	if inWorker == nil || outPointer == nil {
		return errors.Errorf("expected %T->%T; got %T->%T", inWorker, outPointer, in, out)
	}
	*outPointer = inWorker
	return nil
}

func readStateFileImpl(file string) (bool, error) {
	f, err := os.Open(file)
	if err != nil && os.IsNotExist(err) {
		// If the state file does not exist, assume the state was never
		// written, and therefore the uniter has not been started.
		return false, nil
	} else if err != nil {
		return false, errors.Annotatef(err, "could not open uniter state file %q", file)
	}
	var s bool
	dec := json.NewDecoder(f)
	err = dec.Decode(&s)
	if err != nil {
		return false, errors.Annotatef(err, "could not decode uniter state file %q", file)
	}
	return s, nil
}

func writeStateFileImpl(file string, value bool) error {
	b := &bytes.Buffer{}
	enc := json.NewEncoder(b)
	err := enc.Encode(value)
	if err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(ioutil.WriteFile(file, b.Bytes(), 0755))
}

// uniterStateWorker is a degenerate worker that keeps track o
// to its lock.
type uniterStateWorker struct {
	tomb tomb.Tomb
	sync.RWMutex

	file    string
	started bool
}

// Kill is part of the worker.Worker interface.
func (w *uniterStateWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *uniterStateWorker) Wait() error {
	return w.tomb.Wait()
}

// GetStarted is part of the UniterState interface.
func (u *uniterStateWorker) GetStarted() bool {
	u.RLock()
	defer u.RUnlock()
	return u.started
}

// SetStarted is part of the UniterState interface. If writing
// to the persistence file fails, the worker will shut down.
func (u *uniterStateWorker) SetStarted(started bool) (err error) {
	u.Lock()
	defer u.Unlock()
	// Set the in-memory variable if persisting succeeds,
	// shut down worker otherwise.
	defer func() {
		if err == nil {
			u.started = started
		} else {
			u.tomb.Kill(err)
		}
	}()
	err = writeStateFile(u.file, started)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}
