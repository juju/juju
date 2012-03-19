package testing
import (
	"launchpad.net/gocheck"
	"launchpad.net/juju/go/log"
)

// LoggingSuite redirects the juju logger to the test logger
// when embedded in a gocheck suite type.
type LoggingSuite struct {
	oldTarget log.Logger
}

func (t *LoggingSuite) SetUpTest(c *gocheck.C) {
	t.oldTarget = log.Target
	log.Target = c
}

func (t *LoggingSuite) TearDownTest(c *gocheck.C) {
	log.Target = t.oldTarget
}
