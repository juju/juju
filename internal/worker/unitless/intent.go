// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitless

import (
	"github.com/canonical/starform/starform"
	"github.com/canonical/starlark/starlark"

	"github.com/juju/juju/internal/errors"
)

// IntentType identifies a type of Intent.
type IntentType string

// Intent is a declared action resulting from scriptlet execution.
type Intent struct {
	// Type identifies the type of intent.
	Type IntentType

	// Args are arguments to pass to service methods that
	// are called according to the intent type.
	Args map[string]any
}

// IntentCollector accumulates intents during one scriptlet event.
type IntentCollector struct {
	intents []Intent
}

// Intents returns all collected intents.
func (c *IntentCollector) Intents() []Intent {
	return c.intents
}

func (c *IntentCollector) append(thread *starlark.Thread, intent Intent) error {
	if err := thread.AddAllocs(starlark.EstimateSize(intent)); err != nil {
		return errors.Errorf("adding allocations for %q: %w", intent.Type, err)
	}

	appender := starlark.NewSafeAppender(thread, &c.intents)
	if err := appender.Append(intent); err != nil {
		return errors.Errorf("appending intent %q: %w", intent.Type, err)
	}
	return nil
}

func intentCollectorForThread(thread *starlark.Thread) (*IntentCollector, error) {
	event := starform.Event(thread)
	collector, ok := event.State.(*IntentCollector)
	if !ok || collector == nil {
		return nil, starform.ErrUnavailable
	}
	return collector, nil
}
