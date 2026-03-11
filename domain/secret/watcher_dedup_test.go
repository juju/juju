// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secret_test

import (
	"context"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain/secret"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type watcherDedupSuite struct {
}

func TestWatcherDedupSuite(t *stdtesting.T) {
	tc.Run(t, &watcherDedupSuite{})
}

func (s *watcherDedupSuite) TestDedupEvents(c *tc.C) {
	logger := loggertesting.WrapCheckLog(c)

	inputChan := make(chan []string)
	sw, err := secret.NewSecretStringWatcher(
		watchertest.NewMockStringsWatcher(inputChan),
		logger,
		func(ctx context.Context, events ...string) ([]string, error) {
			return events, nil
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	defer watchertest.CleanKill(c, sw)
	w := watchertest.NewStringsWatcherC(c, sw)

	addValues := func(values ...string) {
		inputChan <- values
	}

	// Add several values with duplications.
	addValues("foo")
	addValues("foo", "bar")
	addValues("bar")

	w.AssertChange("foo", "bar") // first fetches should avoid duplication.
	w.AssertNoChange()

	// Add a new value that has been already given.
	addValues("foo", "baz")
	w.AssertChange("foo", "baz") // We should get foo again.
	w.AssertNoChange()
}
