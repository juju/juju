// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual_test

import (
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/manual"
	"launchpad.net/juju-core/testing"
)

type initialisationSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&initialisationSuite{})

func (s *initialisationSuite) TestDetectSeries(c *gc.C) {
	response := strings.Join([]string{
		"edgy",
		"armv4",
		"MemTotal: 4096 kB",
		"processor: 0",
	}, "\n")
	defer installFakeSSH(c, manual.DetectionScript, response, 0)()
	_, series, err := manual.DetectSeriesAndHardwareCharacteristics("whatever")
	c.Assert(err, gc.IsNil)
	c.Assert(series, gc.Equals, "edgy")
}

func (s *initialisationSuite) TestDetectionError(c *gc.C) {
	scriptResponse := strings.Join([]string{
		"edgy",
		"armv4",
		"MemTotal: 4096 kB",
		"processor: 0",
	}, "\n")
	// if the script fails for whatever reason, then checkProvisioned
	// will return an error. stderr will be included in the error message.
	defer installFakeSSH(c, manual.DetectionScript, []string{scriptResponse, "oh noes"}, 33)()
	hc, _, err := manual.DetectSeriesAndHardwareCharacteristics("hostname")
	c.Assert(err, gc.ErrorMatches, "subprocess encountered error code 33 \\(oh noes\\)")
	// if the script doesn't fail, stderr is simply ignored.
	defer installFakeSSH(c, manual.DetectionScript, []string{scriptResponse, "non-empty-stderr"}, 0)()
	hc, _, err = manual.DetectSeriesAndHardwareCharacteristics("hostname")
	c.Assert(err, gc.IsNil)
	c.Assert(hc.String(), gc.Equals, "arch=armhf cpu-cores=1 mem=4M")
}

func (s *initialisationSuite) TestDetectHardwareCharacteristics(c *gc.C) {
	tests := []struct {
		summary        string
		scriptResponse []string
		expectedHc     string
	}{{
		"Single CPU socket, single core, no hyper-threading",
		[]string{"edgy", "armv4", "MemTotal: 4096 kB", "processor: 0"},
		"arch=armhf cpu-cores=1 mem=4M",
	}, {
		"Single CPU socket, single core, hyper-threading",
		[]string{
			"edgy", "armv4", "MemTotal: 4096 kB",
			"processor: 0",
			"physical id: 0",
			"cpu cores: 1",
			"processor: 1",
			"physical id: 0",
			"cpu cores: 1",
		},
		"arch=armhf cpu-cores=1 mem=4M",
	}, {
		"Single CPU socket, dual-core, no hyper-threading",
		[]string{
			"edgy", "armv4", "MemTotal: 4096 kB",
			"processor: 0",
			"physical id: 0",
			"cpu cores: 2",
			"processor: 1",
			"physical id: 0",
			"cpu cores: 2",
		},
		"arch=armhf cpu-cores=2 mem=4M",
	}, {
		"Dual CPU socket, each single-core, hyper-threading",
		[]string{
			"edgy", "armv4", "MemTotal: 4096 kB",
			"processor: 0",
			"physical id: 0",
			"cpu cores: 1",
			"processor: 1",
			"physical id: 0",
			"cpu cores: 1",
			"processor: 2",
			"physical id: 1",
			"cpu cores: 1",
			"processor: 3",
			"physical id: 1",
			"cpu cores: 1",
		},
		"arch=armhf cpu-cores=2 mem=4M",
	}}
	for i, test := range tests {
		c.Logf("test %d: %s", i, test.summary)
		scriptResponse := strings.Join(test.scriptResponse, "\n")
		defer installFakeSSH(c, manual.DetectionScript, scriptResponse, 0)()
		hc, _, err := manual.DetectSeriesAndHardwareCharacteristics("hostname")
		c.Assert(err, gc.IsNil)
		c.Assert(hc.String(), gc.Equals, test.expectedHc)
	}
}

func (s *initialisationSuite) TestCheckProvisioned(c *gc.C) {
	defer installFakeSSH(c, manual.CheckProvisionedScript, "", 0)()
	provisioned, err := manual.CheckProvisioned("example.com")
	c.Assert(err, gc.IsNil)
	c.Assert(provisioned, jc.IsFalse)

	defer installFakeSSH(c, manual.CheckProvisionedScript, "non-empty", 0)()
	provisioned, err = manual.CheckProvisioned("example.com")
	c.Assert(err, gc.IsNil)
	c.Assert(provisioned, jc.IsTrue)

	// stderr should not affect result.
	defer installFakeSSH(c, manual.CheckProvisionedScript, []string{"", "non-empty-stderr"}, 0)()
	provisioned, err = manual.CheckProvisioned("example.com")
	c.Assert(err, gc.IsNil)
	c.Assert(provisioned, jc.IsFalse)

	// if the script fails for whatever reason, then checkProvisioned
	// will return an error. stderr will be included in the error message.
	defer installFakeSSH(c, manual.CheckProvisionedScript, []string{"non-empty-stdout", "non-empty-stderr"}, 255)()
	_, err = manual.CheckProvisioned("example.com")
	c.Assert(err, gc.ErrorMatches, "subprocess encountered error code 255 \\(non-empty-stderr\\)")
}

func (s *initialisationSuite) TestInitUbuntuUserNonExisting(c *gc.C) {
	defer installFakeSSH(c, "", "", 0)() // successful creation of ubuntu user
	defer installFakeSSH(c, "", "", 1)() // simulate failure of ubuntu@ login
	err := manual.InitUbuntuUser("testhost", "testuser", "", nil, nil)
	c.Assert(err, gc.IsNil)
}

func (s *initialisationSuite) TestInitUbuntuUserExisting(c *gc.C) {
	defer installFakeSSH(c, "", nil, 0)()
	manual.InitUbuntuUser("testhost", "testuser", "", nil, nil)
}

func (s *initialisationSuite) TestInitUbuntuUserError(c *gc.C) {
	defer installFakeSSH(c, "", []string{"", "failed to create ubuntu user"}, 123)()
	defer installFakeSSH(c, "", "", 1)() // simulate failure of ubuntu@ login
	err := manual.InitUbuntuUser("testhost", "testuser", "", nil, nil)
	c.Assert(err, gc.ErrorMatches, "subprocess encountered error code 123 \\(failed to create ubuntu user\\)")
}
