// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package retry

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/clock"
)

const (
	// UnlimitedAttempts can be used as a value for `Attempts` to clearly
	// show to the reader that there is no limit to the number of attempts.
	UnlimitedAttempts = -1
)

// retryStopped is the error that is returned from the `Call` function
// when the stop channel has been closed.
type retryStopped struct {
	lastError error
}

// Error provides the implementation for the error interface method.
func (e *retryStopped) Error() string {
	return fmt.Sprintf("retry stopped")
}

// attemptsExceeded is the error that is returned when the retry count has
// been hit without the function returning a nil error result. The last error
// returned from the function being retried is available as the LastError
// attribute.
type attemptsExceeded struct {
	lastError error
}

// Error provides the implementation for the error interface method.
func (e *attemptsExceeded) Error() string {
	return fmt.Sprintf("attempt count exceeded: %s", e.lastError)
}

// durationExceeded is the error that is returned when the total time that the
// `Call` function would have executed exceeds the `MaxDuration` specified.
// The last error returned from the function being retried is available as the
// LastError attribute.
type durationExceeded struct {
	lastError error
}

// Error provides the implementation for the error interface method.
func (e *durationExceeded) Error() string {
	return fmt.Sprintf("max duration exceeded: %s", e.lastError)
}

// LastError retrieves the last error returned from `Func` before iteration
// was terminated due to the attempt count being exceeded, the maximum
// duration being exceeded, or the stop channel being closed.
func LastError(err error) error {
	cause := errors.Cause(err)
	switch err := cause.(type) {
	case *attemptsExceeded:
		return err.lastError
	case *retryStopped:
		return err.lastError
	case *durationExceeded:
		return err.lastError
	}
	return errors.Errorf("unexpected error type: %T, %s", cause, cause)
}

// IsAttemptsExceeded returns true if the error is the result of the `Call`
// function finishing due to hitting the requested number of `Attempts`.
func IsAttemptsExceeded(err error) bool {
	cause := errors.Cause(err)
	_, ok := cause.(*attemptsExceeded)
	return ok
}

// IsDurationExceeded returns true if the error is the result of the `Call`
// function finishing due to the total duration exceeding the specified
// `MaxDuration` value.
func IsDurationExceeded(err error) bool {
	cause := errors.Cause(err)
	_, ok := cause.(*durationExceeded)
	return ok
}

// IsRetryStopped returns true if the error is the result of the `Call`
// function finishing due to the stop channel being closed.
func IsRetryStopped(err error) bool {
	cause := errors.Cause(err)
	_, ok := cause.(*retryStopped)
	return ok
}

// CallArgs is a simple structure used to define the behaviour of the Call
// function.
type CallArgs struct {
	// Func is the function that will be retried if it returns an error result.
	Func func() error

	// IsFatalError is a function that, if set, will be called for every non-
	// nil error result from `Func`. If `IsFatalError` returns true, the error
	// is immediately returned breaking out from any further retries.
	IsFatalError func(error) bool

	// NotifyFunc is a function that is called if Func fails, and the attempt
	// number. The first time this function is called attempt is 1, the second
	// time, attempt is 2 and so on.
	NotifyFunc func(lastError error, attempt int)

	// Attempts specifies the number of times Func should be retried before
	// giving up and returning the `AttemptsExceeded` error. If a negative
	// value is specified, the `Call` will retry forever.
	Attempts int

	// Delay specifies how long to wait between retries.
	Delay time.Duration

	// MaxDelay specifies how longest time to wait between retries. If no
	// value is specified there is no maximum delay.
	MaxDelay time.Duration

	// MaxDuration specifies the maximum time the `Call` function should spend
	// iterating over `Func`. The duration is calculated from the start of the
	// `Call` function.  If the next delay time would take the total duration
	// of the call over MaxDuration, then a DurationExceeded error is
	// returned. If no value is specified, Call will continue until the number
	// of attempts is complete.
	MaxDuration time.Duration

	// BackoffFunc allows the caller to provide a function that alters the
	// delay each time through the loop. If this function is not provided the
	// delay is the same each iteration. Alternatively a function such as
	// `retry.DoubleDelay` can be used that will provide an exponential
	// backoff. The first time this function is called attempt is 1, the
	// second time, attempt is 2 and so on.
	BackoffFunc func(delay time.Duration, attempt int) time.Duration

	// Clock provides the mechanism for waiting. Normal program execution is
	// expected to use something like clock.WallClock, and tests can override
	// this to not actually sleep in tests.
	Clock clock.Clock

	// Stop is a channel that can be used to indicate that the waiting should
	// be interrupted. If Stop is nil, then the Call function cannot be interrupted.
	// If the channel is closed prior to the Call function being executed, the
	// Func is still attempted once.
	Stop <-chan struct{}
}

// Validate the values are valid. The ensures that the Func, Delay, Attempts
// and Clock have been specified.
func (args *CallArgs) Validate() error {
	if args.Func == nil {
		return errors.NotValidf("missing Func")
	}
	if args.Delay == 0 {
		return errors.NotValidf("missing Delay")
	}
	if args.Clock == nil {
		return errors.NotValidf("missing Clock")
	}
	// One of Attempts or MaxDuration need to be specified
	if args.Attempts == 0 && args.MaxDuration == 0 {
		return errors.NotValidf("missing Attempts or MaxDuration")
	}
	return nil
}

// Call will repeatedly execute the Func until either the function returns no
// error, the retry count is exceeded or the stop channel is closed.
func Call(args CallArgs) error {
	err := args.Validate()
	if err != nil {
		return errors.Trace(err)
	}
	start := args.Clock.Now()
	for i := 1; args.Attempts <= 0 || i <= args.Attempts; i++ {
		err = args.Func()
		if err == nil {
			return nil
		}
		if args.IsFatalError != nil && args.IsFatalError(err) {
			return errors.Trace(err)
		}
		if args.NotifyFunc != nil {
			args.NotifyFunc(err, i)
		}
		if i == args.Attempts && args.Attempts > 0 {
			break // don't wait before returning the error
		}

		if args.BackoffFunc != nil {
			delay := args.BackoffFunc(args.Delay, i)
			if delay > args.MaxDelay && args.MaxDelay > 0 {
				delay = args.MaxDelay
			}
			args.Delay = delay
		}
		elapsedTime := args.Clock.Now().Sub(start)
		if args.MaxDuration > 0 && (elapsedTime+args.Delay) > args.MaxDuration {
			return errors.Wrap(err, &durationExceeded{err})
		}

		// Wait for the delay, and retry
		select {
		case <-args.Clock.After(args.Delay):
		case <-args.Stop:
			return errors.Wrap(err, &retryStopped{err})
		}
	}
	return errors.Wrap(err, &attemptsExceeded{err})
}

// DoubleDelay provides a simple function that doubles the duration passed in.
// This can then be easily used as the `BackoffFunc` in the `CallArgs`
// structure.
func DoubleDelay(delay time.Duration, attempt int) time.Duration {
	if attempt == 1 {
		return delay
	}
	return delay * 2
}
