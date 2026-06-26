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

const (
	// IntentStatusSet declares an application status update.
	IntentStatusSet IntentType = "status-set"
)

// Intent is a declared action resulting from scriptlet execution.
// TODO (manadart 2026-06-10): This is a placeholder, and reflects the single
// status-set intent. It will take a generic form as the feature is evolved.
// We may want to do the following:
//   - Use a visitor pattern to accumulate intents as Builtins in
//     NewStarformExecutor.
type Intent struct {
	Type    IntentType
	Status  string
	Message string
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

func (c *IntentCollector) statusSet(thread *starlark.Thread, status, message string) error {
	return c.append(thread, Intent{
		Type:    IntentStatusSet,
		Status:  status,
		Message: message,
	})
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

func statusSet(
	thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var status, message string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "status", &status, "message?", &message); err != nil {
		return nil, err
	}

	collector, err := intentCollectorForThread(thread)
	if err != nil {
		return nil, errors.Errorf("getting IntentCollector from thread: %w", err)
	}
	if err := collector.statusSet(thread, status, message); err != nil {
		return nil, err
	}
	return starlark.None, nil
}

func intentCollectorForThread(thread *starlark.Thread) (*IntentCollector, error) {
	event := starform.Event(thread)
	collector, ok := event.State.(*IntentCollector)
	if !ok || collector == nil {
		return nil, starform.ErrUnavailable
	}
	return collector, nil
}
