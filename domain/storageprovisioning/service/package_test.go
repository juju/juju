// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain/life"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination package_mock_test.go -source service.go

// eventSourceFilterMatcher is a gomock matcher that checks a watcher filter
// to make sure it has the correct change mask and namespace.
type eventSourceFilterMatcher struct {
	ChangeMask changestream.ChangeType
	Namespace  string
}

// eventSourcePredFilterMatcher is a gomock matcher that checks a watcher filter
// to make sure it has the correct change mask, namespace and  that the
// predicate returned true for the value in
// [eventSourcePredFiltermatcher.Predicate].
type eventSourcePredFilterMatcher struct {
	ChangeMask changestream.ChangeType
	Namespace  string
	Predicate  string
}

// Matches check to see the supplied value is an [eventsource.FilterOption] and
// has the correct change mask and namespace.
func (m eventSourceFilterMatcher) Matches(v any) bool {
	filter, ok := v.(eventsource.FilterOption)
	if !ok {
		return false
	}
	return filter.Namespace() == m.Namespace &&
		filter.ChangeMask() == m.ChangeMask
}

// Matches check to see the supplied value is an [eventsource.FilterOption] and
// has the correct change mask, namespace and that the predicate returns true.
func (m eventSourcePredFilterMatcher) Matches(v any) bool {
	filter, ok := v.(eventsource.FilterOption)
	if !ok {
		return false
	}

	return filter.Namespace() == m.Namespace &&
		filter.ChangeMask() == m.ChangeMask &&
		filter.ChangePredicate()(m.Predicate)
}

// String describes what the matcher matches.
func (m eventSourceFilterMatcher) String() string {
	return fmt.Sprintf("event source filter matches ns=%q mask=%q",
		m.Namespace, m.ChangeMask)
}

// String describes what the matcher matches.
func (m eventSourcePredFilterMatcher) String() string {
	return fmt.Sprintf("event source filter matches ns=%q mask=%q",
		m.Namespace, m.ChangeMask)
}

// namespaceQueryReturningError provides a fake [eventsource.NamespaceQuery] for
// service tests. The query returned fails the test if the query is executed as
// the service should not be doing this.
func namespaceQueryReturningError(t *testing.T) eventsource.NamespaceQuery {
	return func(_ context.Context, _ database.TxnRunner) ([]string, error) {
		t.Fatal("namespace query should not have been called")
		return nil, nil
	}
}

// namespaceLifeQueryReturningError provides a fake [eventsource.Query] for
// service tests. The query returned fails the test if the query is executed as
// the service should not be doing this.
func namespaceLifeQueryReturningError(t *testing.T) eventsource.Query[map[string]life.Life] {
	return func(_ context.Context, _ database.TxnRunner) (map[string]life.Life, error) {
		t.Fatal("namespace life query should not have been called")
		return nil, nil
	}
}
