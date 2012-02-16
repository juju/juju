package ec2

import (
	"launchpad.net/juju/go/schema"
"log"
	"time"
)

// this stuff could/should be in the schema package.

// checkerFunc defines a schema.Checker using a function that
// implements scheme.Checker.Coerce.
type checkerFunc func(v interface{}, path []string) (newv interface{}, err error)

func (f checkerFunc) Coerce(v interface{}, path []string) (newv interface{}, err error) {
	return f(v, path)
}

// combineCheckers returns a Checker that checks a value by passing it
// through the "pipeline" defined by checkers. When the returned checker's
// Coerce method is called on a value, the value is passed through the
// first checker in checkers; the resulting value is used as input to the
// next checker, and so on.
func combineCheckers(checkers ...schema.Checker) schema.Checker {
	f := func(v interface{}, path []string) (newv interface{}, err error) {
		for _, c := range checkers {
			v, err = c.Coerce(v, path)
			if err != nil {
				return nil, err
			}
		}
		return v, nil
	}
	return checkerFunc(f)
}

// oneOf(a, b, c) is equivalent to (but less verbose than):
// schema.OneOf(schema.Const(a), schema.Const(b), schema.Const(c))
func oneOf(values ...interface{}) schema.Checker {
	c := make([]schema.Checker, len(values))
	for i, v := range values {
		c[i] = schema.Const(v)
	}
	return schema.OneOf(c...)
}

// attempt represents a strategy for waiting for an ec2 request to complete
// successfully. A request may fail to due "eventual consistency" semantics,
// which should resolve fairly quickly. A request may also fail due to
// a slow state transition (for instance an instance taking a while to
// release a security group after termination).
// 
// An attempt covers both angles by sending an initial burst of attempts
// at relatively high frequency, then polling at low frequency until some
// total time is reached.
// 
type attempt struct {
	burstTotal time.Duration // total duration of burst.
	burstDelay time.Duration // interval between each try in the burst.

	longTotal time.Duration // total duration for attempt after burst.
	longDelay time.Duration // interval between each try after initial burst.
}

// do calls the given function until it succeeds or returns an error such that
// isTransient is false or the attempt times out.
func (a attempt) do(isTransient func(error) bool, f func() error) (err error) {
	start := time.Now()
	// try returns true if do should return.
	try := func(end time.Time, delay time.Duration) bool {
		for time.Now().Before(end) {
			err = f()
			if err == nil || !isTransient(err) {
				return true
			}
			time.Sleep(delay)
		}
		return false
	}
	if try(start.Add(a.burstTotal), a.burstDelay) {
		return
	}
	if try(start.Add(a.burstTotal+a.longTotal), a.longDelay) {
		return
	}
	log.Printf("time out after %v", time.Now().Sub(start))
	return
}

// hasCode returns a function that returns true if the provided error has
// the given ec2 error code.
func hasCode(code string) func(error) bool {
	return func(err error) bool {
		return ec2ErrCode(err) == code
	}
}
