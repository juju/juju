// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/utils/ssh"
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

const FakeDefaultSeries = "precise"

// FakeConfig() returns an environment configuration for a
// fake provider with all required attributes set.
func FakeConfig() Attrs {
	return Attrs{
		"type":                      "someprovider",
		"name":                      "testenv",
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
	return CustomEnvironConfig(c, Attrs{})
}

// CustomEnvironConfig returns an environment configuration with
// additional specified keys added.
func CustomEnvironConfig(c *gc.C, extra Attrs) *config.Config {
	attrs := FakeConfig().Merge(Attrs{
		"agent-version": "1.2.3",
	}).Merge(extra).Delete("admin-secret", "ca-private-key")
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)
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
	BaseSuite
}

func (s *FakeJujuHomeSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	AddEnvironments(c, SingleEnvConfig, SampleCertName)
}
