// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"strings"

	gc "launchpad.net/gocheck"

	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
	sshtesting "launchpad.net/juju-core/utils/ssh/testing"
)

type initialisationSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&initialisationSuite{})

func (s *initialisationSuite) TestDetectSeries(c *gc.C) {
	response := strings.Join([]string{
		"edgy",
		"armv4",
		"MemTotal: 4096 kB",
		"processor: 0",
	}, "\n")
	defer sshtesting.InstallFakeSSH(c, detectionScript, response, 0)()
	_, series, err := DetectSeriesAndHardwareCharacteristics("whatever")
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
	defer sshtesting.InstallFakeSSH(c, detectionScript, []string{scriptResponse, "oh noes"}, 33)()
	hc, _, err := DetectSeriesAndHardwareCharacteristics("hostname")
	c.Assert(err, gc.ErrorMatches, "rc: 33 \\(oh noes\\)")
	// if the script doesn't fail, stderr is simply ignored.
	defer sshtesting.InstallFakeSSH(c, detectionScript, []string{scriptResponse, "non-empty-stderr"}, 0)()
	hc, _, err = DetectSeriesAndHardwareCharacteristics("hostname")
	c.Assert(err, gc.IsNil)
	c.Assert(hc.String(), gc.Equals, "arch=arm cpu-cores=1 mem=4M")
}

func (s *initialisationSuite) TestDetectHardwareCharacteristics(c *gc.C) {
	tests := []struct {
		summary        string
		scriptResponse []string
		expectedHc     string
	}{{
		"Single CPU socket, single core, no hyper-threading",
		[]string{"edgy", "armv4", "MemTotal: 4096 kB", "processor: 0"},
		"arch=arm cpu-cores=1 mem=4M",
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
		"arch=arm cpu-cores=1 mem=4M",
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
		"arch=arm cpu-cores=2 mem=4M",
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
		"arch=arm cpu-cores=2 mem=4M",
	}}
	for i, test := range tests {
		c.Logf("test %d: %s", i, test.summary)
		scriptResponse := strings.Join(test.scriptResponse, "\n")
		defer sshtesting.InstallFakeSSH(c, detectionScript, scriptResponse, 0)()
		hc, _, err := DetectSeriesAndHardwareCharacteristics("hostname")
		c.Assert(err, gc.IsNil)
		c.Assert(hc.String(), gc.Equals, test.expectedHc)
	}
}

func (s *initialisationSuite) TestCheckProvisioned(c *gc.C) {
	defer sshtesting.InstallFakeSSH(c, checkProvisionedScript, "", 0)()
	provisioned, err := checkProvisioned("example.com")
	c.Assert(err, gc.IsNil)
	c.Assert(provisioned, jc.IsFalse)

	defer sshtesting.InstallFakeSSH(c, checkProvisionedScript, "non-empty", 0)()
	provisioned, err = checkProvisioned("example.com")
	c.Assert(err, gc.IsNil)
	c.Assert(provisioned, jc.IsTrue)

	// stderr should not affect result.
	defer sshtesting.InstallFakeSSH(c, checkProvisionedScript, []string{"", "non-empty-stderr"}, 0)()
	provisioned, err = checkProvisioned("example.com")
	c.Assert(err, gc.IsNil)
	c.Assert(provisioned, jc.IsFalse)

	// if the script fails for whatever reason, then checkProvisioned
	// will return an error. stderr will be included in the error message.
	defer sshtesting.InstallFakeSSH(c, checkProvisionedScript, []string{"non-empty-stdout", "non-empty-stderr"}, 255)()
	_, err = checkProvisioned("example.com")
	c.Assert(err, gc.ErrorMatches, "rc: 255 \\(non-empty-stderr\\)")
}

func (s *initialisationSuite) TestInitUbuntuUserNonExisting(c *gc.C) {
	defer sshtesting.InstallFakeSSH(c, "", "", 0)() // successful creation of ubuntu user
	defer sshtesting.InstallFakeSSH(c, "", "", 1)() // simulate failure of ubuntu@ login
	err := InitUbuntuUser("testhost", "testuser", "", nil, nil)
	c.Assert(err, gc.IsNil)
}

func (s *initialisationSuite) TestInitUbuntuUserExisting(c *gc.C) {
	defer sshtesting.InstallFakeSSH(c, "", nil, 0)()
	InitUbuntuUser("testhost", "testuser", "", nil, nil)
}

func (s *initialisationSuite) TestInitUbuntuUserError(c *gc.C) {
	defer sshtesting.InstallFakeSSH(c, "", []string{"", "failed to create ubuntu user"}, 123)()
	defer sshtesting.InstallFakeSSH(c, "", "", 1)() // simulate failure of ubuntu@ login
	err := InitUbuntuUser("testhost", "testuser", "", nil, nil)
	c.Assert(err, gc.ErrorMatches, "rc: 123 \\(failed to create ubuntu user\\)")
}
