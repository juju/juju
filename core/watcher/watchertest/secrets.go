// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watchertest

import (
	"sort"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/testing"
)

// SecretsTriggerWatcherC embeds a gocheck.C and adds methods to help
// verify the behaviour of any watcher that uses a
// <-chan []SecretTriggerChange
type SecretsTriggerWatcherC struct {
	*gc.C
	Watcher watcher.SecretTriggerWatcher
}

// NewSecretsTriggerWatcherC returns a SecretsTriggerWatcherC that
// checks for aggressive event coalescence.
func NewSecretsTriggerWatcherC(c *gc.C, w watcher.SecretTriggerWatcher) SecretsTriggerWatcherC {
	return SecretsTriggerWatcherC{
		C:       c,
		Watcher: w,
	}
}

func (c SecretsTriggerWatcherC) AssertNoChange() {
	select {
	case actual, ok := <-c.Watcher.Changes():
		c.Fatalf("watcher sent unexpected change: (%v, %v)", actual, ok)
	case <-time.After(testing.ShortWait):
	}
}

// AssertChange asserts the given changes was reported by the watcher,
// but does not assume there are no following changes.
func (c SecretsTriggerWatcherC) AssertChange(expect ...watcher.SecretTriggerChange) {
	var received []watcher.SecretTriggerChange
	timeout := time.After(testing.LongWait)
	for a := testing.LongAttempt.Start(); a.Next(); {
		select {
		case actual, ok := <-c.Watcher.Changes():
			c.Logf("Secrets Trigger Watcher.Changes() => %# v", actual)
			c.Assert(ok, jc.IsTrue)
			received = append(received, actual...)
			if len(received) >= len(expect) {
				mc := jc.NewMultiChecker()
				mc.AddExpr(`_[_].NextTriggerTime`, jc.Almost, jc.ExpectedValue)
				c.Assert(received, mc, expect)
				return
			}
		case <-timeout:
			c.Fatalf("watcher did not send change")
		}
	}
}

func (c SecretsTriggerWatcherC) AssertClosed() {
	select {
	case _, ok := <-c.Watcher.Changes():
		c.Assert(ok, jc.IsFalse)
	default:
		c.Fatalf("watcher not closed")
	}
}

// SecretBackendRotateWatcherC embeds a gocheck.C and adds methods to help
// verify the behaviour of any watcher that uses a
// <-chan []SecretBackendRotateChange
type SecretBackendRotateWatcherC struct {
	*gc.C
	Watcher watcher.SecretBackendRotateWatcher
}

// NewSecretBackendRotateWatcherC returns a SecretBackendRotateWatcherC that
// checks for aggressive event coalescence.
func NewSecretBackendRotateWatcherC(c *gc.C, w watcher.SecretBackendRotateWatcher) SecretBackendRotateWatcherC {
	return SecretBackendRotateWatcherC{
		C:       c,
		Watcher: w,
	}
}

func (c SecretBackendRotateWatcherC) AssertNoChange() {
	select {
	case actual, ok := <-c.Watcher.Changes():
		c.Fatalf("watcher sent unexpected change: (%v, %v)", actual, ok)
	case <-time.After(testing.ShortWait):
	}
}

// AssertChanges asserts the given changes was reported by the watcher,
// but does not assume there are no following changes.
func (c SecretBackendRotateWatcherC) AssertChanges(expect ...watcher.SecretBackendRotateChange) {
	var received []watcher.SecretBackendRotateChange
	timeout := time.After(testing.LongWait)
	for a := testing.LongAttempt.Start(); a.Next(); {
		select {
		case actual, ok := <-c.Watcher.Changes():
			c.Logf("Secrets Trigger Watcher.Changes() => %# v", actual)
			c.Assert(ok, jc.IsTrue)
			sort.Slice(actual, func(i, j int) bool {
				return actual[i].Name < actual[j].Name
			})
			received = append(received, actual...)
			if len(received) >= len(expect) {
				mc := jc.NewMultiChecker()
				mc.AddExpr(`_[_].NextTriggerTime`, jc.Almost, jc.ExpectedValue)
				c.Assert(received, mc, expect)
				return
			}
		case <-timeout:
			c.Fatalf("watcher did not send change")
		}
	}
}

func (c SecretBackendRotateWatcherC) AssertClosed() {
	select {
	case _, ok := <-c.Watcher.Changes():
		c.Assert(ok, jc.IsFalse)
	default:
		c.Fatalf("watcher not closed")
	}
}
