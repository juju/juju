package authentication_test

import (
	"testing"

	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&AgentAuthenticatorSuite{})

func TestAll(t *testing.T) {
	coretesting.MgoTestPackage(t)
}
