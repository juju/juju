package ec2

import (
	"launchpad.net/juju/go/schema"
	"sync"
)

// this stuff could/should be in the schema package.

// checkerFunc defines a schema.Checker using a function that
// implemenets scheme.Checker.Coerce.
type checkerFunc func(v interface{}, path []string) (newv interface{}, err error)

func (f checkerFunc) Coerce(v interface{}, path []string) (newv interface{}, err error) {
	return f(v, path)
}

// combineCheckers returns a Checker that checks a value by passing
// it through the "pipeline" defined by checkers. When
// the returned checker's Coerce method is called on a value,
// the value is passed through the first checker in checkers;
// the resulting value is used as input to the next checker, and so on.
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
	n int
	max int
	work chan func() error
	done chan error
	firstErr chan error
	wg sync.WaitGroup
}

// newParallel returns a new parallel instance.
// It will run up to maxPar functions concurrently.
func newParallel(maxPar int) *parallel {
	p := &parallel{
		max: maxPar,
		work: make(chan func() error),
		done: make(chan error),
		firstErr: make(chan error),
	}
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

// do requests that p run f concurrently.
// If there are already the maximum number
// of functions running concurrently, it
// will block until one of them has completed.
func (p *parallel) do(f func() error) {
	if p.n < p.max {
		p.wg.Add(1)
		go func(){
			for f := range p.work {
				p.done <- f()
			}
			p.wg.Done()
		}()
	}
	p.work <- f
	p.n++
}

// wait marks the parallel instance as complete
// and waits for all the functions to complete.
// It returns an error from some function that
// has encountered one, discarding other errors.
func (p *parallel) wait() error {
	close(p.work)
	p.wg.Wait()
	close(p.done)
	return <-p.firstErr
}
