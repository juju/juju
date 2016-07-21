// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config

// Validator is an interface for validating model configuration.
type Validator interface {
	// Validate ensures that cfg is a valid configuration.
	// If old is not nil, Validate should use it to determine
	// whether a configuration change is valid.
	//
	// TODO(axw) Validate should just return an error. We should
	// use a separate mechanism for updating config.
	Validate(cfg, old *Config) (valid *Config, _ error)
}
