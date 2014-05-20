package audit

import (
	"testing"

	"github.com/juju/loggo"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/state"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

type auditSuite struct{}

var _ = gc.Suite(&auditSuite{})

func (*auditSuite) SetUpTest(c *gc.C) {
	loggo.ResetLoggers()
}

func TestAudit(t *testing.T) {
	var u state.User
	Audit(&u, "donut eaten, %v donut(s) remain", 7)
}
