// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"github.com/juju/errors"
)

type Description struct {
	// Version conceptually encapsulates an understanding of which fields
	// exist and how they are populated. As extra fields and entities are
	// added, the version should be incremented and tests written to ensure
	// that newer versions of the code are still able to create Model
	// representations from versions.
	//
	// The version is all about the serialization of the structures from
	// the migration package. Each type will likely have a version.
	Version int
	Model   Model
	// TODO: extra binaries...
	// Tools
	// Charms
}

type Model struct {
	Version int `yaml:"version"`

	UUID  string `yaml:"model-uuid"`
	Name  string `yaml:"name"`
	Owner string `yaml:"owner"`

	Machines []Machine `yaml:"machines"`

	// TODO: add extra entities, but initially focus on Machines.
	// Services, and through them, Units
	// Relations
	// Spaces
	// Storage

}

// NewModel constructs a new Model from a map that in normal usage situations
// will be the result of interpreting a large YAML document.
func NewModel(source map[string]interface{}) (*Model, error) {
	version, ok := source["version"]
	if !ok {
		return nil, errors.New("missing 'version'")
	}

	_ = version

	return &Model{}, nil
}

type Machine struct {
	Version int `yaml:"version"`
}
