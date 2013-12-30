// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent

import (
	"fmt"
	"os"
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/config"
	envtesting "launchpad.net/juju-core/environs/testing"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/testbase"
)

func TestJoyentProvider(t *stdtesting.T) {
	gc.TestingT(t)
}

type providerSuite struct {
	testbase.LoggingSuite
	envtesting.ToolsFixture
	restoreTimeouts func()
}

var _ = gc.Suite(&providerSuite{})

func (s *providerSuite) SetUpSuite(c *gc.C) {
	s.restoreTimeouts = envtesting.PatchAttemptStrategies()
	s.LoggingSuite.SetUpSuite(c)
}

func (s *providerSuite) TearDownSuite(c *gc.C) {
	s.restoreTimeouts()
	s.LoggingSuite.TearDownSuite(c)
}

func (s *providerSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)
}

func (s *providerSuite) TearDownTest(c *gc.C) {
	s.ToolsFixture.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
}

// makeEnviron creates a functional Joyent environ for a test.
func (suite *providerSuite) makeEnviron() *joyentEnviron {
	attrs := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"name":         "joyent test environment",
		"type":         "joyent",
		"sdc-user":     "dstroppa",
		"sdc-key-id":   "12:c3:a7:cb:a2:29:e2:90:88:3f:04:53:3b:4e:75:40",
		"sdc-url":      "https://us-west-1.api.joyentcloud.com",
		"manta-user":   "dstroppa",
		"manta-key-id": "12:c3:a7:cb:a2:29:e2:90:88:3f:04:53:3b:4e:75:40",
		"manta-url":    "https://us-east.manta.joyent.com",
		"key-file":     fmt.Sprintf("%s/.ssh/id_rsa", os.Getenv("HOME")),
		"algorithm":    "rsa-sha256",
		"control-dir":  "juju-test",
	})
	cfg, err := config.New(config.NoDefaults, attrs)
	if err != nil {
		panic(err)
	}
	env, err := NewEnviron(cfg)
	if err != nil {
		panic(err)
	}
	return env
}
