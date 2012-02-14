package ec2

import (
	"log"
	"launchpad.net/goamz/ec2"
	"launchpad.net/juju/go/schema"
	"sync"
	"time"
"local/runtime/debug"
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

// parallel represents a number of functions running concurrently.
type parallel struct {
	n        int
	max      int
	work     chan func() error
	done     chan error
	firstErr chan error
	wg       sync.WaitGroup
}

// newParallel returns a new parallel instance.  It will run up to maxPar
// functions concurrently.
func newParallel(maxPar int) *parallel {
	p := &parallel{
		max:      maxPar,
		work:     make(chan func() error),
		done:     make(chan error),
		firstErr: make(chan error),
	}
	// gather the errors from all dos and produce the first
	// one only.
	// TODO decide what to do with the other errors
	go func() {
		var err error
		for e := range p.done {
			if err == nil {
				err = e
			}
		}
		p.firstErr <- err
	}()
	return p
}

// do requests that p run f concurrently.  If there are already the maximum
// number of functions running concurrently, it will block until one of
// them has completed.
func (p *parallel) do(f func() error) {
	if p.n < p.max {
		p.wg.Add(1)
		go func() {
			for f := range p.work {
				p.done <- f()
			}
			p.wg.Done()
		}()
	}
	p.work <- f
	p.n++
}

// wait marks the parallel instance as complete and waits for all the
// functions to complete.  It returns an error from some function that has
// encountered one, discarding other errors.
func (p *parallel) wait() error {
	close(p.work)
	p.wg.Wait()
	close(p.done)
	return <-p.firstErr
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
	burstTotal time.Duration		// total duration of burst.
	burstDelay time.Duration		// interval between each try in the burst.

	longTotal time.Duration		// total duration for attempt after burst.
	longDelay time.Duration		// interval between each try after initial burst.
}

// do calls the given function until it succeeds or returns an error such that
// isTransient is false or the attempt times out.
func (a attempt) do(isTransient func(error) bool, f func() error) (err error) {
	log.Printf("attempt, callers %s", debug.Callers(1, 3))
	start := time.Now()
	i := 0
	// try returns true if do should return.
	try := func(end time.Time, delay time.Duration) bool {
		for time.Now().Before(end) {
			i++
			err = f()
			if err == nil || !isTransient(err) {
				return true
			}
			time.Sleep(delay)
		}
		return false
	}
	if try(start.Add(a.burstTotal), a.burstDelay) {
		if i > 0 {
			log.Printf("after attempt %d, err %v", i, err)
		}
		return
	}
	if try(start.Add(a.burstTotal + a.longTotal), a.longDelay) {
		if i > 0 {
			log.Printf("after attempt %d, err %v", i, err)
		}
		return
	}
	log.Printf("attempt time (%d attempts, %gs) exceeded (err %v)", i, (a.burstTotal + a.longTotal).Seconds(), err)
	return
}

// hasCode returns a function that returns true if the provided error has
// the given ec2 error code.
func hasCode(code string) func(error) bool {
	return func(err error) bool {
		ec2err, _ := err.(*ec2.Error)
		return ec2err != nil && ec2err.Code == code
	}
}
