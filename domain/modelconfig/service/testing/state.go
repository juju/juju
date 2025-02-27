// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"fmt"

	"github.com/juju/juju/core/changestream"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/domain/testing"
)

// MemoryState implements an in memory representation of the state required for
// managing model config.
type MemoryState struct {
	Config map[string]string
	*testing.NamespaceWatcherFactory
}

const (
	allKeysQuery = "may I please have some keys"
)

func (*MemoryState) AgentVersion(_ context.Context) (string, error) {
	return jujuversion.Current.String(), nil
}

// AllKeysQuery implements the AllKeysQuery func required by state.
func (s *MemoryState) AllKeysQuery() string {
	return allKeysQuery
}

// KeysQuery performs a query for all of the model config keys currently set and
// returns them as a slice of strings. If they query does not match allKeysQuery
// then an error is returned.
func (s *MemoryState) KeysQuery(query string) ([]string, error) {
	if query != allKeysQuery {
		return []string{}, fmt.Errorf("unexpected all keys query %q", query)
	}

	keys := make([]string, 0, len(s.Config))
	for k := range s.Config {
		keys = append(keys, k)
	}
	return keys, nil
}

// ModelConfig returns the currently set config for the model.
func (s *MemoryState) ModelConfig(_ context.Context) (map[string]string, error) {
	return s.Config, nil
}

// ModelConfigHasAttributes returns the set of attributes that model config
// currently has set out of the list supplied.
func (s *MemoryState) ModelConfigHasAttributes(
	_ context.Context,
	hasAttrs []string,
) ([]string, error) {
	rval := make([]string, 0, len(hasAttrs))
	for _, has := range hasAttrs {
		if _, y := s.Config[has]; y {
			rval = append(rval, has)
		}
	}
	return rval, nil
}

// NewState constructs a new in memory state for model config.
func NewState() *MemoryState {
	st := &MemoryState{
		Config: make(map[string]string),
	}
	st.NamespaceWatcherFactory = testing.NewNamespaceWatcherFactory(
		func() ([]string, error) {
			return st.KeysQuery(allKeysQuery)
		},
	)
	return st
}

// SetModelConfig is responsible for setting the current model config and
// overwriting all previously set values even if the config supplied is
// empty or nil.
func (s *MemoryState) SetModelConfig(
	ctx context.Context,
	conf map[string]string,
) error {
	if conf == nil {
		conf = map[string]string{}
	}
	s.Config = conf

	changes, err := s.KeysQuery(allKeysQuery)
	if err != nil {
		return fmt.Errorf("getting model config keys")
	}
	return s.FeedChange(ctx, "model_config", changestream.Changed, changes)
}

// UpdateModelConfig is responsible for both inserting, updating and
// removing model config values for the current model.
func (s *MemoryState) UpdateModelConfig(
	ctx context.Context,
	update map[string]string,
	remove []string,
) error {
	for _, k := range remove {
		delete(s.Config, k)
	}
	for k, v := range update {
		s.Config[k] = v
	}

	// At the moment this is a little hacky. We should be breaking the changes up
	// into their respective update types and filtering out any keys that aren't
	// having a delta applied. This will require some sync work on the channel
	// as we don't want to end up with go routine hell in the tests.
	// Future needs can expand on this if we are watching for specific types of
	// update changes. For now this will do.
	changes, err := s.KeysQuery(allKeysQuery)
	if err != nil {
		return fmt.Errorf("getting model config keys")
	}
	changes = append(changes, remove...)
	return s.FeedChange(ctx, "model_config", changestream.Changed, changes)
}

// NamespaceForWatchModelConfig returns the namespace identifier used for
// watching model configuration changes.
func (s *MemoryState) NamespaceForWatchModelConfig() string {
	return "model_config"
}

// SpaceExists checks if the space identified by the given space name exists.
func (st *MemoryState) SpaceExists(ctx context.Context, spaceName string) (bool, error) {
	return false, nil
}
