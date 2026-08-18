// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitless

import (
	"context"

	coreapplication "github.com/juju/juju/core/application"
	coreerrors "github.com/juju/juju/core/errors"
	corelife "github.com/juju/juju/core/life"
	"github.com/juju/juju/internal/errors"
)

// ScriptletApplication contains an application's identity, lifecycle, and
// Starform sources.
type ScriptletApplication struct {
	UUID coreapplication.UUID
	Name string
	Life corelife.Value

	// Sources are the Starform sources to load.
	Sources []ScriptSource
}

// Validate checks that a scriptlet application can be loaded.
func (s ScriptletApplication) Validate() error {
	if len(s.Sources) == 0 {
		return errors.New("no scriptlet sources not valid").Add(coreerrors.NotValid)
	}
	for _, source := range s.Sources {
		if source.LoadPath == "" {
			return errors.New("empty scriptlet source path not valid").Add(coreerrors.NotValid)
		}
		if source.Source == "" {
			return errors.Errorf("empty scriptlet source %q not valid", source.LoadPath).Add(coreerrors.NotValid)
		}
	}
	return nil
}

// ScriptSource is one Starform source file.
type ScriptSource struct {
	// LoadPath is the stable logical Starlark source/load path.
	LoadPath string

	// Source is the Starlark source text.
	Source string
}

// Path implements starform.ScriptSource.
func (s ScriptSource) Path() string {
	return s.LoadPath
}

// Content implements starform.ScriptSource.
func (s ScriptSource) Content(context.Context) ([]byte, error) {
	return []byte(s.Source), nil
}

// Event is emitted to an application's staged scriptlet when something
// pertinent to the application changes.
type Event struct {
	// Name is the name of the event.
	Name string

	// Attrs are data associated with the event,
	// particular to the application receiving it.
	Attrs map[string]any
}
