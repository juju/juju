// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventsource

import (
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
	dbtesting "github.com/juju/juju/database/testing"
)

//go:generate go run github.com/golang/mock/mockgen -package eventsource -destination package_mock_test.go github.com/juju/juju/core/watcher/eventsource Logger
//go:generate go run github.com/golang/mock/mockgen -package eventsource -destination changestream_mock_test.go github.com/juju/juju/core/changestream Subscription,EventSource

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type baseSuite struct {
	dbtesting.ControllerSuite

	events *MockEventSource
	logger *MockLogger
	sub    *MockSubscription
}

func (s *baseSuite) setUpMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.events = NewMockEventSource(ctrl)
	s.logger = NewMockLogger(ctrl)
	s.sub = NewMockSubscription(ctrl)

	return ctrl
}

func (s *baseSuite) newBaseWatcher() *BaseWatcher {
	return NewBaseWatcher(s.events, s.TxnRunner(), s.logger)
}

// subscriptionOptionMatcher is a gomock.Matcher that can be used to check
// that subscription options match, by comparing their namespaces and masks.
// The filter func is omitted from comparison.
type subscriptionOptionMatcher struct {
	opt changestream.SubscriptionOption
}

// Matches returns true if the argument is a changestream.SubscriptionOption,
// and its namespace and mask match those of our member.
func (m subscriptionOptionMatcher) Matches(arg interface{}) bool {
	optArg, ok := arg.(changestream.SubscriptionOption)
	if !ok {
		return false
	}

	return optArg.Namespace() == m.opt.Namespace() && optArg.ChangeMask() == m.opt.ChangeMask()
}

// String exists to satisfy the gomock.Matcher interface.
func (m subscriptionOptionMatcher) String() string {
	return fmt.Sprintf("%s %d", m.opt.Namespace(), m.opt.ChangeMask())
}

type changeEvent struct {
	changeType changestream.ChangeType
	namespace  string
	uuid       string
}

func (e changeEvent) Type() changestream.ChangeType {
	return e.changeType
}

func (e changeEvent) Namespace() string {
	return e.namespace
}

func (e changeEvent) ChangedUUID() string {
	return e.uuid
}
