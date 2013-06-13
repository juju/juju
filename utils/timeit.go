// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"fmt"
	"os"
	"time"
)

type timer struct {
	action     string
	start      time.Time
	depth      int
	duration   time.Duration
	subActions []*timer
}

func (t *timer) String() string {
	this := fmt.Sprintf("%.3fs %*s%s\n", t.duration.Seconds(), t.depth, "", t.action)
	for _, sub := range t.subActions {
		this += sub.String()
	}
	return this
}

var stack []*timer

// Start a timer, used for tracking time spent.
// Generally used with either defer, as in:
//  defer utils.Timeit("my func")()
// Which will track how much time is spent in your function. Or
// if you want to track the time spent in a function you are calling
// then you would use:
//  toc := utils.Timeit("anotherFunc()")
//  anotherFunc()
//  toc()
// This tracks nested calls by indenting the output, and will print out the
// full stack of timing when we reach the top of the stack.
func Timeit(action string) func() {
	cur := &timer{action: action, start: time.Now(), depth: len(stack)}
	if len(stack) != 0 {
		tip := stack[len(stack)-1]
		tip.subActions = append(tip.subActions, cur)
	}
	stack = append(stack, cur)
	return func() {
		cur.duration = time.Since(cur.start)
		if len(stack) == 0 || cur == stack[0] {
			fmt.Fprint(os.Stderr, cur)
			stack = nil
		} else {
			stack = stack[0 : len(stack)-1]
		}
	}
}
