package ec2

import (
	"launchpad.net/juju/go/schema"
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
type attemptStrategy struct {
	total time.Duration		// total duration of attempt.
	delay time.Duration		// interval between each try in the burst.
}

type attempt struct {
	attemptStrategy
	end time.Time
}

func (a attemptStrategy) start() *attempt {
	return &attempt{
		attemptStrategy: a,
	}
}

func (a *attempt) next() bool {
	now := time.Now()
	// we always make at least one attempt.
	if a.end.IsZero() {
		a.end = now.Add(a.total)
		return true
	}

	if !now.Add(a.delay).Before(a.end) {
		return false
	}
	time.Sleep(a.delay)
	return true
}
