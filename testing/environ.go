// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"os"

	"github.com/juju/names"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/ssh"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/juju/osenv"
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

// ModelTag is a defined known valid UUID that can be used in testing.
var ModelTag = names.NewModelTag("deadbeef-0bad-400d-8000-4b1d0d06f00d")

// FakeConfig() returns an environment configuration for a
// fake provider with all required attributes set.
func FakeConfig() Attrs {
	return Attrs{
		"type":                      "someprovider",
		"name":                      "testenv",
		"uuid":                      ModelTag.Id(),
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

// ModelConfig returns a default environment configuration suitable for
// setting in the state.
func ModelConfig(c *gc.C) *config.Config {
	return CustomModelConfig(c, Attrs{"uuid": mustUUID()})
}

// mustUUID returns a stringified uuid or panics
func mustUUID() string {
	uuid, err := utils.NewUUID()
	if err != nil {
		panic(err)
	}
	return uuid.String()
}

// CustomModelConfig returns an environment configuration with
// additional specified keys added.
func CustomModelConfig(c *gc.C, extra Attrs) *config.Config {
	attrs := FakeConfig().Merge(Attrs{
		"agent-version": "1.2.3",
	}).Merge(extra).Delete("admin-secret", "ca-private-key")
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	return cfg
}

const (
	SampleModelName = "erewhemos"
)

const DefaultMongoPassword = "conn-from-name-secret"

// FakeJujuXDGDataHomeSuite isolates the user's home directory and
// sets up a Juju home with a sample environment and certificate.
type FakeJujuXDGDataHomeSuite struct {
	JujuOSEnvSuite
	gitjujutesting.FakeHomeSuite
	oldJujuXDGDataHome string
}

func (s *FakeJujuXDGDataHomeSuite) SetUpSuite(c *gc.C) {
	s.JujuOSEnvSuite.SetUpTest(c)
	s.FakeHomeSuite.SetUpTest(c)
}

func (s *FakeJujuXDGDataHomeSuite) TearDownSuite(c *gc.C) {
	s.FakeHomeSuite.SetUpTest(c)
	s.JujuOSEnvSuite.SetUpTest(c)
}

func (s *FakeJujuXDGDataHomeSuite) SetUpTest(c *gc.C) {
	s.JujuOSEnvSuite.SetUpTest(c)
	s.FakeHomeSuite.SetUpTest(c)
	jujuXDGDataHome := gitjujutesting.JujuXDGDataHomePath()
	err := os.MkdirAll(jujuXDGDataHome, 0700)
	c.Assert(err, jc.ErrorIsNil)
	s.oldJujuXDGDataHome = osenv.SetJujuXDGDataHome(jujuXDGDataHome)
}

func (s *FakeJujuXDGDataHomeSuite) TearDownTest(c *gc.C) {
	osenv.SetJujuXDGDataHome(s.oldJujuXDGDataHome)
	s.FakeHomeSuite.TearDownTest(c)
	s.JujuOSEnvSuite.TearDownTest(c)
}

// AssertConfigParameterUpdated updates environment parameter and
// asserts that no errors were encountered.
func (s *FakeJujuXDGDataHomeSuite) AssertConfigParameterUpdated(c *gc.C, key, value string) {
	s.PatchEnvironment(key, value)
}
