// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keyvalue

import (
	"fmt"
	"os"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/utils"
)

// notSetError denotes that the value for the specified key was not set.
type notSetError struct {
	key string
}

// Error implements the error interface.
func (e *notSetError) Error() string {
	return fmt.Sprintf("value for key %q not set", e.key)
}

func newNotSetError(key string) *notSetError {
	return &notSetError{key: key}
}

// IsNotSetError returns true if the error is a notSetError.
func IsNotSetError(e error) bool {
	_, ok := errors.Cause(e).(*notSetError)
	return ok
}

// newKeyValueStore creates a store for persistent storage of
// the key-value pairs.
var newKeyValueStore = func(path string) *KeyValueStore {
	return &KeyValueStore{path: path, data: make(map[string]interface{})}
}

// KeyValueStore stores the key-value pairs and persists them
// in a file on disk.
type KeyValueStore struct {
	mu   sync.RWMutex
	data map[string]interface{}
	path string
}

// Get retrieves the value for the specified key.
func (f *KeyValueStore) Get(key string) (interface{}, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	value, ok := f.data[key]
	if ok {
		return value, nil
	}

	if err := utils.ReadYaml(f.path, &f.data); err != nil {
		if os.IsNotExist(err) {
			return nil, newNotSetError(key)
		}
		return nil, errors.Trace(err)
	}

	value, ok = f.data[key]
	if ok {
		return value, nil
	}
	return nil, newNotSetError(key)
}

// Set stores the value for the specified key.
func (f *KeyValueStore) Set(key string, in interface{}) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data[key] = in
	err := utils.WriteYaml(f.path, f.data)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}
