// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent_test

import (
	"crypto/rand"
	"fmt"
	"io"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
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
		"name":           "sample-" + uniqueName,
		"type":           "joyent",
		"control-bucket": "juju-test-" + uniqueName,
		"admin-secret":   "for real",
		"firewall-mode":  config.FwInstance,
		"agent-version":  version.Current.Number.String(),
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
	// TODO: Share code from jujutest.LiveTests for creating environment
	e, err := environs.NewFromAttrs(t.TestConfig)
	c.Assert(err, gc.IsNil)

	// Put some fake tools in place so that tests that are simply
	// starting instances without any need to check if those instances
	// are running will find them in the public bucket.
	envtesting.UploadFakeTools(c, e.Storage())
	t.LiveTests.SetUpSuite(c)
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
}

func (t *LiveTests) TearDownTest(c *gc.C) {
	t.LiveTests.TearDownTest(c)
	t.LoggingSuite.TearDownTest(c)
}
