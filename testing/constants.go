// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"

	"github.com/juju/utils/v4"

	coretesting "github.com/juju/juju/core/testing"
)

// ShortWait is a reasonable amount of time to block waiting for something that
// shouldn't actually happen. (as in, the test suite will *actually* wait this
// long before continuing)
// Deprecated: use core/testing.ShortWait instead. This is so you don't bring
// in extra dependencies from this package.
const ShortWait = coretesting.ShortWait

// LongWait is used when something should have already happened, or happens
// quickly, but we want to make sure we just haven't missed it. As in, the test
// suite should proceed without sleeping at all, but just in case. It is long
// so that we don't have spurious failures without actually slowing down the
// test suite
// Deprecated: use core/testing.LongWait instead. This is so you don't bring
// in extra dependencies from this package.
const LongWait = coretesting.LongWait

// TODO(katco): 2016-08-09: lp:1611427
var LongAttempt = &utils.AttemptStrategy{
	Total: LongWait,
	Delay: ShortWait,
}

// LongWaitContext returns a context whose deadline is tied to the duration of
// a LongWait.
func LongWaitContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), LongWait)
}
