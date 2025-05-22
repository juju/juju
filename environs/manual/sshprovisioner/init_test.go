// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package sshprovisioner_test

import (
	"strings"
	stdtesting "testing"

	"github.com/juju/tc"

	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/environs/manual/sshprovisioner"
	"github.com/juju/juju/internal/service"
	"github.com/juju/juju/internal/testing"
)

type initialisationSuite struct {
	testing.BaseSuite
}

func TestInitialisationSuite(t *stdtesting.T) {
	tc.Run(t, &initialisationSuite{})
}

func (s *initialisationSuite) TestDetectBase(c *tc.C) {
	response := strings.Join([]string{
		"ubuntu",
		"6.10",
		"armv4",
		"MemTotal: 4096 kB",
		"processor: 0",
	}, "\n")
	defer installFakeSSH(c, sshprovisioner.DetectionScript, response, 0)()
	_, base, err := sshprovisioner.DetectBaseAndHardwareCharacteristics("whatever", "vmuser")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(base, tc.Equals, corebase.MustParseBaseFromString("ubuntu@6.10"))
}

func (s *initialisationSuite) TestDetectionError(c *tc.C) {
	scriptResponse := strings.Join([]string{
		"ubuntu",
		"6.10",
		"ppc64le",
		"MemTotal: 4096 kB",
		"processor: 0",
	}, "\n")
	// if the script fails for whatever reason, then checkProvisioned
	// will return an error. stderr will be included in the error message.
	defer installFakeSSH(c, sshprovisioner.DetectionScript, []string{scriptResponse, "oh noes"}, 33)()
	_, _, err := sshprovisioner.DetectBaseAndHardwareCharacteristics("hostname", "vmuser")
	c.Assert(err, tc.ErrorMatches, "subprocess encountered error code 33 \\(oh noes\\)")
	// if the script doesn't fail, stderr is simply ignored.
	defer installFakeSSH(c, sshprovisioner.DetectionScript, []string{scriptResponse, "non-empty-stderr"}, 0)()
	hc, _, err := sshprovisioner.DetectBaseAndHardwareCharacteristics("hostname", "vmuser")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(hc.String(), tc.Equals, "arch=ppc64el cores=1 mem=4M")
}

func (s *initialisationSuite) TestDetectHardwareCharacteristics(c *tc.C) {
	tests := []struct {
		summary        string
		scriptResponse []string
		expectedHc     string
	}{{
		"Single CPU socket, single core, no hyper-threading",
		[]string{"ubuntu", "6.10", "s390x", "MemTotal: 4096 kB", "processor: 0"},
		"arch=s390x cores=1 mem=4M",
	}, {
		"Single CPU socket, single core, hyper-threading",
		[]string{
			"ubuntu", "6.10", "s390x", "MemTotal: 4096 kB",
			"processor: 0",
			"physical id: 0",
			"cpu cores: 1",
			"processor: 1",
			"physical id: 0",
			"cpu cores: 1",
		},
		"arch=s390x cores=1 mem=4M",
	}, {
		"Single CPU socket, dual-core, no hyper-threading",
		[]string{
			"ubuntu", "6.10", "s390x", "MemTotal: 4096 kB",
			"processor: 0",
			"physical id: 0",
			"cpu cores: 2",
			"processor: 1",
			"physical id: 0",
			"cpu cores: 2",
		},
		"arch=s390x cores=2 mem=4M",
	}, {
		"Dual CPU socket, each single-core, hyper-threading",
		[]string{
			"ubuntu", "6.10", "s390x", "MemTotal: 4096 kB",
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
		"arch=s390x cores=2 mem=4M",
	}, {
		"4 CPU sockets, each single-core, no hyper-threading, no physical id field",
		[]string{
			"ubuntu", "6.10", "arm64", "MemTotal: 16384 kB",
			"processor: 0",
			"processor: 1",
			"processor: 2",
			"processor: 3",
		},
		"arch=arm64 cores=4 mem=16M",
	}}

	for i, test := range tests {
		c.Logf("test %d: %s", i, test.summary)
		scriptResponse := strings.Join(test.scriptResponse, "\n")
		defer installFakeSSH(c, sshprovisioner.DetectionScript, scriptResponse, 0)()
		hc, _, err := sshprovisioner.DetectBaseAndHardwareCharacteristics("hostname", "vmuser")
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(hc.String(), tc.Equals, test.expectedHc)
	}
}

func (s *initialisationSuite) TestCheckProvisioned(c *tc.C) {
	listCmd := service.ListServicesScript()
	defer installFakeSSH(c, listCmd, "", 0)()
	provisioned, err := sshprovisioner.CheckProvisioned("example.com", "vmuser")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(provisioned, tc.IsFalse)

	defer installFakeSSH(c, listCmd, "snap.juju.fetch-oci", 0)()
	provisioned, err = sshprovisioner.CheckProvisioned("example.com", "vmuser")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(provisioned, tc.IsFalse)

	defer installFakeSSH(c, listCmd, "jujud-machine-42", 0)()
	provisioned, err = sshprovisioner.CheckProvisioned("example.com", "vmuser")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(provisioned, tc.IsTrue)

	// stderr should not affect result.
	defer installFakeSSH(c, listCmd, []string{"", "non-empty-stderr"}, 0)()
	provisioned, err = sshprovisioner.CheckProvisioned("example.com", "vmuser")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(provisioned, tc.IsFalse)

	// if the script fails for whatever reason, then checkProvisioned
	// will return an error. stderr will be included in the error message.
	defer installFakeSSH(c, listCmd, []string{"non-empty-stdout", "non-empty-stderr"}, 255)()
	_, err = sshprovisioner.CheckProvisioned("example.com", "vmuser")
	c.Assert(err, tc.ErrorMatches, "subprocess encountered error code 255 \\(non-empty-stderr\\)")
}

func (s *initialisationSuite) TestInitUbuntuUserNonExisting(c *tc.C) {
	defer installFakeSSH(c, "", "", 0)() // successful creation of ubuntu user
	defer installFakeSSH(c, "", "", 1)() // simulate failure of ubuntu@ login
	err := sshprovisioner.InitUbuntuUser("testhost", "testuser", "", "", nil, nil)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *initialisationSuite) TestInitUbuntuUserExisting(c *tc.C) {
	defer installFakeSSH(c, "", nil, 0)()
	sshprovisioner.InitUbuntuUser("testhost", "testuser", "", "", nil, nil)
}

func (s *initialisationSuite) TestInitUbuntuUserError(c *tc.C) {
	defer installFakeSSH(c, "", []string{"", "failed to create ubuntu user"}, 123)()
	defer installFakeSSH(c, "", "", 1)() // simulate failure of ubuntu@ login
	err := sshprovisioner.InitUbuntuUser("testhost", "testuser", "", "", nil, nil)
	c.Assert(err, tc.ErrorMatches, "subprocess encountered error code 123 \\(failed to create ubuntu user\\)")
}
