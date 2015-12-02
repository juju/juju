// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"sort"
	"sync"
	"time"

	"github.com/juju/utils/clock"
)

// timerClock exposes the underlying Clock's capabilities to a Timer.
type timerClock interface {
	reset(id int, d time.Duration) bool
	stop(id int) bool
}

// Timer implements a mock clock.Timer for testing purposes.
type Timer struct {
	ID    int
	clock timerClock
}

// Reset is part of the clock.Timer interface.
func (t *Timer) Reset(d time.Duration) bool {
	return t.clock.reset(t.ID, d)
}

// Stop is part of the clock.Timer interface.
func (t *Timer) Stop() bool {
	return t.clock.stop(t.ID)
}

// stoppedTimer is a no-op implementation of clock.Timer.
type stoppedTimer struct{}

// Reset is part of the clock.Timer interface.
func (stoppedTimer) Reset(time.Duration) bool { return false }

// Stop is part of the clock.Timer interface.
func (stoppedTimer) Stop() bool { return false }

// Clock implements a mock clock.Clock for testing purposes.
type Clock struct {
	mu             sync.Mutex
	now            time.Time
	alarms         []alarm
	currentAlarmID int
	notifyAlarms   chan struct{}
}

// NewClock returns a new clock set to the supplied time. If your SUT needs to
// call After, AfterFunc, or Timer.Reset more than 1024 times: (1) you have
// probably written a bad test; and (2) you'll need to read from the Alarms
// chan to keep the buffer clear.
func NewClock(now time.Time) *Clock {
	return &Clock{
		now:          now,
		notifyAlarms: make(chan struct{}, 1024),
	}
}

// Now is part of the clock.Clock interface.
func (clock *Clock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return clock.now
}

// After is part of the clock.Clock interface.
func (clock *Clock) After(d time.Duration) <-chan time.Time {
	defer clock.notifyAlarm()
	clock.mu.Lock()
	defer clock.mu.Unlock()
	notify := make(chan time.Time, 1)
	if d <= 0 {
		notify <- clock.now
	} else {
		clock.setAlarm(clock.now.Add(d), func() { notify <- clock.now })
	}
	return notify
}

// AfterFunc is part of the clock.Clock interface.
func (clock *Clock) AfterFunc(d time.Duration, f func()) clock.Timer {
	defer clock.notifyAlarm()
	clock.mu.Lock()
	defer clock.mu.Unlock()
	if d <= 0 {
		f()
		return &stoppedTimer{}
	}
	id := clock.setAlarm(clock.now.Add(d), f)
	return &Timer{id, clock}
}

// Advance advances the result of Now by the supplied duration, and sends
// the "current" time on all alarms which are no longer "in the future".
func (clock *Clock) Advance(d time.Duration) {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	clock.now = clock.now.Add(d)
	triggered := 0
	for _, alarm := range clock.alarms {
		if clock.now.Before(alarm.time) {
			break
		}
		alarm.trigger()
		triggered++
	}
	clock.alarms = clock.alarms[triggered:]
}

// Alarms returns a channel on which you can read one value for every call to
// After and AfterFunc; and for every successful Timer.Reset backed by this
// Clock. It might not be elegant but it's necessary when testing time logic
// that runs on a goroutine other than that of the test.
func (clock *Clock) Alarms() <-chan struct{} {
	return clock.notifyAlarms
}

// reset is the underlying implementation of clock.Timer.Reset, which may be
// called by any Timer backed by this Clock.
func (clock *Clock) reset(id int, d time.Duration) bool {
	clock.mu.Lock()
	defer clock.mu.Unlock()

	for i, alarm := range clock.alarms {
		if id == alarm.ID {
			defer clock.notifyAlarm()
			clock.alarms[i].time = clock.now.Add(d)
			sort.Sort(byTime(clock.alarms))
			return true
		}
	}
	return false
}

// stop is the underlying implementation of clock.Timer.Reset, which may be
// called by any Timer backed by this Clock.
func (clock *Clock) stop(id int) bool {
	clock.mu.Lock()
	defer clock.mu.Unlock()

	for i, alarm := range clock.alarms {
		if id == alarm.ID {
			clock.alarms = removeFromSlice(clock.alarms, i)
			return true
		}
	}
	return false
}

// setAlarm adds an alarm at time t.
// It also sorts the alarms and increments the current ID by 1.
func (clock *Clock) setAlarm(t time.Time, trigger func()) int {
	alarm := alarm{
		time:    t,
		trigger: trigger,
		ID:      clock.currentAlarmID,
	}
	clock.alarms = append(clock.alarms, alarm)
	sort.Sort(byTime(clock.alarms))
	clock.currentAlarmID = clock.currentAlarmID + 1
	return alarm.ID
}

// notifyAlarm sends a value on the channel exposed by Alarms().
func (clock *Clock) notifyAlarm() {
	select {
	case clock.notifyAlarms <- struct{}{}:
	default:
		panic("alarm notification buffer full")
	}
}

// alarm records the time at which we're expected to execute trigger.
type alarm struct {
	ID      int
	time    time.Time
	trigger func()
}

// byTime is used to sort alarms by time.
type byTime []alarm

func (a byTime) Len() int           { return len(a) }
func (a byTime) Less(i, j int) bool { return a[i].time.Before(a[j].time) }
func (a byTime) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }

// removeFromSlice removes item at the specified index from the slice
// It exists to make the append train clearer
// This doesn't check that index is valid, so the caller needs to check that.
func removeFromSlice(sl []alarm, index int) []alarm {
	return append(sl[:index], sl[index+1:]...)
}
