package testing_test
import (
	. "launchpad.net/gocheck"
	_ "launchpad.net/juju/go/testing"
	"launchpad.net/juju/go/environs"
	"launchpad.net/juju/go/environs/jujutest"
	stdtesting "testing"
	"fmt"
)

func init() {
	config := `
environments:
    only:
        type: testing
        basename: foo
`
	envs, err := environs.ReadEnvironsBytes([]byte(config))
	if err != nil {
		panic(fmt.Errorf("cannot parse testing config: %v", err))
	}
	Suite(jujutest.LiveTests{
		Environs: envs,
		Name: "only",
	})
	Suite(jujutest.Tests{
		Environs: envs,
		Name: "only",
	})
}

func TestSuite(t *stdtesting.T) {
	TestingT(t)
}
