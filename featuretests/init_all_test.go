package feature_tests

import (
	"testing"

	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
)

func Test(t *testing.T) {
	coretesting.MgoTestPackage(t)

	// Initialize all suites here.
	gc.Suite(&leadershipSuite{})
}
