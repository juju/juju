// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testbacking

import (
	"strings"
	"sync"
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/core/multiwatcher"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/testing"
)

// Backing is a test state AllWatcherBacking
type Backing struct {
	mu       sync.Mutex
	fetchErr error
	entities map[multiwatcher.EntityID]multiwatcher.EntityInfo
	watchc   chan<- watcher.Change
	txnRevno int64
}

// New returns a new test backing.
func New(initial []multiwatcher.EntityInfo) *Backing {
	b := &Backing{
		entities: make(map[multiwatcher.EntityID]multiwatcher.EntityInfo),
	}
	for _, info := range initial {
		b.entities[info.EntityID()] = info
	}
	return b
}

// Changed process the change event from the state base watcher.
func (b *Backing) Changed(store multiwatcher.Store, change watcher.Change) error {
	modelUUID, changeID, ok := SplitDocID(change.Id.(string))
	if !ok {
		return errors.Errorf("unexpected id format: %v", change.Id)
	}
	id := multiwatcher.EntityID{
		Kind:      change.C,
		ModelUUID: modelUUID,
		ID:        changeID,
	}
	info, err := b.fetch(id)
	if errors.Is(err, errors.NotFound) {
		store.Remove(id)
		return nil
	}
	if err != nil {
		return err
	}
	store.Update(info)
	return nil
}

func (b *Backing) fetch(id multiwatcher.EntityID) (multiwatcher.EntityInfo, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.fetchErr != nil {
		return nil, b.fetchErr
	}
	if info, ok := b.entities[id]; ok {
		return info, nil
	}
	return nil, errors.NotFoundf("%s.%s", id.Kind, id.ID)
}

// Watch sets up the channel for the events.
func (b *Backing) Watch(c chan<- watcher.Change) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.watchc != nil {
		panic("test backing can only watch once")
	}
	b.watchc = c
}

// Unwatch clears the channel for the events.
func (b *Backing) Unwatch(c chan<- watcher.Change) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if c != b.watchc {
		panic("unwatching wrong channel")
	}
	b.watchc = nil
}

// GetAll does the initial population of the store.
func (b *Backing) GetAll(store multiwatcher.Store) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, info := range b.entities {
		store.Update(info)
	}
	return nil
}

// UpdateEntity allows the test to push an update.
func (b *Backing) UpdateEntity(info multiwatcher.EntityInfo) {
	b.mu.Lock()
	id := info.EntityID()
	b.entities[id] = info
	b.txnRevno++
	change := watcher.Change{
		C:     id.Kind,
		Id:    EnsureModelUUID(id.ModelUUID, id.ID),
		Revno: b.txnRevno, // This is actually ignored, but fill it in anyway.
	}
	listener := b.watchc
	b.mu.Unlock()
	if b.watchc != nil {
		select {
		case listener <- change:
		case <-time.After(testing.LongWait):
			panic("watcher isn't reading off channel")

		}
	}
}

// SetFetchError queues up an error to return on the next fetch.
func (b *Backing) SetFetchError(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.fetchErr = err
}

// DeleteEntity allows the test to push a delete through the test.
func (b *Backing) DeleteEntity(id multiwatcher.EntityID) {
	b.mu.Lock()
	delete(b.entities, id)
	change := watcher.Change{
		C:     id.Kind,
		Id:    EnsureModelUUID(id.ModelUUID, id.ID),
		Revno: -1,
	}
	b.txnRevno++
	listener := b.watchc
	b.mu.Unlock()
	if b.watchc != nil {
		select {
		case listener <- change:
		case <-time.After(testing.LongWait):
			panic("watcher isn't reading off channel")
		}
	}
}

// EnsureModelUUID is exported as it is used in other _test files.
func EnsureModelUUID(modelUUID, id string) string {
	prefix := modelUUID + ":"
	if strings.HasPrefix(id, prefix) {
		return id
	}
	return prefix + id
}

// SplitDocID is exported as it is used in other _test files.
func SplitDocID(id string) (string, string, bool) {
	parts := strings.SplitN(id, ":", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}
