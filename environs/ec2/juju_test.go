package ec2

import (
	"fmt"
	"launchpad.net/goamz/aws"
	"launchpad.net/goamz/ec2/ec2test"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/environs"
	"launchpad.net/juju/go/environs/jujutest"
)

var functionalConfig = []byte(`
environments:
  sample:
    type: ec2
    region: test
`)

// jujuTests wraps jujutest.Tests by adding
// set up and tear down functions that start a new
// ec2test server for each test.
// The server is accessed by using the "test" region,
// which is changed to point to the network address
// of the local server.
type jujuTests struct {
	*jujutest.Tests
	srv   *ec2test.Server
	setup func(*ec2test.Server)
}

// Each test is run in each of the following scenarios.
// A scenario is implemented by mutating the ec2test
// server after it starts.
var scenarios = []struct {
	name  string
	setup func(*ec2test.Server)
}{
	{"normal", normalScenario},
	{"initial-state-running", initialStateRunningScenario},
	{"extra-instances", extraInstancesScenario},
}

func normalScenario(*ec2test.Server) {
}

func initialStateRunningScenario(srv *ec2test.Server) {
	srv.SetInitialInstanceState(ec2test.Running)
}

func extraInstancesScenario(srv *ec2test.Server) {
	states := []ec2test.InstanceState{
		ec2test.ShuttingDown,
		ec2test.Terminated,
		ec2test.Stopped,
	}
	for _, state := range states {
		srv.NewInstances(1, "m1.small", "ami-a7f539ce", state, nil)
	}
}

func registerJujuFunctionalTests() {
	Regions["test"] = aws.Region{}
	envs, err := environs.ReadEnvironsBytes(functionalConfig)
	if err != nil {
		panic(fmt.Errorf("cannot parse functional tests config data: %v", err))
	}

	for _, name := range envs.Names() {
		for _, scen := range scenarios {
			scen := scen
			Suite(&jujuTests{
				setup: scen.setup,
				Tests: &jujutest.Tests{
					Environs: envs,
					Name:     name,
				},
			})
		}
	}
}

func (t *jujuTests) SetUpTest(c *C) {
	var err error
	t.srv, err = ec2test.NewServer()
	if err != nil {
		c.Fatalf("cannot start ec2 test server: %v", err)
	}
	Regions["test"] = aws.Region{
		EC2Endpoint: t.srv.Address(),
	}
	t.setup(t.srv)
}

func (t *jujuTests) TearDownTest(c *C) {
	t.Tests.TearDownTest(c)
	t.srv.Quit()
	t.srv = nil
	// Clear out the region because the server address is
	// no longer valid.
	Regions["test"] = aws.Region{}
}

// integrationConfig holds the environments configuration
// for running the amazon EC2 integration tests.
//
// This is missing keys for security reasons; set the following environment variables
// to make the integration testing work:
//  access-key: $AWS_ACCESS_KEY_ID
//  secret-key: $AWS_SECRET_ACCESS_KEY
var integrationConfig = []byte(`
environments:
  sample:
    type: ec2
`)

func registerJujuIntegrationTests() {
	envs, err := environs.ReadEnvironsBytes(integrationConfig)
	if err != nil {
		panic(fmt.Errorf("cannot parse integration tests config data: %v", err))
	}
	for _, name := range envs.Names() {
		Suite(&jujutest.Tests{
			Environs: envs,
			Name:     name,
		})
	}
}
