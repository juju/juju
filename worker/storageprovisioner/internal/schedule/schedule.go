// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schedule

import (
	"container/heap"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/clock"
)

// Schedule provides a schedule for storage operations, with the following
// properties:
//  - fast to add and remove items by key: O(log(n)); n is the total number of items
//  - fast to identify/remove the next scheduled item: O(log(n))
type Schedule struct {
	time  clock.Clock
	items scheduleItems
	m     map[interface{}]*scheduleItem
}

// NewSchedule constructs a new schedule, using the given Clock for the Next
// method.
func NewSchedule(clock clock.Clock) *Schedule {
	return &Schedule{
		time: clock,
		m:    make(map[interface{}]*scheduleItem),
	}
}

// Next returns a channel which will send after the next scheduled item's time
// has been reached. If there are no scheduled items, nil is returned.
func (s *Schedule) Next() <-chan time.Time {
	if len(s.items) > 0 {
		return s.time.After(s.items[0].t.Sub(s.time.Now()))
	}
	return nil
}

// Ready returns the parameters for items that are scheduled at or before
// "now", and removes them from the schedule. The resulting slices are in
// order of time; items scheduled for the same time have no defined relative
// order.
func (s *Schedule) Ready(now time.Time) []interface{} {
	var ready []interface{}
	for len(s.items) > 0 && !s.items[0].t.After(now) {
		item := heap.Pop(&s.items).(*scheduleItem)
		delete(s.m, item.key)
		ready = append(ready, item.value)
	}
	return ready
}

// Add adds an item with the specified value, with the corresponding key
// and time to the schedule. Add will panic if there already exists an item
// with the same key.
func (s *Schedule) Add(key, value interface{}, t time.Time) {
	if _, ok := s.m[key]; ok {
		panic(errors.Errorf("duplicate key %v", key))
	}
	item := &scheduleItem{key: key, value: value, t: t}
	s.m[key] = item
	heap.Push(&s.items, item)
}

// Remove removes the item corresponding to the specified key from the
// schedule. If no item with the specified key exists, this is a no-op.
func (s *Schedule) Remove(key interface{}) {
	if item, ok := s.m[key]; ok {
		heap.Remove(&s.items, item.i)
		delete(s.m, key)
	}
}

type scheduleItems []*scheduleItem

type scheduleItem struct {
	i     int
	key   interface{}
	value interface{}
	t     time.Time
}

func (s scheduleItems) Len() int {
	return len(s)
}

func (s scheduleItems) Less(i, j int) bool {
	return s[i].t.Before(s[j].t)
}

func (s scheduleItems) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
	s[i].i = i
	s[j].i = j
}

func (s *scheduleItems) Push(x interface{}) {
	item := x.(*scheduleItem)
	item.i = len(*s)
	*s = append(*s, item)
}

func (s *scheduleItems) Pop() interface{} {
	n := len(*s) - 1
	x := (*s)[n]
	*s = (*s)[:n]
	return x
}
