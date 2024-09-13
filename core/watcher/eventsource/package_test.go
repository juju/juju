// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventsource

import (
	"fmt"
	"testing"

	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	dbtesting "github.com/juju/juju/internal/database/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package eventsource -destination changestream_mock_test.go github.com/juju/juju/core/changestream Subscription,WatchableDB,EventSource
//go:generate go run go.uber.org/mock/mockgen -typed -package eventsource -destination watcher_mock_test.go -source=./consume.go

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type ImportTest struct{}

var _ = gc.Suite(&ImportTest{})

func (*ImportTest) TestImports(c *gc.C) {
	found := coretesting.FindJujuCoreImports(c, "github.com/juju/juju/core/watcher/eventsource")

	// This package brings in nothing else from outside juju/juju/core
	c.Assert(found, jc.SameContents, []string{
		"core/changestream",
		"core/database",
		"core/life",
		"core/logger",
		"core/migration",
		"core/network",
		"core/resources",
		"core/secrets",
		"core/status",
		"core/watcher",
		"internal/charm/resource",
		"internal/errors",
		"internal/logger",
		"internal/uuid",
	})

}

type watchableDBShim struct {
	database.TxnRunner
	changestream.EventSource
}

type baseSuite struct {
	dbtesting.DqliteSuite

	watchableDB watchableDBShim
	eventsource *MockEventSource
	sub         *MockSubscription
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.eventsource = NewMockEventSource(ctrl)
	s.watchableDB = watchableDBShim{
		TxnRunner:   s.TxnRunner(),
		EventSource: s.eventsource,
	}
	s.sub = NewMockSubscription(ctrl)

	return ctrl
}

func (s *baseSuite) newBaseWatcher(c *gc.C) *BaseWatcher {
	return NewBaseWatcher(s.watchableDB, loggertesting.WrapCheckLog(c))
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
	changed    string
}

func (e changeEvent) Type() changestream.ChangeType {
	return e.changeType
}

func (e changeEvent) Namespace() string {
	return e.namespace
}

func (e changeEvent) Changed() string {
	return e.changed
}
