// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"

	"github.com/juju/errors"
	"launchpad.net/goyaml"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/storage"
	"launchpad.net/juju-core/instance"
)

// StateFile is the name of the file where the provider's state is stored.
const StateFile = "provider-state"

// BootstrapState is the state information that is stored in StateFile.
//
// Individual providers may define their own state structures instead of
// this one, and use their own code for loading and saving those, but this is
// the definition that most practically useful providers share unchanged.
type BootstrapState struct {
	// StateInstances are the state servers.
	StateInstances []instance.Id `yaml:"state-instances"`
}

// putState writes the given data to the state file on the given storage.
// The file's name is as defined in StateFile.
func putState(stor storage.StorageWriter, data []byte) error {
	logger.Debugf("putting %q to bootstrap storage %T", StateFile, stor)
	return stor.Put(StateFile, bytes.NewBuffer(data), int64(len(data)))
}

// CreateStateFile creates an empty state file on the given storage, and
// returns its URL.
func CreateStateFile(stor storage.Storage) (string, error) {
	err := putState(stor, []byte{})
	if err != nil {
		return "", fmt.Errorf("cannot create initial state file: %v", err)
	}
	return stor.URL(StateFile)
}

// DeleteStateFile deletes the state file on the given storage.
func DeleteStateFile(stor storage.Storage) error {
	return stor.Remove(StateFile)
}

// SaveState writes the given state to the given storage.
func SaveState(storage storage.StorageWriter, state *BootstrapState) error {
	data, err := goyaml.Marshal(state)
	if err != nil {
		return err
	}
	return putState(storage, data)
}

// LoadState reads state from the given storage.
func LoadState(stor storage.StorageReader) (*BootstrapState, error) {
	r, err := storage.Get(stor, StateFile)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, environs.ErrNotBootstrapped
		}
		return nil, err
	}
	return loadState(r)
}

func loadState(r io.ReadCloser) (*BootstrapState, error) {
	defer r.Close()
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("error reading %q: %v", StateFile, err)
	}
	var state BootstrapState
	err = goyaml.Unmarshal(data, &state)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling %q: %v", StateFile, err)
	}
	return &state, nil
}
