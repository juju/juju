package ec2

import (
	"fmt"
	"launchpad.net/goamz/aws"
	"launchpad.net/goamz/ec2/ec2test"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/juju"
	"launchpad.net/juju/go/juju/jujutest"
)

var functionalConfig = []byte(`
environments:
  sample:
    type: ec2
    region: test
`)

type jujuTests struct {
	*jujutest.Tests
	srv   *ec2test.Server
	setup func(*ec2test.Server)
}

var scenarios = []struct {
	name  string
	setup func(*ec2test.Server)
}{
	{
		"normal", func(*ec2test.Server) {},
	}, {
		"initial-state-running", func(srv *ec2test.Server) {
			srv.SetInitialInstanceState(ec2test.Running)
		},
	}, {
		"other-instances", func(srv *ec2test.Server) {
			for _, state := range []ec2test.InstanceState{ec2test.ShuttingDown, ec2test.Terminated, ec2test.Stopped} {
				srv.NewInstances(1, "m1.small", "ami-a7f539ce", state, nil)
			}
		},
	},
}

func registerJujuFunctionalTests() {
	Regions["test"] = aws.Region{}
	envs, err := juju.ReadEnvironsBytes(functionalConfig)
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
	Regions["test"] = aws.Region{}
}

// integration_test_environments holds the environments configuration
// for running the amazon EC2 integration tests.
//
// This is missing keys for security reasons; set the following environment variables
// to make the integration testing work:
//  access-key: $AWS_ACCESS_KEY_ID
//  admin-secret: $AWS_SECRET_ACCESS_KEY
var integrationConfig = []byte(`
environments:
  sample:
    type: ec2
`)

func registerJujuIntegrationTests() {
	envs, err := juju.ReadEnvironsBytes(integrationConfig)
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
