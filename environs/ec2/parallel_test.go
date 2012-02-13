package ec2
import (
	. "launchpad.net/gocheck"
	"sync"
	"time"
)

type parSuite struct {}

var _ = Suite(parSuite{})

func (parSuite) TestParallelMaxPar(c *C) {
	var mu sync.Mutex
	max := 0
	n := 0
	p := newParallel(3)
	for i := 0; i < 10; i++ {
		p.do(func()error {
			mu.Lock()
			n++
			if n > max {
				max = n
			}
			mu.Unlock()
			time.Sleep(0.1e9)
			mu.Lock()
			n--
			mu.Unlock()
			return nil
		})
	}
	err := p.wait()
	c.Check(n, Equals, 0)
	c.Check(err, IsNil)
	c.Check(max, Equals, 3)
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
