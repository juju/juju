package testing

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/log"
)

// LoggingSuite redirects the juju logger to the test logger
// when embedded in a gocheck suite type.
type LoggingSuite struct {
	restoreLog func()
}

func (t *LoggingSuite) SetUpSuite(c *C)    {}
func (t *LoggingSuite) TearDownSuite(c *C) {}

func (t *LoggingSuite) SetUpTest(c *C) {
	target, debug := log.SetTarget(c), log.Debug
	t.restoreLog = func() {
		log.SetTarget(target)
		log.Debug = debug
	}
	log.Debug = true
}

func (t *LoggingSuite) TearDownTest(c *C) {
	t.restoreLog()
}
