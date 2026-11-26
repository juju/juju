// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package scriptlet

import (
	"fmt"
	"reflect"

	"github.com/canonical/starform/starform"
	"github.com/canonical/starlark/starlark"
)

// IntentCollectors holds the intent collectors for one event.
type IntentCollectors struct {
	collectorByType map[reflect.Type]any
}

// add registers the given collector, panicking on duplicates.
func (ic *IntentCollectors) add(collector any) {
	if ic.collectorByType == nil {
		ic.collectorByType = make(map[reflect.Type]any)
	}
	key := reflect.TypeOf(collector)
	if _, ok := ic.collectorByType[key]; ok {
		panic(fmt.Sprintf("intent collector already registered %v", key))
	}
	ic.collectorByType[key] = collector
}

// IntentCollector returns the collector for type T. It panics if none was
// registered.
func IntentCollector[T any](collectors *IntentCollectors) T {
	var zero T
	intentCollector, ok := collectors.collectorByType[reflect.TypeFor[T]()]
	if !ok {
		panic(fmt.Sprintf("internal error: intent collector %T not registered", zero))
	}
	return intentCollector.(T)
}

// intentCollectorForThread returns the T collector of the thread's
// event state, or starform.ErrUnavailable without an observed event.
func intentCollectorForThread[T any](thread *starlark.Thread) (T, error) {
	collectors, ok := starform.Event(thread).State.(*IntentCollectors)
	if !ok || collectors == nil {
		var zero T
		return zero, starform.ErrUnavailable
	}
	return IntentCollector[T](collectors), nil
}
