// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrettriggerschedule

import (
	"time"

	"github.com/juju/clock"
)

const (
	// MaxTriggerRetries is the maximum retry count for a trigger before
	// the worker should exit so it can be restarted with fresh watcher
	// state from the controller. Retries run at fixed intervals of the
	// worker's base delay, so with the 5-minute base delays used by the
	// secret workers, retries run every 5 minutes and the restart
	// happens 20 minutes after the initial fire.
	MaxTriggerRetries = 3
)

// TriggerSchedule tracks the scheduled trigger time and retry state
// for a backend token rotation or secret revision expiry.
type TriggerSchedule struct {
	// TriggerTime is the trigger time last reported by the watcher.
	TriggerTime time.Time
	// NextTime is when the trigger should next fire (initially TriggerTime,
	// then fixed-interval retry deadlines after firing).
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
// returns the schedule bumped to its next retry deadline (baseDelay after
// now). The returned schedule is a copy; the original is not modified.
func (s TriggerSchedule) AfterFire(
	logger Logger, kind, target string, baseDelay time.Duration, now time.Time,
) TriggerSchedule {
	if s.Fired {
		if s.RetryCount < MaxTriggerRetries {
			s.RetryCount++
		}
		logger.Debugf(
			"retrying %s (attempt %d/%d) for %s at %s (scheduled %s)",
			kind, s.RetryCount, MaxTriggerRetries, target, now, s.NextTime)
	} else {
		s.Fired = true
		logger.Infof(
			"firing %s for %s at %s (scheduled %s)",
			kind, target, now, s.NextTime)
	}
	s.NextTime = now.Add(baseDelay)
	return s
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
//
// The caller must pass back the current value returned by the previous
// call (the deadline the timer was armed with), not a value derived
// separately; otherwise the unchanged check in isScheduleUnchanged
// misfires and the timer is needlessly reset or left stale.
func ScheduleTimer(
	clk clock.Clock, timer clock.Timer, current, soonest time.Time,
) (clock.Timer, time.Time, bool) {
	deadline, now, stopOnly, unchanged := nextDeadline(clk, timer != nil, current, soonest)
	if unchanged {
		return timer, current, false
	}
	if stopOnly {
		Stop(timer)
		return nil, time.Time{}, true
	}
	nextDuration := deadline.Sub(now)
	if timer == nil {
		return clk.NewTimer(nextDuration), deadline, true
	}
	Stop(timer)
	timer.Reset(nextDuration)
	return timer, deadline, true
}

// ScheduleAlarm arms or resets alarm on clk to fire at soonest (set to
// now if soonest is in the past). current is the currently scheduled trigger
// time, or zero if alarm is nil or has already fired. If the schedule is
// unchanged, alarm and current are returned as-is with changed=false. If
// soonest is zero and alarm is non-nil, alarm is stopped and nil is returned.
//
// The caller must pass back the current value returned by the previous
// call (the deadline the alarm was armed with), not a value derived
// separately; otherwise the unchanged check in isScheduleUnchanged
// misfires and the alarm is needlessly reset or left stale.
func ScheduleAlarm(
	clk clock.Clock, alarm clock.Alarm, current, soonest time.Time,
) (clock.Alarm, time.Time, bool) {
	deadline, _, stopOnly, unchanged := nextDeadline(clk, alarm != nil, current, soonest)
	if unchanged {
		return alarm, current, false
	}
	if stopOnly {
		Stop(alarm)
		return nil, time.Time{}, true
	}
	if alarm == nil {
		return clk.NewAlarm(deadline), deadline, true
	}
	Stop(alarm)
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

// Stop stops the StoppableChannel (a clock.Timer or clock.Alarm) if
// non-nil and drains any buffered tick.
func Stop(timer StoppableChannel) {
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
