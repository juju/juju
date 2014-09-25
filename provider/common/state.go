// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"

	"github.com/juju/errors"
	goyaml "gopkg.in/yaml.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/instance"
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

// AddStateInstance adds a state-server instance ID to the provider-state
// file in storage.
func AddStateInstance(stor storage.Storage, id instance.Id) error {
	state, err := LoadState(stor)
	if err == environs.ErrNotBootstrapped {
		state = &BootstrapState{}
	} else if err != nil {
		return errors.Annotate(err, "cannot record state instance-id")
	}
	state.StateInstances = append(state.StateInstances, id)
	return SaveState(stor, state)
}

// RemoveStateInstances removes state-server instance IDs from the
// provider-state file in storage. Instance IDs that are not found
// in the file are ignored.
func RemoveStateInstances(stor storage.Storage, ids ...instance.Id) error {
	state, err := LoadState(stor)
	if err == environs.ErrNotBootstrapped {
		return nil
	} else if err != nil {
		return errors.Annotate(err, "cannot remove recorded state instance-id")
	}
	var anyFound bool
	for i := 0; i < len(state.StateInstances); i++ {
		for _, id := range ids {
			if state.StateInstances[i] == id {
				head := state.StateInstances[:i]
				tail := state.StateInstances[i+1:]
				state.StateInstances = append(head, tail...)
				anyFound = true
				i--
				break
			}
		}
	}
	if !anyFound {
		return nil
	}
	return SaveState(stor, state)
}

// ProviderStateInstances extracts the instance IDs from provider-state.
func ProviderStateInstances(
	env environs.Environ,
	stor storage.StorageReader,
) ([]instance.Id, error) {
	st, err := LoadState(stor)
	if err != nil {
		return nil, err
	}
	return st.StateInstances, nil
}
