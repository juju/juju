package testing

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/log"
)

// LoggingSuite redirects the juju logger to the test logger
// when embedded in a gocheck suite type.
type LoggingSuite struct {
	oldTarget log.Logger
	oldDebug  bool
}

func (t *LoggingSuite) SetUpSuite(c *C)    {}
func (t *LoggingSuite) TearDownSuite(c *C) {}

func (t *LoggingSuite) SetUpTest(c *C) {
	t.oldTarget = log.Target()
	t.oldDebug = log.Debug
	log.Debug = true
	log.SetTarget(c)
}

func (t *LoggingSuite) TearDownTest(c *C) {
	log.SetTarget(t.oldTarget)
	log.Debug = t.oldDebug
}
