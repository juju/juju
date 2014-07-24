package authentication_test

import (
	"testing"

	gc "launchpad.net/gocheck"

	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&AgentAuthenticatorSuite{})

func TestAll(t *testing.T) {
	coretesting.MgoTestPackage(t)
}
