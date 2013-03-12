package trivial

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

func Timeit(action string) func() {
	cur := &timer{action: action, start: time.Now(), depth: len(stack)}
	if len(stack) != 0 {
		tip := stack[len(stack)-1]
		tip.subActions = append(tip.subActions, cur)
	}
	stack = append(stack, cur)
	return func() {
		cur.duration = time.Since(cur.start)
		if cur == stack[0] {
			fmt.Fprint(os.Stderr, cur)
			stack = nil
		} else {
			stack = stack[0 : len(stack)-1]
		}
	}
}
