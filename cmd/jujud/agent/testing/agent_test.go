package testing

import (
	coretesting "github.com/juju/juju/testing"
	gc "gopkg.in/check.v1"
	"testing"
)

var (
	_ = gc.Suite(&apiOpenSuite{})
)

// Reentrancy point for testing (something as close as possible to) the jujud
// tool itself.
func Test(t *testing.T) {
	coretesting.MgoTestPackage(t)
	gc.TestingT(t)
}
