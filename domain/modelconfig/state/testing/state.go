// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
)

// MemoryState implements an in memory representation of the state required for
// managing model config.
type MemoryState struct {
	Config map[string]string
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

func NewState() *MemoryState {
	return &MemoryState{
		Config: make(map[string]string),
	}
}

// SetModelConfig is responsible for setting the current model config and
// overwriting all previously set values even if the config supplied is
// empty or nil.
func (s *MemoryState) SetModelConfig(
	_ context.Context,
	conf map[string]string,
) error {
	if conf == nil {
		conf = map[string]string{}
	}
	s.Config = conf
	return nil
}

// UpdateModelConfig is responsible for both inserting, updating and
// removing model config values for the current model.
func (s *MemoryState) UpdateModelConfig(
	_ context.Context,
	update map[string]string,
	remove []string,
) error {
	for _, k := range remove {
		if _, exists := s.Config[k]; exists {
			delete(s.Config, k)
		}
	}

	for k, v := range update {
		s.Config[k] = v
	}

	return nil
}
