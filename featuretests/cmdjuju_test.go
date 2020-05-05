// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"github.com/juju/charm/v7"
	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/cmd/juju/application"
	"github.com/juju/juju/cmd/juju/model"
	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	coremodel "github.com/juju/juju/core/model"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

// cmdJujuSuite tests the connectivity of juju commands.  These tests
// go from the command line, api client, api server, db. The db changes
// are then checked.  Only one test for each command is done here to
// check connectivity.  Exhaustive unit tests are at each layer.
type cmdJujuSuite struct {
	jujutesting.JujuConnSuite
}

func uint64p(val uint64) *uint64 {
	return &val
}

func (s *cmdJujuSuite) SetUpSuite(c *gc.C) {
	s.JujuConnSuite.SetUpSuite(c)
}

func (s *cmdJujuSuite) TestSetConstraints(c *gc.C) {
	_, err := cmdtesting.RunCommand(c, model.NewModelSetConstraintsCommand(), "mem=4G", "cpu-power=250")
	c.Assert(err, jc.ErrorIsNil)

	cons, err := s.State.ModelConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons, gc.DeepEquals, constraints.Value{
		CpuPower: uint64p(250),
		Mem:      uint64p(4096),
	})
}

func (s *cmdJujuSuite) TestGetConstraints(c *gc.C) {
	app := s.AddTestingApplication(c, "app", s.AddTestingCharm(c, "dummy"))
	err := app.SetConstraints(constraints.Value{CpuCores: uint64p(64)})
	c.Assert(err, jc.ErrorIsNil)

	context, err := cmdtesting.RunCommand(c, application.NewApplicationGetConstraintsCommand(), "app")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), gc.Equals, "cores=64\n")
	c.Assert(cmdtesting.Stderr(context), gc.Equals, "")
}

func (s *cmdJujuSuite) combinedSettings(ch *state.Charm, inSettings charm.Settings) charm.Settings {
	result := ch.Config().DefaultSettings()
	for name, value := range inSettings {
		result[name] = value
	}
	return result
}

func (s *cmdJujuSuite) TestApplicationSet(c *gc.C) {
	ch := s.AddTestingCharm(c, "dummy")
	app := s.AddTestingApplication(c, "dummy-application", ch)

	_, err := cmdtesting.RunCommand(c, application.NewConfigCommand(), "dummy-application",
		"username=hello", "outlook=hello@world.tld")
	c.Assert(err, jc.ErrorIsNil)

	expect := charm.Settings{
		"username": "hello",
		"outlook":  "hello@world.tld",
	}

	settings, err := app.CharmConfig(coremodel.GenerationMaster)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, s.combinedSettings(ch, expect))
}

func (s *cmdJujuSuite) TestApplicationUnset(c *gc.C) {
	ch := s.AddTestingCharm(c, "dummy")
	app := s.AddTestingApplication(c, "dummy-application", ch)

	settings := charm.Settings{
		"username": "hello",
		"outlook":  "hello@world.tld",
	}

	err := app.UpdateCharmConfig(coremodel.GenerationMaster, settings)
	c.Assert(err, jc.ErrorIsNil)

	_, err = cmdtesting.RunCommand(c, application.NewConfigCommand(), "dummy-application", "--reset", "username")
	c.Assert(err, jc.ErrorIsNil)

	expect := charm.Settings{
		"outlook": "hello@world.tld",
	}
	settings, err = app.CharmConfig(coremodel.GenerationMaster)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, s.combinedSettings(ch, expect))
}

func (s *cmdJujuSuite) TestApplicationGetIAASModel(c *gc.C) {
	expected := `application: dummy-application
application-config:
  trust:
    default: false
    description: Does this application have access to trusted credentials
    source: default
    type: bool
    value: false
charm: dummy
settings:
  outlook:
    description: No default outlook.
    source: unset
    type: string
  skill-level:
    description: A number indicating skill.
    source: unset
    type: int
  title:
    default: My Title
    description: A descriptive title used for the application.
    source: default
    type: string
    value: My Title
  username:
    default: admin001
    description: The name of the initial account (given admin permissions).
    source: default
    type: string
    value: admin001
`
	ch := s.AddTestingCharm(c, "dummy")
	s.AddTestingApplication(c, "dummy-application", ch)

	context, err := cmdtesting.RunCommand(c, application.NewConfigCommand(), "dummy-application")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), jc.DeepEquals, expected)
}

// [TODO](externalreality): this tests has become more generic. It now tests for
// ALL application configuration rather than just CAAS configuration. It should
// be renamed.
func (s *cmdJujuSuite) TestApplicationGetCAASModel(c *gc.C) {
	expected := `application: gitlab-application
application-config:
  juju-application-path:
    default: /
    description: the relative http path used to access an application
    source: default
    type: string
    value: /
  juju-external-hostname:
    description: the external hostname of an exposed application
    source: user
    type: string
    value: ext-host
  kubernetes-ingress-allow-http:
    default: false
    description: whether to allow HTTP traffic to the ingress controller
    source: default
    type: bool
    value: false
  kubernetes-ingress-class:
    default: nginx
    description: the class of the ingress controller to be used by the ingress resource
    source: default
    type: string
    value: nginx
  kubernetes-ingress-ssl-passthrough:
    default: false
    description: whether to passthrough SSL traffic to the ingress controller
    source: default
    type: bool
    value: false
  kubernetes-ingress-ssl-redirect:
    default: false
    description: whether to redirect SSL traffic to the ingress controller
    source: default
    type: bool
    value: false
  kubernetes-service-annotations:
    description: a space separated set of annotations to add to the service
    source: unset
    type: attrs
  kubernetes-service-external-ips:
    description: list of IP addresses for which nodes in the cluster will also accept
      traffic
    source: unset
    type: string
  kubernetes-service-externalname:
    description: external reference that kubedns or equivalent will return as a CNAME
      record
    source: unset
    type: string
  kubernetes-service-loadbalancer-ip:
    description: LoadBalancer will get created with the IP specified in this field
    source: unset
    type: string
  kubernetes-service-loadbalancer-sourceranges:
    description: traffic through the load-balancer will be restricted to the specified
      client IPs
    source: unset
    type: string
  kubernetes-service-target-port:
    description: name or number of the port to access on the pods targeted by the
      service
    source: unset
    type: string
  kubernetes-service-type:
    description: determines how the Service is exposed
    source: unset
    type: string
  trust:
    default: false
    description: Does this application have access to trusted credentials
    source: default
    type: bool
    value: false
charm: gitlab
settings:
  outlook:
    description: No default outlook.
    source: unset
    type: string
  skill-level:
    description: A number indicating skill.
    source: unset
    type: int
  title:
    default: My Title
    description: A descriptive title used for the application.
    source: default
    type: string
    value: My Title
  username:
    default: admin001
    description: The name of the initial account (given admin permissions).
    source: default
    type: string
    value: admin001
`
	st := s.Factory.MakeCAASModel(c, &factory.ModelParams{Name: "caas-model"})

	// Ensure the new CAAS model is in the local store.
	modelDetails := jujuclient.ModelDetails{
		ModelUUID:    st.ModelUUID(),
		ModelType:    "caas",
		ActiveBranch: coremodel.GenerationMaster,
	}
	c.Assert(s.ControllerStore.UpdateModel("kontroll", "admin/caas-model", modelDetails), jc.ErrorIsNil)

	defer st.Close()
	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "gitlab", Series: "kubernetes"})
	app := f.MakeApplication(c, &factory.ApplicationParams{Name: "gitlab-application", Charm: ch})
	schema, err := caas.ConfigSchema(nil)
	c.Assert(err, jc.ErrorIsNil)
	err = app.UpdateApplicationConfig(coreapplication.ConfigAttributes{"juju-external-hostname": "ext-host"}, nil, schema, nil)
	c.Assert(err, jc.ErrorIsNil)

	context, err := cmdtesting.RunCommand(c, application.NewConfigCommand(), "-m", "caas-model", "gitlab-application")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), jc.DeepEquals, expected)
}

func (s *cmdJujuSuite) TestApplicationGetWeirdYAML(c *gc.C) {
	expected := `application: yaml-config
application-config:
  trust:
    default: false
    description: Does this application have access to trusted credentials
    source: default
    type: bool
    value: false
charm: yaml-config
settings:
  hexstring:
    default: "0xD06F00D"
    description: A hex string that should be a string, not a number.
    source: default
    type: string
    value: "0xD06F00D"
  nonoctal:
    default: 01182252
    description: Number that isn't valid octal, so should be a string.
    source: default
    type: string
    value: 01182252
  numberstring:
    default: "123456"
    description: A string that happens to contain a number.
    source: default
    type: string
    value: "123456"
`
	ch := s.AddTestingCharm(c, "yaml-config")
	s.AddTestingApplication(c, "yaml-config", ch)

	context, err := cmdtesting.RunCommand(c, application.NewConfigCommand(), "yaml-config")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), jc.DeepEquals, expected)
}

func (s *cmdJujuSuite) TestApplicationAddUnitExistingContainer(c *gc.C) {
	ch := s.AddTestingCharm(c, "dummy")
	app := s.AddTestingApplication(c, "some-application-name", ch)

	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	template := state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	container, err := s.State.AddMachineInsideMachine(template, machine.Id(), instance.LXD)
	c.Assert(err, jc.ErrorIsNil)

	_, err = cmdtesting.RunCommand(c, application.NewAddUnitCommand(), "some-application-name",
		"--to", container.Id())
	c.Assert(err, jc.ErrorIsNil)

	units, err := app.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	mid, err := units[0].AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mid, gc.Equals, container.Id())
}

type cmdJujuSuiteNoCAAS struct {
	jujutesting.JujuConnSuite
}
