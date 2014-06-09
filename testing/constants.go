// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"time"

	"github.com/juju/utils"
)

// ShortWait is a reasonable amount of time to block waiting for something that
// shouldn't actually happen. (as in, the test suite will *actually* wait this
// long before continuing)
const ShortWait = 50 * time.Millisecond

// LongWait is used when something should have already happened, or happens
// quickly, but we want to make sure we just haven't missed it. As in, the test
// suite should proceed without sleeping at all, but just in case. It is long
// so that we don't have spurious failures without actually slowing down the
// test suite
const LongWait = 10 * time.Second

var LongAttempt = &utils.AttemptStrategy{
	Total: LongWait,
	Delay: ShortWait,
}
