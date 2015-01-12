package leadership

import (
	"testing"

	gc "gopkg.in/check.v1"
)

func init() {
	// Initialize all suites here.
	gc.Suite(&leadershipSuite{})
	gc.Suite(&settingsSuite{})
}

func Test(t *testing.T) {
	gc.TestingT(t)
}
