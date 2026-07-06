// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitless

import (
	"sync"

	"github.com/canonical/starform/starform"
	"github.com/canonical/starlark/starlark"

	"github.com/juju/juju/internal/errors"
)

// IntentType identifies a scriptlet intent type.
// Scriptlets may not have side effects; rather, they return intents with data
// corresponding to individual intent types, which are then actioned at the
// discretion of the Juju domain.
type IntentType string

// Intent is a declared action resulting from scriptlet execution.
// We may want to do the following:
//   - Use a visitor pattern to accumulate intents as Builtins in
//     NewStarformExecutor.
type Intent struct {
	Type    IntentType
	Args    map[string]any
}

// IntentCollector accumulates intents during one scriptlet event.
type IntentCollector struct {
	mu      sync.Mutex
	intents []Intent
}

// Intents returns a copy of all collected intents.
func (c *IntentCollector) Intents() []Intent {
	c.mu.Lock()
	defer c.mu.Unlock()

	result := make([]Intent, len(c.intents))
	copy(result, c.intents)
	return result
}

func (c *IntentCollector) append(thread *starlark.Thread, intent Intent) error {
	c.mu.Lock()
	defer c.mu.Unlock()

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
