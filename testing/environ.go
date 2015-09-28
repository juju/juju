// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"io/ioutil"
	"os"

	"github.com/juju/names"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/utils/ssh"
	"github.com/juju/juju/version"
)

// FakeAuthKeys holds the authorized key used for testing
// purposes in FakeConfig. It is valid for parsing with the utils/ssh
// authorized-key utilities.
const FakeAuthKeys = `ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAAAYQDP8fPSAMFm2PQGoVUks/FENVUMww1QTK6m++Y2qX9NGHm43kwEzxfoWR77wo6fhBhgFHsQ6ogE/cYLx77hOvjTchMEP74EVxSce0qtDjI7SwYbOpAButRId3g/Ef4STz8= joe@0.1.2.4`

func init() {
	_, err := ssh.ParseAuthorisedKey(FakeAuthKeys)
	if err != nil {
		panic("FakeAuthKeys does not hold a valid authorized key: " + err.Error())
	}
}

const FakeDefaultSeries = "trusty"

// FakeVersionNumber is a valid version number that can be used in testing.
var FakeVersionNumber = version.MustParse("1.99.0")

// EnvironmentTag is a defined known valid UUID that can be used in testing.
var EnvironmentTag = names.NewEnvironTag("deadbeef-0bad-400d-8000-4b1d0d06f00d")

// FakeConfig() returns an environment configuration for a
// fake provider with all required attributes set.
func FakeConfig() Attrs {
	return Attrs{
		"type":                      "someprovider",
		"name":                      "testenv",
		"uuid":                      EnvironmentTag.Id(),
		"authorized-keys":           FakeAuthKeys,
		"firewall-mode":             config.FwInstance,
		"admin-secret":              "fish",
		"ca-cert":                   CACert,
		"ca-private-key":            CAKey,
		"ssl-hostname-verification": true,
		"development":               false,
		"state-port":                19034,
		"api-port":                  17777,
		"default-series":            FakeDefaultSeries,
	}
}

// EnvironConfig returns a default environment configuration suitable for
// setting in the state.
func EnvironConfig(c *gc.C) *config.Config {
	return CustomEnvironConfig(c, Attrs{"uuid": mustUUID()})
}

// mustUUID returns a stringified uuid or panics
func mustUUID() string {
	uuid, err := utils.NewUUID()
	if err != nil {
		panic(err)
	}
	return uuid.String()
}

// CustomEnvironConfig returns an environment configuration with
// additional specified keys added.
func CustomEnvironConfig(c *gc.C, extra Attrs) *config.Config {
	attrs := FakeConfig().Merge(Attrs{
		"agent-version": "1.2.3",
	}).Merge(extra).Delete("admin-secret", "ca-private-key")
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	return cfg
}

const (
	SampleEnvName = "erewhemos"
	EnvDefault    = "default:\n  " + SampleEnvName + "\n"
)

const DefaultMongoPassword = "conn-from-name-secret"

// Environment names below are explicit as it makes them more readable.
const SingleEnvConfigNoDefault = `
environments:
    erewhemos:
        type: dummy
        state-server: true
        authorized-keys: i-am-a-key
        admin-secret: ` + DefaultMongoPassword + `
`

const SingleEnvConfig = EnvDefault + SingleEnvConfigNoDefault

const MultipleEnvConfigNoDefault = `
environments:
    erewhemos:
        type: dummy
        state-server: true
        authorized-keys: i-am-a-key
        admin-secret: ` + DefaultMongoPassword + `
    erewhemos-2:
        type: dummy
        state-server: true
        authorized-keys: i-am-a-key
        admin-secret: ` + DefaultMongoPassword + `
`

const MultipleEnvConfig = EnvDefault + MultipleEnvConfigNoDefault

const SampleCertName = "erewhemos"

// FakeJujuHomeSuite isolates the user's home directory and
// sets up a Juju home with a sample environment and certificate.
type FakeJujuHomeSuite struct {
	JujuOSEnvSuite
	gitjujutesting.FakeHomeSuite
	oldJujuHome string
}

func (s *FakeJujuHomeSuite) SetUpSuite(c *gc.C) {
	s.JujuOSEnvSuite.SetUpTest(c)
	s.FakeHomeSuite.SetUpTest(c)
}

func (s *FakeJujuHomeSuite) TearDownSuite(c *gc.C) {
	s.FakeHomeSuite.SetUpTest(c)
	s.JujuOSEnvSuite.SetUpTest(c)
}

func (s *FakeJujuHomeSuite) SetUpTest(c *gc.C) {
	s.JujuOSEnvSuite.SetUpTest(c)
	s.FakeHomeSuite.SetUpTest(c)
	jujuHome := gitjujutesting.HomePath(".juju")
	err := os.Mkdir(jujuHome, 0700)
	c.Assert(err, jc.ErrorIsNil)
	s.oldJujuHome = osenv.SetJujuHome(jujuHome)
	WriteEnvironments(c, SingleEnvConfig, SampleCertName)
}

func (s *FakeJujuHomeSuite) TearDownTest(c *gc.C) {
	osenv.SetJujuHome(s.oldJujuHome)
	s.FakeHomeSuite.TearDownTest(c)
	s.JujuOSEnvSuite.TearDownTest(c)
}

// AssertConfigParameterUpdated updates environment parameter and
// asserts that no errors were encountered.
func (s *FakeJujuHomeSuite) AssertConfigParameterUpdated(c *gc.C, key, value string) {
	s.PatchEnvironment(key, value)
}

// MakeSampleJujuHome sets up a sample Juju environment.
func MakeSampleJujuHome(c *gc.C) {
	WriteEnvironments(c, SingleEnvConfig, SampleCertName)
}

// WriteEnvironments creates an environments file with envConfig and certs
// from certNames.
func WriteEnvironments(c *gc.C, envConfig string, certNames ...string) {
	envs := osenv.JujuHomePath("environments.yaml")
	err := ioutil.WriteFile(envs, []byte(envConfig), 0644)
	c.Assert(err, jc.ErrorIsNil)
	for _, name := range certNames {
		err := ioutil.WriteFile(osenv.JujuHomePath(name+"-cert.pem"), []byte(CACert), 0600)
		c.Assert(err, jc.ErrorIsNil)
		err = ioutil.WriteFile(osenv.JujuHomePath(name+"-private-key.pem"), []byte(CAKey), 0600)
		c.Assert(err, jc.ErrorIsNil)
	}
}
