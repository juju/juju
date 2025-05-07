// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v4/ssh"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/semversion"
	jujuversion "github.com/juju/juju/core/version"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/charmhub"
	"github.com/juju/juju/internal/uuid"
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

var (
	// FakeSupportedJujuBases is used to provide a list of canned results
	// of a base to test bootstrap code against.
	FakeSupportedJujuBases = []corebase.Base{
		corebase.MustParseBaseFromString("ubuntu@20.04"),
		corebase.MustParseBaseFromString("ubuntu@22.04"),
		corebase.MustParseBaseFromString("ubuntu@24.04"),
		jujuversion.DefaultSupportedLTSBase(),
	}
)

// FakeVersionNumber is a valid version number that can be used in testing.
var FakeVersionNumber = semversion.MustParse("2.99.0")

// ModelTag is a defined known valid UUID that can be used in testing.
var ModelTag = names.NewModelTag("deadbeef-0bad-400d-8000-4b1d0d06f00d")

// ControllerTag is a defined known valid UUID that can be used in testing.
var ControllerTag = names.NewControllerTag("deadbeef-1bad-500d-9000-4b1d0d06f00d")

// ControllerModelTag is a defined known valid UUID that can be used in testing
// for the model the controller is running on.
var ControllerModelTag = names.NewControllerTag("deadbeef-2bad-500d-9000-4b1d0d06f00d")

// FakeControllerConfig returns an environment configuration
// that is expected to be found in state for a fake controller.
func FakeControllerConfig() controller.Config {
	return controller.Config{
		"controller-uuid":           ControllerTag.Id(),
		"ca-cert":                   CACert,
		"state-port":                1234,
		"api-port":                  17777,
		"set-numa-control-policy":   false,
		"model-logfile-max-backups": 1,
		"model-logfile-max-size":    "1M",
		"model-logs-size":           "1M",
		"max-txn-log-size":          "10M",
		"auditing-enabled":          false,
		"audit-log-capture-args":    true,
		"audit-log-max-size":        "200M",
		"audit-log-max-backups":     5,
		"query-tracing-threshold":   "1s",
		"object-store-type":         objectstore.FileBackend,
	}
}

// FakeConfig returns an model configuration for a
// fake provider with all required attributes set.
func FakeConfig() Attrs {
	return Attrs{
		"type":                      "dummy",
		"name":                      "testmodel",
		"uuid":                      ModelTag.Id(),
		"firewall-mode":             config.FwInstance,
		"ssl-hostname-verification": true,
		"development":               false,
	}
}

// FakeCloudSpec returns a cloud spec with sample data.
func FakeCloudSpec() environscloudspec.CloudSpec {
	cred := cloud.NewCredential(cloud.UserPassAuthType, map[string]string{"username": "dummy", "password": "secret"})
	return environscloudspec.CloudSpec{
		Type:             "dummy",
		Name:             "dummy",
		Endpoint:         "dummy-endpoint",
		IdentityEndpoint: "dummy-identity-endpoint",
		Region:           "dummy-region",
		StorageEndpoint:  "dummy-storage-endpoint",
		Credential:       &cred,
	}
}

// ModelConfig returns a default environment configuration suitable for
// setting in the state.
func ModelConfig(c *tc.C) *config.Config {
	uuid := mustUUID()
	return CustomModelConfig(c, Attrs{"uuid": uuid})
}

// mustUUID returns a stringified uuid or panics
func mustUUID() string {
	uuid, err := uuid.NewUUID()
	if err != nil {
		panic(err)
	}
	return uuid.String()
}

// CustomModelConfig returns an environment configuration with
// additional specified keys added.
func CustomModelConfig(c *tc.C, extra Attrs) *config.Config {
	attrs := FakeConfig().Merge(Attrs{
		"agent-version": "2.0.0",
		"charmhub-url":  charmhub.DefaultServerURL,
	}).Merge(extra).Delete("admin-secret")
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	return cfg
}

const DefaultMongoPassword = "conn-from-name-secret"

// FakeJujuXDGDataHomeSuite isolates the user's home directory and
// sets up a Juju home with a sample environment and certificate.
type FakeJujuXDGDataHomeSuite struct {
	JujuOSEnvSuite
	testing.FakeHomeSuite
}

func (s *FakeJujuXDGDataHomeSuite) SetUpTest(c *tc.C) {
	s.JujuOSEnvSuite.SetUpTest(c)
	s.FakeHomeSuite.SetUpTest(c)
}

func (s *FakeJujuXDGDataHomeSuite) TearDownTest(c *tc.C) {
	s.FakeHomeSuite.TearDownTest(c)
	s.JujuOSEnvSuite.TearDownTest(c)
}

// AssertConfigParameterUpdated updates environment parameter and
// asserts that no errors were encountered.
func (s *FakeJujuXDGDataHomeSuite) AssertConfigParameterUpdated(c *tc.C, key, value string) {
	s.PatchEnvironment(key, value)
}
