// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventsource

import "github.com/juju/juju/core/changestream"

// FilterOption is a filter option for the FilterWatcher.
type FilterOption interface {
	// Namespace is the namespace to watch for changes.
	Namespace() string

	// ChangeMask is the type of change to watch for.
	ChangeMask() changestream.ChangeType

	// ChangePredicate returns a function that returns true if the change event
	// is selected for emission.
	ChangePredicate() Predicate
}

// Predicate is the filter function type for FilterOptions. A predicate
// should return true when passed a change event change string that should be
// emitted by a watcher, false otherwise.
type Predicate func(changed string) bool

// AlwaysPredicate is a predicate that accepts all change events.
var AlwaysPredicate Predicate = func(string) bool { return true }

// EqualsPredicate returns a predicate that only accepts change events whose value
// is equal to the supplied value.
func EqualsPredicate(value string) Predicate {
	return func(s string) bool { return s == value }
}

type filter struct {
	namespace       string
	changeMask      changestream.ChangeType
	changePredicate Predicate
}

// Namespace is the namespace to watch for changes.
func (f filter) Namespace() string {
	return f.namespace
}

// ChangeMask is the type of change to watch for.
func (f filter) ChangeMask() changestream.ChangeType {
	return f.changeMask
}

// ChangePredicate returns a function that returns true if the change event is
// selected for emission.
func (f filter) ChangePredicate() Predicate {
	return f.changePredicate
}

// PredicateFilter returns a filter option that filters the watcher changes
// based on the predicate. The filter will only emit events from the namespace
// that match the change mask and cause the supplied predicate to return true.
func PredicateFilter(namespace string, changeMask changestream.ChangeType, changePredicate Predicate) FilterOption {
	return filter{
		namespace:       namespace,
		changeMask:      changeMask,
		changePredicate: changePredicate,
	}
}

// NamespaceFilter returns a filter option that filters the watcher changes in
// the namespace that match the change mask. The filter will emit all events
// from the namespace that match the change mask.
func NamespaceFilter(namespace string, changeMask changestream.ChangeType) FilterOption {
	return filter{
		namespace:       namespace,
		changeMask:      changeMask,
		changePredicate: AlwaysPredicate,
	}
}
