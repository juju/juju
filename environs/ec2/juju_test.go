package ec2

import (
	"fmt"
	"launchpad.net/goamz/aws"
	"launchpad.net/goamz/ec2/ec2test"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/environ/jujutest"
	"launchpad.net/juju/go/environs"
)

type jujuLocalTests struct {
	*jujutest.Tests
	srv localServer
}

// jujuLocalLiveTests performs the live test suite, but locally.
type jujuLocalLiveTests struct {
	*jujutest.LiveTests
	srv localServer
}

type localServer struct {
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

var functionalConfig = []byte(`
environments:
  sample:
    type: ec2
    region: test
`)

func registerJujuFunctionalTests() {
	Regions["test"] = aws.Region{}
	envs, err := environs.ReadEnvironsBytes(functionalConfig)
	if err != nil {
		panic(fmt.Errorf("cannot parse functional tests config data: %v", err))
	}

	for _, name := range envs.Names() {
		for _, scen := range scenarios {
			scen := scen
			Suite(&jujuLocalTests{
				srv: localServer{
					setup: scen.setup,
				},
				Tests: &jujutest.Tests{
					Environs: envs,
					Name:     name,
				},
			})
			Suite(&jujuLocalLiveTests{
				srv: localServer{
					setup: scen.setup,
				},
				LiveTests: &jujutest.LiveTests{
					Environs: envs,
					Name:     name,
				},
			})
		}
	}
}

func (t *jujuLocalTests) SetUpTest(c *C) {
	if t, ok := interface{}(t.Tests).(interface{SetUpTest(*C)}); ok {
		t.SetUpTest(c)
	}
	t.srv.startServer(c)
}

func (t *jujuLocalTests) TearDownTest(c *C) {
	if t, ok := interface{}(t.Tests).(interface{TearDownTest(*C)}); ok {
		t.TearDownTest(c)
	}
	t.srv.stopServer(c)
}

func (t *jujuLocalLiveTests) SetUpTest(c *C) {
	if t, ok := interface{}(t.LiveTests).(interface{SetUpTest(*C)}); ok {
		t.SetUpTest(c)
	}
	t.srv.startServer(c)
}

func (t *jujuLocalLiveTests) TearDownTest(c *C) {
	if t, ok := interface{}(t.LiveTests).(interface{TearDownTest(*C)}); ok {
		t.TearDownTest(c)
	}
	t.srv.stopServer(c)
}

func (srv *localServer) startServer(c *C) {
	var err error
	srv.srv, err = ec2test.NewServer()
	if err != nil {
		c.Fatalf("cannot start ec2 test server: %v", err)
	}
	Regions["test"] = aws.Region{
		EC2Endpoint: srv.srv.Address(),
	}
	srv.setup(srv.srv)
}

func (srv *localServer) stopServer(c *C) {
	srv.srv.Quit()
	Regions["test"] = aws.Region{}
}
