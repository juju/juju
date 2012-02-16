package ec2

import (
	. "launchpad.net/gocheck"
	"sync"
	"time"
)

type parSuite struct{}

var _ = Suite(parSuite{})

func (parSuite) testDoneAndMax(c *C, total, maxProc int) {
	c.Logf("total %d, maxProc %d", total, maxProc)
	var mu sync.Mutex
	max := 0
	done := make([]bool, total)
	n := 0
	p := newParallel(maxProc)
	for i := 0; i < total; i++ {
		i := i
		p.do(func() error {
			mu.Lock()
			n++
			if n > max {
				max = n
			}
			mu.Unlock()

			time.Sleep(0.1e9)

			mu.Lock()
			n--
			done[i] = true
			mu.Unlock()
			return nil
		})
	}
	err := p.wait()
	for i, ok := range done {
		if !ok {
			c.Errorf("parallel task %d was not done", i)
		}
	}
	c.Check(n, Equals, 0)
	c.Check(err, IsNil)
	if maxProc < total {
		c.Check(max, Equals, maxProc)
	} else {
		c.Check(max, Equals, total)
	}
}

func (p parSuite) TestParallelMaxPar(c *C) {
	p.testDoneAndMax(c, 10, 3)
	p.testDoneAndMax(c, 3, 10)
}

type intError int

func (intError) Error() string {
	return "error"
}

func (parSuite) TestParallelError(c *C) {
	p := newParallel(6)
	for i := 0; i < 10; i++ {
		i := i
		if i > 5 {
			p.do(func() error {
				return intError(i)
			})
		} else {
			p.do(func() error {
				return nil
			})
		}
	}
	err := p.wait()
	c.Check(err, NotNil)
	if err.(intError) <= 5 {
		c.Errorf("unexpected error: %v", err)
	}
}
