package singular_test

import (
	"fmt"
	"sync"
	"testing"

	gc "launchpad.net/gocheck"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/worker"
	"launchpad.net/juju-core/worker/singular"
)

var _ = gc.Suite(&singularSuite{})

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type singularSuite struct {
	testbase.LoggingSuite
}

func (*singularSuite) TestWithMasterError(c *gc.C) {
	expectErr := fmt.Errorf("an error")
	conn := &fakeConn{
		isMasterErr: expectErr,
	}
	r, err := singular.New(newRunner(), conn)
	c.Check(err, gc.ErrorMatches, "cannot get master status: an error")
	c.Check(r, gc.IsNil)
}

func newRunner() worker.Runner {
	return worker.NewRunner(
		func(error) bool { return true },
		func(err0, err1 error) bool { return true },
	)
}

type fakeConn struct {
	isMaster    bool
	isMasterErr error

	mu      sync.Mutex
	pingErr error
}

func (c *fakeConn) IsMaster() (bool, error) {
	return c.isMaster, c.isMasterErr
}

func (c *fakeConn) Ping() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pingErr
}

func (c *fakeConn) setPingErr(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pingErr = err
}
