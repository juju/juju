// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrettriggerschedule

import (
	"time"

	"github.com/juju/clock"
)

const (
	// MaxTriggerRetryDelay is the maximum backoff delay between trigger
	// retries when confirmation from the controller has not arrived.
	MaxTriggerRetryDelay = time.Hour

	// MaxTriggerRetries is the maximum retry count for Fibonacci backoff
	// on a trigger before the worker should resync its watcher state from
	// the controller.
	MaxTriggerRetries = 5
)

// TriggerSchedule tracks the scheduled trigger time and retry backoff state
// for a backend token rotation or secret revision expiry.
type TriggerSchedule struct {
	// TriggerTime is the trigger time last reported by the watcher.
	TriggerTime time.Time
	// NextTime is when the trigger should next fire (initially TriggerTime,
	// then backed-off retry deadlines after firing).
	NextTime time.Time
	// Fired reports whether the initial trigger has already fired.
	Fired bool
	// RetryCount is the number of retry attempts performed after the initial
	// fire (capped at MaxTriggerRetries).
	RetryCount int
}

// NewTriggerSchedule returns a TriggerSchedule initialized for triggerTime.
func NewTriggerSchedule(triggerTime time.Time) TriggerSchedule {
	return TriggerSchedule{
		TriggerTime: triggerTime,
		NextTime:    triggerTime,
	}
}

// RetriesExhausted reports whether MaxTriggerRetries has been reached.
func (s TriggerSchedule) RetriesExhausted() bool {
	return s.RetryCount >= MaxTriggerRetries
}

// ShouldIgnoreTime reports whether an incoming watcher update with a
// nextTriggerTime for an already-tracked target should be ignored because a
// fired trigger is currently awaiting confirmation or retrying with the
// identical trigger time. Strictly later times (normal reschedule) and
// strictly earlier times (operator shortening the schedule) are both accepted.
func (s TriggerSchedule) ShouldIgnoreTime(nextTriggerTime time.Time) bool {
	// While a fired trigger is awaiting confirmation or retrying, ignore
	// duplicate or redelivered events carrying the identical trigger time.
	// Before the first fire (!s.Fired), accept all updates.
	return s.Fired && nextTriggerTime.Equal(s.TriggerTime)
}

// Logger logs trigger firing and retry state transitions.
type Logger interface {
	Debugf(string, ...any)
	Infof(string, ...any)
}

// AfterFire logs the trigger firing (or retry) for kind and target, then
// returns the schedule bumped to its next retry deadline.
func (s TriggerSchedule) AfterFire(
	logger Logger, kind, target string, baseDelay time.Duration, now time.Time,
) TriggerSchedule {
	next := s.Bumped(baseDelay, now)
	if s.Fired {
		logger.Debugf(
			"retrying %s (attempt %d/%d) for %s at %s (scheduled %s)",
			kind, next.RetryCount, MaxTriggerRetries, target, now, s.NextTime)
	} else {
		logger.Infof(
			"firing %s for %s at %s (scheduled %s)",
			kind, target, now, s.NextTime)
	}
	return next
}

// Bumped returns a copy of s with Fired set, RetryCount incremented up to
// MaxTriggerRetries on subsequent fires, and NextTime advanced to the next
// retry deadline from now.
func (s TriggerSchedule) Bumped(baseDelay time.Duration, now time.Time) TriggerSchedule {
	if !s.Fired {
		s.Fired = true
	} else if s.RetryCount < MaxTriggerRetries {
		s.RetryCount++
	}
	s.NextTime = now.Add(TriggerRetryDelay(baseDelay, s.RetryCount))
	return s
}

// TriggerRetryDelay returns the backoff delay for the given retry attempt
// (0 is the first retry after initial firing), scaling baseDelay by the
// Fibonacci sequence (1, 2, 3, 5, 8, ...) up to MaxTriggerRetryDelay.
func TriggerRetryDelay(baseDelay time.Duration, retryCount int) time.Duration {
	if baseDelay >= MaxTriggerRetryDelay {
		return MaxTriggerRetryDelay
	}
	prev, curr := time.Duration(0), baseDelay
	for range retryCount + 1 {
		next := prev + curr
		if next >= MaxTriggerRetryDelay {
			return MaxTriggerRetryDelay
		}
		prev, curr = curr, next
	}
	return curr
}

// SoonestTime returns the earliest non-zero trigger time across entries, or
// the zero time if none are schedulable.
func SoonestTime[T any](entries map[string]T, triggerTime func(T) time.Time) time.Time {
	var soonest time.Time
	for _, entry := range entries {
		t := triggerTime(entry)
		if t.IsZero() {
			continue
		}
		if soonest.IsZero() || t.Before(soonest) {
			soonest = t
		}
	}
	return soonest
}

// ScheduleTimer arms or resets timer on clk to fire at soonest (set to
// now if soonest is in the past). current is the currently scheduled trigger
// time, or zero if timer is nil or has already fired. If the schedule is
// unchanged, timer and current are returned as-is with changed=false. If
// soonest is zero and timer is non-nil, timer is stopped and nil is returned.
func ScheduleTimer(
	clk clock.Clock, timer clock.Timer, current, soonest time.Time,
) (clock.Timer, time.Time, bool) {
	deadline, now, stopOnly, unchanged := nextDeadline(clk, timer != nil, current, soonest)
	if unchanged {
		return timer, current, false
	}
	if stopOnly {
		StopTimer(timer)
		return nil, time.Time{}, true
	}
	nextDuration := deadline.Sub(now)
	if timer == nil {
		return clk.NewTimer(nextDuration), deadline, true
	}
	StopTimer(timer)
	timer.Reset(nextDuration)
	return timer, deadline, true
}

// ScheduleAlarm arms or resets alarm on clk to fire at soonest (set to
// now if soonest is in the past). current is the currently scheduled trigger
// time, or zero if alarm is nil or has already fired. If the schedule is
// unchanged, alarm and current are returned as-is with changed=false. If
// soonest is zero and alarm is non-nil, alarm is stopped and nil is returned.
func ScheduleAlarm(
	clk clock.Clock, alarm clock.Alarm, current, soonest time.Time,
) (clock.Alarm, time.Time, bool) {
	deadline, _, stopOnly, unchanged := nextDeadline(clk, alarm != nil, current, soonest)
	if unchanged {
		return alarm, current, false
	}
	if stopOnly {
		StopTimer(alarm)
		return nil, time.Time{}, true
	}
	if alarm == nil {
		return clk.NewAlarm(deadline), deadline, true
	}
	StopTimer(alarm)
	alarm.Reset(deadline)
	return alarm, deadline, true
}

func nextDeadline(
	clk clock.Clock, isActive bool, current, soonest time.Time,
) (deadline, now time.Time, stopOnly, unchanged bool) {
	if isScheduleUnchanged(isActive, current, soonest) {
		return current, time.Time{}, false, true
	}
	if soonest.IsZero() {
		return time.Time{}, time.Time{}, true, false
	}
	now = clk.Now()
	if soonest.Before(now) {
		soonest = now
	}
	return soonest, now, false, false
}

// isScheduleUnchanged reports whether a timer or alarm needs no update.
func isScheduleUnchanged(isActive bool, current, soonest time.Time) bool {
	if soonest.IsZero() {
		return !isActive
	}
	return isActive && soonest.Equal(current)
}

// StoppableChannel is implemented by clock.Timer and clock.Alarm.
type StoppableChannel interface {
	Chan() <-chan time.Time
	Stop() bool
}

// StopTimer stops timer if non-nil and drains any buffered tick.
func StopTimer(timer StoppableChannel) {
	if timer == nil {
		return
	}
	if !timer.Stop() {
		drainTimer(timer)
	}
}

// drainTimer drains a single pending tick from timer's channel if one is ready
// without blocking. Call after Stop returns false.
func drainTimer(timer StoppableChannel) {
	select {
	case <-timer.Chan():
	default:
	}
}
