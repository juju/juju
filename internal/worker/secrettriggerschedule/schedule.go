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

	// MaxTriggerRetries is the maximum retry count for exponential backoff
	// on a rotation or expiry trigger. Once reached, the worker logs a
	// warning and continues retrying at MaxTriggerRetryDelay until a watcher
	// update confirms or removes the trigger.
	MaxTriggerRetries = 5
)

// TriggerSchedule tracks the scheduled trigger time and retry backoff state
// for a secret rotation, backend token rotation, or revision expiry.
type TriggerSchedule struct {
	// TriggerTime is the trigger time last reported by the watcher.
	TriggerTime time.Time
	// NextTime is when the trigger should next fire (initially TriggerTime,
	// then backed-off retry deadlines after firing).
	NextTime time.Time
	// RetryCount is the number of retry attempts scheduled since the initial
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

// ShouldIgnoreTime reports whether an incoming watcher update carrying
// nextTriggerTime for an already-tracked target should be ignored because a
// fired trigger is currently retrying with the identical trigger time.
// Strictly later times (normal reschedule) and strictly earlier times
// (operator shortening the schedule) are both accepted.
func (s TriggerSchedule) ShouldIgnoreTime(nextTriggerTime time.Time) bool {
	// While retrying a fired trigger, ignore duplicate or redelivered events
	// carrying the identical trigger time. Before the first fire
	// (RetryCount == 0), accept all updates.
	return s.RetryCount > 0 && nextTriggerTime.Equal(s.TriggerTime)
}

// Logger logs trigger firing and retry state transitions.
type Logger interface {
	Infof(string, ...interface{})
	Warningf(string, ...interface{})
}

// AfterFire logs the trigger firing (or retry) for kind and target, then
// returns the schedule bumped to its next retry deadline.
func AfterFire(
	s TriggerSchedule, logger Logger, kind, target string, baseDelay time.Duration, now time.Time,
) TriggerSchedule {
	if s.RetryCount >= MaxTriggerRetries {
		logger.Warningf(
			"%s for %s has not been confirmed after %d retries; retrying every %s",
			kind, target, s.RetryCount, MaxTriggerRetryDelay)
	} else if s.RetryCount > 0 {
		logger.Warningf(
			"retrying %s (attempt %d/%d) for %s at %s (scheduled %s)",
			kind, s.RetryCount, MaxTriggerRetries, target, now, s.NextTime)
	} else {
		logger.Infof(
			"firing %s for %s at %s (scheduled %s)",
			kind, target, now, s.NextTime)
	}
	return s.Bumped(baseDelay, now)
}

// Bumped returns a copy of s with NextTime advanced to the next retry deadline
// from now and RetryCount incremented up to MaxTriggerRetries.
func (s TriggerSchedule) Bumped(baseDelay time.Duration, now time.Time) TriggerSchedule {
	s.NextTime = now.Add(TriggerRetryDelay(baseDelay, s.RetryCount))
	if s.RetryCount < MaxTriggerRetries {
		s.RetryCount++
	}
	return s
}

// TriggerRetryDelay returns the backoff delay for the given retry attempt
// (0 is the first retry after initial firing), doubling baseDelay on each
// attempt up to MaxTriggerRetryDelay.
func TriggerRetryDelay(baseDelay time.Duration, retryCount int) time.Duration {
	if baseDelay >= MaxTriggerRetryDelay {
		return MaxTriggerRetryDelay
	}
	delay := baseDelay
	for range retryCount {
		delay *= 2
		if delay >= MaxTriggerRetryDelay {
			return MaxTriggerRetryDelay
		}
	}
	return delay
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
) (_ clock.Timer, _ time.Time, _ time.Duration, changed bool) {
	deadline, now, stopOnly, unchanged := nextDeadline(clk, timer != nil, current, soonest)
	if unchanged {
		return timer, current, 0, false
	}
	if stopOnly {
		stopTimer(timer)
		return nil, time.Time{}, 0, true
	}
	nextDuration := deadline.Sub(now)
	if timer == nil {
		return clk.NewTimer(nextDuration), deadline, nextDuration, true
	}
	stopTimer(timer)
	timer.Reset(nextDuration)
	return timer, deadline, nextDuration, true
}

// ScheduleAlarm arms or resets alarm on clk to fire at soonest (set to
// now if soonest is in the past). current is the currently scheduled trigger
// time, or zero if alarm is nil or has already fired. If the schedule is
// unchanged, alarm and current are returned as-is with changed=false. If
// soonest is zero and alarm is non-nil, alarm is stopped and nil is returned.
func ScheduleAlarm(
	clk clock.Clock, alarm clock.Alarm, current, soonest time.Time,
) (_ clock.Alarm, _ time.Time, changed bool) {
	deadline, _, stopOnly, unchanged := nextDeadline(clk, alarm != nil, current, soonest)
	if unchanged {
		return alarm, current, false
	}
	if stopOnly {
		stopTimer(alarm)
		return nil, time.Time{}, true
	}
	if alarm == nil {
		return clk.NewAlarm(deadline), deadline, true
	}
	stopTimer(alarm)
	alarm.Reset(deadline)
	return alarm, deadline, true
}

func nextDeadline(
	clk clock.Clock, active bool, current, soonest time.Time,
) (deadline, now time.Time, stopOnly, unchanged bool) {
	if isScheduleUnchanged(active, current, soonest) {
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
func isScheduleUnchanged(active bool, current, soonest time.Time) bool {
	if soonest.IsZero() {
		return !active
	}
	return active && soonest.Equal(current)
}

// stoppableChannel is implemented by clock.Timer and clock.Alarm.
type stoppableChannel interface {
	Chan() <-chan time.Time
	Stop() bool
}

// stopTimer stops timer if non-nil and drains any buffered tick.
func stopTimer(timer stoppableChannel) {
	if timer == nil {
		return
	}
	if !timer.Stop() {
		drainTimer(timer)
	}
}

// drainTimer drains a single pending tick from timer's channel if one is ready
// without blocking. Call after Stop returns false.
func drainTimer(timer stoppableChannel) {
	select {
	case <-timer.Chan():
	default:
	}
}
