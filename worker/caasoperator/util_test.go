// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator_test

import (
	"sync"
	"time"

	"github.com/juju/juju/testing"
	gc "gopkg.in/check.v1"
)

type hookObserver struct {
	mu             sync.Mutex
	hooksCompleted []string
}

func (ctx *hookObserver) HookCompleted(hookName string) {
	ctx.mu.Lock()
	ctx.hooksCompleted = append(ctx.hooksCompleted, hookName)
	ctx.mu.Unlock()
}

func (ctx *hookObserver) HookFailed(hookName string) {
	ctx.mu.Lock()
	ctx.hooksCompleted = append(ctx.hooksCompleted, "fail-"+hookName)
	ctx.mu.Unlock()
}

func (ctx *hookObserver) matchHooks(c *gc.C, hooks []string) bool {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	c.Logf("actual hooks: %#v", ctx.hooksCompleted)
	c.Logf("expected hooks: %#v", hooks)
	if len(ctx.hooksCompleted) < len(hooks) {
		return false
	}
	for i, e := range hooks {
		if ctx.hooksCompleted[i] != e {
			return false
		}
	}
	return true
}

func (ctx *hookObserver) waitForHooks(c *gc.C, hooks []string) {
	c.Logf("waiting for hooks: %#v", hooks)
	timeout := time.After(testing.LongWait)
	for {
		select {
		case <-time.After(testing.ShortWait):
			if ctx.matchHooks(c, hooks) {
				return
			}
		case <-timeout:
			c.Fatalf("never got expected hooks")
		}
	}
}
