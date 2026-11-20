// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package sshprovisioner_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	gc "gopkg.in/check.v1"

	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/environs/manual/sshprovisioner"
	"github.com/juju/juju/service"
	"github.com/juju/juju/testing"
)

type initialisationSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&initialisationSuite{})

func (s *initialisationSuite) TestDetectBase(c *gc.C) {
	response := strings.Join([]string{
		"ubuntu",
		"6.10",
		"armv4",
		"MemTotal: 4096 kB",
		"processor: 0",
	}, "\n")
	defer installFakeSSH(c, sshprovisioner.DetectionScript, response, 0)()
	_, base, err := sshprovisioner.DetectBaseAndHardwareCharacteristics("whatever")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(base, gc.Equals, corebase.MustParseBaseFromString("ubuntu@6.10"))
}

func (s *initialisationSuite) TestDetectionError(c *gc.C) {
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
	_, _, err := sshprovisioner.DetectBaseAndHardwareCharacteristics("hostname")
	c.Assert(err, gc.ErrorMatches, "subprocess encountered error code 33 \\(oh noes\\)")
	// if the script doesn't fail, stderr is simply ignored.
	defer installFakeSSH(c, sshprovisioner.DetectionScript, []string{scriptResponse, "non-empty-stderr"}, 0)()
	hc, _, err := sshprovisioner.DetectBaseAndHardwareCharacteristics("hostname")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(hc.String(), gc.Equals, "arch=ppc64el cores=1 mem=4M")
}

func (s *initialisationSuite) TestDetectHardwareCharacteristics(c *gc.C) {
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
		hc, _, err := sshprovisioner.DetectBaseAndHardwareCharacteristics("hostname")
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(hc.String(), gc.Equals, test.expectedHc)
	}
}

func (s *initialisationSuite) TestCheckProvisioned(c *gc.C) {
	listCmd := service.ListServicesScript()
	defer installFakeSSH(c, listCmd, "", 0)()
	provisioned, err := sshprovisioner.CheckProvisioned("example.com")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(provisioned, jc.IsFalse)

	defer installFakeSSH(c, listCmd, "snap.juju.fetch-oci", 0)()
	provisioned, err = sshprovisioner.CheckProvisioned("example.com")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(provisioned, jc.IsFalse)

	defer installFakeSSH(c, listCmd, "jujud-machine-42", 0)()
	provisioned, err = sshprovisioner.CheckProvisioned("example.com")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(provisioned, jc.IsTrue)

	// stderr should not affect result.
	defer installFakeSSH(c, listCmd, []string{"", "non-empty-stderr"}, 0)()
	provisioned, err = sshprovisioner.CheckProvisioned("example.com")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(provisioned, jc.IsFalse)

	// if the script fails for whatever reason, then checkProvisioned
	// will return an error. stderr will be included in the error message.
	defer installFakeSSH(c, listCmd, []string{"non-empty-stdout", "non-empty-stderr"}, 255)()
	_, err = sshprovisioner.CheckProvisioned("example.com")
	c.Assert(err, gc.ErrorMatches, "subprocess encountered error code 255 \\(non-empty-stderr\\)")
}

func (s *initialisationSuite) TestInitUbuntuUserNonExisting(c *gc.C) {
	defer installFakeSSH(c, "", "", 0)() // successful creation of ubuntu user
	defer installFakeSSH(c, "", "", 1)() // simulate failure of ubuntu@ login
	err := sshprovisioner.InitUbuntuUser("testhost", "testuser", "", "", nil, nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *initialisationSuite) TestInitUbuntuUserExisting(c *gc.C) {
	defer installFakeSSH(c, "", nil, 0)()
	sshprovisioner.InitUbuntuUser("testhost", "testuser", "", "", nil, nil)
}

func (s *initialisationSuite) TestInitUbuntuUserError(c *gc.C) {
	defer installFakeSSH(c, "", []string{"", "failed to create ubuntu user"}, 123)()
	defer installFakeSSH(c, "", "", 1)() // simulate failure of ubuntu@ login
	err := sshprovisioner.InitUbuntuUser("testhost", "testuser", "", "", nil, nil)
	c.Assert(err, gc.ErrorMatches, "subprocess encountered error code 123 \\(failed to create ubuntu user\\)")
}

func (s *initialisationSuite) TestInitUbuntuUserProbeWithPrivateKey(c *gc.C) {
	fakebin := c.MkDir()
	argsFile := filepath.Join(fakebin, "ssh.args")
	sshScript := fmt.Sprintf("#!/bin/bash --norc\nprintf '%%s\\n' \"$*\" > %s\nexit 0\n", utils.ShQuote(argsFile))
	err := os.WriteFile(filepath.Join(fakebin, "ssh"), []byte(sshScript), 0755)
	c.Assert(err, jc.ErrorIsNil)
	s.PatchEnvPathPrepend(fakebin)

	err = sshprovisioner.InitUbuntuUser("testhost", "testuser", "", "/tmp/my-private-key", nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	argsBytes, err := os.ReadFile(argsFile)
	c.Assert(err, jc.ErrorIsNil)
	args := string(argsBytes)
	c.Check(strings.Contains(args, "-i /tmp/my-private-key"), jc.IsTrue, gc.Commentf("args: %s", args))
	c.Check(strings.Contains(args, "ubuntu@testhost sudo -n true"), jc.IsTrue, gc.Commentf("args: %s", args))
}

func (s *initialisationSuite) TestInitUbuntuUserFallbackArgsAndQuoting(c *gc.C) {
	fakebin := c.MkDir()
	probeFile := filepath.Join(fakebin, "probe.args")
	fallbackFile := filepath.Join(fakebin, "fallback.args")
	markerFile := filepath.Join(fakebin, "ran.probe")
	sshScript := fmt.Sprintf(`#!/bin/bash --norc
if [ ! -e %s ]; then
    touch %s
    printf '%%s\n' "$*" > %s
    exit 1
fi
printf '%%s\n' "$*" > %s
exit 0
`, utils.ShQuote(markerFile), utils.ShQuote(markerFile), utils.ShQuote(probeFile), utils.ShQuote(fallbackFile))
	err := os.WriteFile(filepath.Join(fakebin, "ssh"), []byte(sshScript), 0755)
	c.Assert(err, jc.ErrorIsNil)
	s.PatchEnvPathPrepend(fakebin)

	specialKeys := "ssh-rsa AAAA_old 'single' \"double\" $(echo injected) `uname`\n-n hyphen\\key"
	err = sshprovisioner.InitUbuntuUser("testhost", "testuser", specialKeys, "/tmp/my-private-key", nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	probeArgs, err := os.ReadFile(probeFile)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(strings.Contains(string(probeArgs), "-i /tmp/my-private-key"), jc.IsTrue)
	c.Check(strings.Contains(string(probeArgs), "ubuntu@testhost sudo -n true"), jc.IsTrue)

	fallbackArgs, err := os.ReadFile(fallbackFile)
	c.Assert(err, jc.ErrorIsNil)
	fb := string(fallbackArgs)
	c.Check(strings.Contains(fb, "-i /tmp/my-private-key"), jc.IsTrue)
	c.Check(strings.Contains(fb, "-t -t"), jc.IsTrue)
	c.Check(strings.Contains(fb, "testuser@testhost sudo /bin/bash -c "), jc.IsTrue)

	// Exercise the exact ShQuote -> bash -c -> export -> sh -c chain produced
	// by InitUbuntuScript against a pre-existing authorized_keys file.
	formatted := fmt.Sprintf(
		sshprovisioner.InitUbuntuScript,
		utils.ShQuote(specialKeys),
		utils.ShQuote(sshprovisioner.EnsureAuthorizedKeysScript),
	)
	idx := strings.Index(formatted, "export authorized_keys=")
	c.Assert(idx, gc.Not(gc.Equals), -1)
	tailScript := strings.Replace(formatted[idx:], "su -s /bin/sh ubuntu -c ", "sh -c ", 1)

	homeDir := c.MkDir()
	err = os.Mkdir(filepath.Join(homeDir, ".ssh"), 0700)
	c.Assert(err, jc.ErrorIsNil)
	err = os.WriteFile(filepath.Join(homeDir, ".ssh", "authorized_keys"), []byte("existing-key"), 0600)
	c.Assert(err, jc.ErrorIsNil)

	cmd := exec.Command("bash", "-c", "set -e\n"+tailScript)
	cmd.Env = append(os.Environ(), "HOME="+homeDir)
	out, err := cmd.CombinedOutput()
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("output: %s", string(out)))

	got, err := os.ReadFile(filepath.Join(homeDir, ".ssh", "authorized_keys"))
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(got), gc.Equals, "existing-key\n"+specialKeys+"\n")
}

func (s *initialisationSuite) TestInitUbuntuScriptSyntax(c *gc.C) {
	script := fmt.Sprintf(
		sshprovisioner.InitUbuntuScript,
		utils.ShQuote("test-key"),
		utils.ShQuote(sshprovisioner.EnsureAuthorizedKeysScript),
	)
	cmd := exec.Command("bash", "-n")
	cmd.Stdin = strings.NewReader(script)
	out, err := cmd.CombinedOutput()
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("bash -n failed: %s", string(out)))

	for _, shell := range []string{"sh", "bash"} {
		shCmd := exec.Command(shell, "-n")
		shCmd.Stdin = strings.NewReader(sshprovisioner.EnsureAuthorizedKeysScript)
		out, err = shCmd.CombinedOutput()
		c.Assert(err, jc.ErrorIsNil, gc.Commentf("%s -n failed: %s", shell, string(out)))
	}
}

func (s *initialisationSuite) TestInitUbuntuScriptDoesNotTruncate(c *gc.C) {
	c.Check(strings.Contains(sshprovisioner.InitUbuntuScript, "install -D -m 0600 /dev/null"), jc.IsFalse)
	c.Check(strings.Contains(sshprovisioner.InitUbuntuScript, "/dev/null ~/.ssh/authorized_keys"), jc.IsFalse)
	c.Check(strings.Contains(sshprovisioner.InitUbuntuScript, "su -s /bin/sh ubuntu -c"), jc.IsTrue)
	c.Check(strings.Contains(sshprovisioner.EnsureAuthorizedKeysScript, "install -d -m 0700 ~/.ssh"), jc.IsTrue)
	c.Check(strings.Contains(sshprovisioner.EnsureAuthorizedKeysScript, "touch ~/.ssh/authorized_keys"), jc.IsTrue)
	c.Check(strings.Contains(sshprovisioner.EnsureAuthorizedKeysScript, "chmod 0600 ~/.ssh/authorized_keys"), jc.IsTrue)
	c.Check(strings.Contains(sshprovisioner.EnsureAuthorizedKeysScript, `grep -qxF -e "$line"`), jc.IsTrue)
}

func (s *initialisationSuite) TestInitUbuntuScriptPreservesAuthorizedKeys(c *gc.C) {
	runScript := func(shell, homeDir, incomingKeys string) ([]byte, error) {
		cmd := exec.Command(shell, "-c", sshprovisioner.EnsureAuthorizedKeysScript)
		cmd.Env = append(os.Environ(),
			"HOME="+homeDir,
			"authorized_keys="+incomingKeys,
		)
		return cmd.CombinedOutput()
	}

	for _, shell := range []string{"sh", "bash"} {
		c.Logf("testing with shell %q", shell)

		// 1. Initial setup when .ssh/authorized_keys does not exist yet.
		home1 := c.MkDir()
		out, err := runScript(shell, home1, "ssh-rsa AAAA_juju_key user@juju\n")
		c.Assert(err, jc.ErrorIsNil, gc.Commentf("output: %s", string(out)))
		content1, err := os.ReadFile(filepath.Join(home1, ".ssh", "authorized_keys"))
		c.Assert(err, jc.ErrorIsNil)
		c.Check(string(content1), gc.Equals, "ssh-rsa AAAA_juju_key user@juju\n")

		dirInfo, err := os.Stat(filepath.Join(home1, ".ssh"))
		c.Assert(err, jc.ErrorIsNil)
		c.Check(dirInfo.Mode().Perm(), gc.Equals, os.FileMode(0700))

		fileInfo, err := os.Stat(filepath.Join(home1, ".ssh", "authorized_keys"))
		c.Assert(err, jc.ErrorIsNil)
		c.Check(fileInfo.Mode().Perm(), gc.Equals, os.FileMode(0600))

		// 2. Pre-existing keys with trailing newline are preserved.
		home2 := c.MkDir()
		err = os.Mkdir(filepath.Join(home2, ".ssh"), 0700)
		c.Assert(err, jc.ErrorIsNil)
		preExisting2 := "ssh-ed25519 AAAA_existing_1 admin@host\nssh-rsa AAAA_existing_2 dev@host\n"
		err = os.WriteFile(filepath.Join(home2, ".ssh", "authorized_keys"), []byte(preExisting2), 0600)
		c.Assert(err, jc.ErrorIsNil)

		out, err = runScript(shell, home2, "ssh-rsa AAAA_juju_key user@juju\n")
		c.Assert(err, jc.ErrorIsNil, gc.Commentf("output: %s", string(out)))
		content2, err := os.ReadFile(filepath.Join(home2, ".ssh", "authorized_keys"))
		c.Assert(err, jc.ErrorIsNil)
		c.Check(string(content2), gc.Equals, preExisting2+"ssh-rsa AAAA_juju_key user@juju\n")

		// 3. Pre-existing keys without trailing newline are preserved and
		// properly formatted with a separating newline.
		home3 := c.MkDir()
		err = os.Mkdir(filepath.Join(home3, ".ssh"), 0700)
		c.Assert(err, jc.ErrorIsNil)
		preExisting3 := "ssh-ed25519 AAAA_existing_no_newline admin@host"
		err = os.WriteFile(filepath.Join(home3, ".ssh", "authorized_keys"), []byte(preExisting3), 0600)
		c.Assert(err, jc.ErrorIsNil)

		out, err = runScript(shell, home3, "ssh-rsa AAAA_juju_key user@juju\n")
		c.Assert(err, jc.ErrorIsNil, gc.Commentf("output: %s", string(out)))
		content3, err := os.ReadFile(filepath.Join(home3, ".ssh", "authorized_keys"))
		c.Assert(err, jc.ErrorIsNil)
		c.Check(string(content3), gc.Equals, preExisting3+"\nssh-rsa AAAA_juju_key user@juju\n")

		// 4. Duplicate keys are not added twice.
		home4 := c.MkDir()
		err = os.Mkdir(filepath.Join(home4, ".ssh"), 0700)
		c.Assert(err, jc.ErrorIsNil)
		preExisting4 := "ssh-rsa AAAA_juju_key user@juju\n"
		err = os.WriteFile(filepath.Join(home4, ".ssh", "authorized_keys"), []byte(preExisting4), 0600)
		c.Assert(err, jc.ErrorIsNil)

		out, err = runScript(shell, home4, "ssh-rsa AAAA_juju_key user@juju\n")
		c.Assert(err, jc.ErrorIsNil, gc.Commentf("output: %s", string(out)))
		content4, err := os.ReadFile(filepath.Join(home4, ".ssh", "authorized_keys"))
		c.Assert(err, jc.ErrorIsNil)
		c.Check(string(content4), gc.Equals, preExisting4)

		// 5. Empty incoming key leaves pre-existing keys intact.
		home5 := c.MkDir()
		err = os.Mkdir(filepath.Join(home5, ".ssh"), 0700)
		c.Assert(err, jc.ErrorIsNil)
		preExisting5 := "ssh-rsa AAAA_pre_existing dev@host\n"
		err = os.WriteFile(filepath.Join(home5, ".ssh", "authorized_keys"), []byte(preExisting5), 0600)
		c.Assert(err, jc.ErrorIsNil)

		out, err = runScript(shell, home5, "")
		c.Assert(err, jc.ErrorIsNil, gc.Commentf("output: %s", string(out)))
		content5, err := os.ReadFile(filepath.Join(home5, ".ssh", "authorized_keys"))
		c.Assert(err, jc.ErrorIsNil)
		c.Check(string(content5), gc.Equals, preExisting5)

		// 6. Hyphen-prefixed keys/comments do not collide with grep flags.
		home6 := c.MkDir()
		err = os.Mkdir(filepath.Join(home6, ".ssh"), 0700)
		c.Assert(err, jc.ErrorIsNil)
		err = os.WriteFile(filepath.Join(home6, ".ssh", "authorized_keys"), []byte("ssh-rsa AAAA_old dev\n"), 0600)
		c.Assert(err, jc.ErrorIsNil)

		out, err = runScript(shell, home6, "-n option-like-key\n")
		c.Assert(err, jc.ErrorIsNil, gc.Commentf("output: %s", string(out)))
		content6, err := os.ReadFile(filepath.Join(home6, ".ssh", "authorized_keys"))
		c.Assert(err, jc.ErrorIsNil)
		c.Check(string(content6), gc.Equals, "ssh-rsa AAAA_old dev\n-n option-like-key\n")

		// 7. Multi-line keys with empty lines and no trailing newline.
		home7 := c.MkDir()
		err = os.Mkdir(filepath.Join(home7, ".ssh"), 0700)
		c.Assert(err, jc.ErrorIsNil)

		out, err = runScript(shell, home7, "\nkeyA\n\nkeyB")
		c.Assert(err, jc.ErrorIsNil, gc.Commentf("output: %s", string(out)))
		content7, err := os.ReadFile(filepath.Join(home7, ".ssh", "authorized_keys"))
		c.Assert(err, jc.ErrorIsNil)
		c.Check(string(content7), gc.Equals, "keyA\nkeyB\n")

		// 8. Write failure fails the script rather than being masked.
		home8 := c.MkDir()
		err = os.MkdirAll(filepath.Join(home8, ".ssh", "authorized_keys"), 0700)
		c.Assert(err, jc.ErrorIsNil)

		_, err = runScript(shell, home8, "key1\nkey2\n")
		c.Check(err, gc.NotNil)
	}
}
