// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"sort"
	"sync"
	"time"

	"github.com/juju/utils/clock"
)

// Clock implements a mock clock.Clock for testing purposes.
type Clock struct {
	mu             sync.Mutex
	now            time.Time
	alarms         []alarm
	currentAlarmID int
}

// Timer implements a mock clock.Timer for testing purposes.
// It's Reset and Stop functions might alter the contents
// of clock by changing the alarm with id ID.
type Timer struct {
	clock *Clock
	ID    int
}

// Reset is part of the clock.Timer interface
func (t *Timer) Reset(d time.Duration) bool {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()

	for i, alarm := range t.clock.alarms {
		if t.ID == alarm.ID {
			t.clock.alarms[i].time = t.clock.now.Add(d)
			sort.Sort(byTime(t.clock.alarms))
			return true
		}
	}
	return false
}

// Stop is part of the clock.Timer interface
func (t *Timer) Stop() bool {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()

	for i, alarm := range t.clock.alarms {
		if t.ID == alarm.ID {
			t.clock.alarms = removeFromSlice(t.clock.alarms, i)
			return true
		}
	}
	return false
}

// NewClock returns a new clock set to the supplied time.
func NewClock(now time.Time) *Clock {
	return &Clock{now: now}
}

// Now is part of the clock.Clock interface.
func (clock *Clock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return clock.now
}

// After is part of the clock.Clock interface.
func (clock *Clock) After(d time.Duration) <-chan time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	notify := make(chan time.Time, 1)
	if d <= 0 {
		notify <- clock.now
	} else {
		clock.setAlarm(clock.now.Add(d), func() { notify <- clock.now })
		clock.currentAlarmID = clock.currentAlarmID + 1
	}
	return notify
}

// AfterFunc is part of the clock.Clock interface.
func (clock *Clock) AfterFunc(d time.Duration, f func()) clock.Timer {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	if d <= 0 {
		f()
	} else {
		clock.setAlarm(clock.now.Add(d), f)
	}

	// If d <= we're sending an id that's not attached to any alarm
	// which is what's intended
	clock.currentAlarmID = clock.currentAlarmID + 1
	t := &Timer{clock, clock.currentAlarmID}
	return t
}

func (clock *Clock) setAlarm(t time.Time, trigger func()) {
	alarm := alarm{
		time:    t,
		trigger: trigger,
		ID:      clock.currentAlarmID,
	}
	clock.alarms = append(clock.alarms, alarm)
	sort.Sort(byTime(clock.alarms))
}

// Advance advances the result of Now by the supplied duration, and sends
// the "current" time on all alarms which are no longer "in the future".
func (clock *Clock) Advance(d time.Duration) {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	clock.now = clock.now.Add(d)
	rung := 0
	for _, alarm := range clock.alarms {
		if clock.now.Before(alarm.time) {
			break
		}
		alarm.trigger()
		rung++
	}
	clock.alarms = clock.alarms[rung:]
}

// alarm records the time at which we're expected to execute trigger.
type alarm struct {
	time    time.Time
	trigger func()
	ID      int
}

// removeFromSlice removes item at the specified index from the slice
// It exists to make the append train clearer
// This doesn't check that index is valid, so the caller needs to check that.
func removeFromSlice(sl []alarm, index int) []alarm {
	return append(sl[:index], sl[index+1:]...)
}

// byTime is used to sort alarms by time.
type byTime []alarm

func (a byTime) Len() int           { return len(a) }
func (a byTime) Less(i, j int) bool { return a[i].time.Before(a[j].time) }
func (a byTime) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
