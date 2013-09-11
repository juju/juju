// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config

// HasConfig instances have an environment configuration.
type HasConfig interface {
	// Config returns the current configuration of an instance.
	Config() *Config
}
