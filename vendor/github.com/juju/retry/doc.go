// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

// The retry package encapsulates the mechanism around retrying commands.
//
// The simple use is to call retry.Call with a function closure.
//
// ```go
//	err := retry.Call(retry.CallArgs{
//		Func:     func() error { ... },
//		Attempts: 5,
//		Delay:    time.Minute,
//		Clock:    clock.WallClock,
//	})
// ```
//
// The bare minimum arguments that need to be specified are:
// * Func - the function to call
// * Attempts - the number of times to try Func before giving up, or a negative number for unlimited attempts (`retry.UnlimitedAttempts`)
// * Delay - how long to wait between each try that returns an error
// * Clock - either the wall clock, or some testing clock
//
// Any error that is returned from the `Func` is considered transient.
// In order to identify some errors as fatal, pass in a function for the
// `IsFatalError` CallArgs value.
//
// In order to have the `Delay` change for each iteration, a `BackoffFunc`
// needs to be set on the CallArgs. A simple doubling delay function is
// provided by `DoubleDelay`.
//
// An example of a more complex `BackoffFunc` could be a stepped function such
// as:
//
// ```go
//	func StepDelay(last time.Duration, attempt int) time.Duration {
//		switch attempt{
//		case 1:
//			return time.Second
//		case 2:
//			return 5 * time.Second
//		case 3:
//			return 20 * time.Second
//		case 4:
//			return time.Minute
//		case 5:
//			return 5 * time.Minute
//		default:
//			return 2 * last
//		}
//	}
// ```
//
// Consider some package `foo` that has a `TryAgainError`, which looks something
// like this:
// ```go
//	type TryAgainError struct {
//		After time.Duration
//	}
// ```
// and we create something that looks like this:
//
// ```go
//	type TryAgainHelper struct {
//		next time.Duration
//	}
//
//	func (h *TryAgainHelper) notify(lastError error, attempt int) {
//		if tryAgain, ok := lastError.(*foo.TryAgainError); ok {
//			h.next = tryAgain.After
//		} else {
//			h.next = 0
//		}
//	}
//
//	func (h *TryAgainHelper) next(last time.Duration) time.Duration {
//		if h.next != 0 {
//			return h.next
//		}
//		return last
//	}
// ```
//
// Then we could do this:
// ```go
//	helper := TryAgainHelper{}
//	retry.Call(retry.CallArgs{
//		Func: func() error {
//			return foo.SomeFunc()
//		},
//		NotifyFunc:  helper.notify,
//		BackoffFunc: helper.next,
//		Attempts:    20,
//		Delay:       100 * time.Millisecond,
//		Clock:       clock.WallClock,
//	})
// ```
package retry
