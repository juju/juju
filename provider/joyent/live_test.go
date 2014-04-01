// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent_test

import (
	"crypto/rand"
	"fmt"
	"io"
	"os"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/jujutest"
	envtesting "launchpad.net/juju-core/environs/testing"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/version"
)

// uniqueName is generated afresh for every test run, so that
// we are not polluted by previous test state.
var uniqueName = randomName()

func randomName() string {
	buf := make([]byte, 8)
	_, err := io.ReadFull(rand.Reader, buf)
	if err != nil {
		panic(fmt.Sprintf("error from crypto rand: %v", err))
	}
	return fmt.Sprintf("%x", buf)
}

func registerLiveTests() {
	attrs := coretesting.FakeConfig().Merge(map[string]interface{}{
		"name":          "sample-" + uniqueName,
		"type":          "joyent",
		"sdc-user":      os.Getenv("SDC_ACCOUNT"),
		"sdc-key-id":    os.Getenv("SDC_KEY_ID"),
		"manta-user":    os.Getenv("MANTA_USER"),
		"manta-key-id":  os.Getenv("MANTA_KEY_ID"),
		"control-dir":   "juju-test-" + uniqueName,
		"admin-secret":  "for real",
		"firewall-mode": config.FwInstance,
		"agent-version": version.Current.Number.String(),
	})
	gc.Suite(&LiveTests{
		LiveTests: jujutest.LiveTests{
			TestConfig:     attrs,
			CanOpenState:   true,
			HasProvisioner: true,
		},
	})
}

// LiveTests contains tests that can be run against the Joyent Public Cloud.
type LiveTests struct {
	testbase.LoggingSuite
	jujutest.LiveTests
}

func (t *LiveTests) SetUpSuite(c *gc.C) {
	t.LoggingSuite.SetUpSuite(c)

	t.LiveTests.SetUpSuite(c)
	// For testing, we create a storage instance to which is uploaded tools and image metadata.
	t.PrepareOnce(c)
	c.Assert(t.Env.Storage(), gc.NotNil)
	// Put some fake tools metadata in place so that tests that are simply
	// starting instances without any need to check if those instances
	// are running can find the metadata.
	envtesting.UploadFakeTools(c, t.Env.Storage())
}

func (t *LiveTests) TearDownSuite(c *gc.C) {
	if t.Env == nil {
		// This can happen if SetUpSuite fails.
		return
	}
	t.LiveTests.TearDownSuite(c)
	t.LoggingSuite.TearDownSuite(c)
}

func (t *LiveTests) SetUpTest(c *gc.C) {
	t.LoggingSuite.SetUpTest(c)
	t.LiveTests.SetUpTest(c)
	c.Assert(t.Env.Storage(), gc.NotNil)
}

func (t *LiveTests) TearDownTest(c *gc.C) {
	t.LiveTests.TearDownTest(c)
	t.LoggingSuite.TearDownTest(c)
}
