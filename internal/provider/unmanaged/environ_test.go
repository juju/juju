// Copyright 2012 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unmanaged

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/manual/sshprovisioner"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	internalrecovery "github.com/juju/juju/internal/recovery"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/tools"
)

type baseEnvironSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	env *manualEnviron
}

func (s *baseEnvironSuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	env, err := UnmanagedProvider{}.Open(c.Context(), environs.OpenParams{
		Cloud:  CloudSpec(),
		Config: MinimalConfig(c),
	}, environs.NoopCredentialInvalidator())
	c.Assert(err, tc.ErrorIsNil)
	s.env = env.(*manualEnviron)
}

type environSuite struct {
	baseEnvironSuite
}

func TestEnvironSuite(t *testing.T) {
	tc.Run(t, &environSuite{})
}

func (s *environSuite) TestInstances(c *tc.C) {
	var ids []instance.Id

	instances, err := s.env.Instances(c.Context(), ids)
	c.Assert(err, tc.Equals, environs.ErrNoInstances)
	c.Assert(instances, tc.HasLen, 0)

	ids = append(ids, BootstrapInstanceId)
	instances, err = s.env.Instances(c.Context(), ids)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(instances, tc.HasLen, 1)
	c.Assert(instances[0], tc.NotNil)

	ids = append(ids, BootstrapInstanceId)
	instances, err = s.env.Instances(c.Context(), ids)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(instances, tc.HasLen, 2)
	c.Assert(instances[0], tc.NotNil)
	c.Assert(instances[1], tc.NotNil)

	ids = append(ids, instance.Id("invalid"))
	instances, err = s.env.Instances(c.Context(), ids)
	c.Assert(err, tc.Equals, environs.ErrPartialInstances)
	c.Assert(instances, tc.HasLen, 3)
	c.Assert(instances[0], tc.NotNil)
	c.Assert(instances[1], tc.NotNil)
	c.Assert(instances[2], tc.IsNil)

	ids = []instance.Id{instance.Id("invalid")}
	instances, err = s.env.Instances(c.Context(), ids)
	c.Assert(err, tc.Equals, environs.ErrNoInstances)
	c.Assert(instances, tc.HasLen, 1)
	c.Assert(instances[0], tc.IsNil)
}

func (s *environSuite) TestDestroyController(c *tc.C) {
	var resultStdout string
	var resultErr error
	runSSHCommandTesting := func(host string, command []string, stdin string) (string, string, error) {
		c.Assert(host, tc.Equals, "hostname")
		c.Assert(command, tc.DeepEquals, []string{"sudo", "/bin/bash"})
		c.Assert(stdin, tc.Equals, `
# Signal the jujuagentd process to stop, then check it has done so.
set -x

stopped=0
function wait_for_jujuagentd {
    for i in {1..30}; do
        if pgrep jujuagentd > /dev/null ; then
            sleep 1
        else
            echo jujuagentd stopped
            stopped=1
            logger --id jujuagentd stopped on attempt $i
            break
        fi
    done
}

# There might be no jujuagentd at all (for example, after a failed deployment) so
# don't require pkill to succeed before looking for a jujuagentd process.
# SIGABRT not SIGTERM, as abort lets the worker know it should uninstall itself,
# rather than terminate normally.
pkill -SIGABRT jujuagentd
wait_for_jujuagentd

[[ $stopped -ne 1 ]] && {
    # If jujuagentd didn't stop nicely, we kill it hard here.
    pkill -SIGKILL jujuagentd && wait_for_jujuagentd
}
[[ $stopped -ne 1 ]] && {
    echo stopping jujuagentd failed
    logger --id $(ps -o pid,cmd,state -p $(pgrep jujuagentd) | awk 'NR != 1 {printf("Process %d (%s) has state %s\n", $1, $2, $3)}')
    exit 1
}
exit 0
`)
		return resultStdout, "", resultErr
	}
	s.PatchValue(&runSSHCommand, runSSHCommandTesting)
	type test struct {
		stdout string
		err    error
		match  string
	}
	tests := []test{
		{"", nil, ""},
		{"abc", nil, ""},
		{"", errors.New("oh noes"), "oh noes"},
	}
	for i, t := range tests {
		c.Logf("test %d: %v", i, t)
		resultStdout, resultErr = t.stdout, t.err
		err := s.env.DestroyController(c.Context(), "controller-uuid")
		if t.match == "" {
			c.Assert(err, tc.ErrorIsNil)
		} else {
			c.Assert(err, tc.ErrorMatches, t.match)
		}
	}
}

func (s *environSuite) TestConstraintsValidator(c *tc.C) {
	s.PatchValue(&sshprovisioner.DetectBaseAndHardwareCharacteristics,
		func(string, string) (instance.HardwareCharacteristics, base.Base, error) {
			amd64 := "amd64"
			return instance.HardwareCharacteristics{
				Arch: &amd64,
			}, base.Base{}, nil
		},
	)

	validator, err := s.env.ConstraintsValidator(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	cons := constraints.MustParse("arch=amd64 instance-type=foo tags=bar cpu-power=10 cores=2 mem=1G virt-type=kvm")
	unsupported, err := validator.Validate(cons)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unsupported, tc.SameContents, []string{"cpu-power", "instance-type", "tags", "virt-type"})
}

func (s *environSuite) TestConstraintsValidatorInsideController(c *tc.C) {
	// Patch os.Args so it appears that we're running in "jujuagentd", and then
	// patch the host arch so it looks like we're running arm64.
	s.PatchValue(&os.Args, []string{"/some/where/containing/jujuagentd", "whatever"})
	s.PatchValue(&arch.HostArch, func() string { return arch.ARM64 })

	validator, err := s.env.ConstraintsValidator(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	cons := constraints.MustParse("arch=arm64")
	_, err = validator.Validate(cons)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *environSuite) TestPrecheck(c *tc.C) {
	// Patch os.Args so it appears that we're running in "jujuagentd", and then
	// patch the host arch so it looks like we're running amd64.
	s.PatchValue(&os.Args, []string{"/some/where/containing/jujuagentd", "whatever"})
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })

	constraint := constraints.MustParse("arch=amd64")

	// Prechecks with an explicit placement should fail
	err := s.env.PrecheckInstance(c.Context(), environs.PrecheckInstanceParams{
		Placement:   "42",
		Constraints: constraint,
	})
	c.Assert(err, tc.Not(tc.IsNil))
	c.Assert(err.Error(), tc.Equals, `use "juju add-machine ssh:[user@]<host>" to provision machines`)

	// Prechecks with no placement should work if the constraints match
	err = s.env.PrecheckInstance(c.Context(), environs.PrecheckInstanceParams{
		Constraints: constraint,
	})
	c.Assert(err, tc.ErrorIsNil)
}

type controllerInstancesSuite struct {
	baseEnvironSuite
}

func TestControllerInstancesSuite(t *testing.T) {
	tc.Run(t, &controllerInstancesSuite{})
}
func (s *controllerInstancesSuite) TestControllerInstances(c *tc.C) {
	var outputResult string
	var errResult error
	runSSHCommandTesting := func(host string, command []string, stdin string) (string, string, error) {
		return outputResult, "", errResult
	}
	s.PatchValue(&runSSHCommand, runSSHCommandTesting)

	type test struct {
		output      string
		err         error
		expectedErr string
	}
	tests := []test{{
		output: "",
	}, {
		output:      "no-agent-dir",
		expectedErr: "model is not bootstrapped",
	}, {
		output:      "woo",
		expectedErr: `unexpected output: "woo"`,
	}, {
		err:         errors.New("an error"),
		expectedErr: "an error",
	}}

	for i, test := range tests {
		c.Logf("test %d", i)
		outputResult = test.output
		errResult = test.err
		instances, err := s.env.ControllerInstances(c.Context(), "not-used")
		if test.expectedErr == "" {
			c.Assert(err, tc.ErrorIsNil)
			c.Assert(instances, tc.DeepEquals, []instance.Id{BootstrapInstanceId})
		} else {
			c.Assert(err, tc.ErrorMatches, test.expectedErr)
			c.Assert(instances, tc.HasLen, 0)
		}
	}
}

func (s *controllerInstancesSuite) TestControllerInstancesStderr(c *tc.C) {
	// Stderr should not affect the behaviour of ControllerInstances.
	testhelpers.PatchExecutable(c, s, "ssh", "#!/bin/sh\nhead -n1 > /dev/null; echo abc >&2; exit 0")
	_, err := s.env.ControllerInstances(c.Context(), "not-used")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *controllerInstancesSuite) TestControllerInstancesError(c *tc.C) {
	// If the ssh execution fails, its stderr will be captured in the error message.
	testhelpers.PatchExecutable(c, s, "ssh", "#!/bin/sh\nhead -n1 > /dev/null; echo abc >&2; exit 1")
	_, err := s.env.ControllerInstances(c.Context(), "not-used")
	c.Assert(err, tc.ErrorMatches, "abc: .*")
}

func (s *controllerInstancesSuite) TestControllerInstancesInternal(c *tc.C) {
	// Patch os.Args so it appears that we're running in "jujuagentd".
	s.PatchValue(&os.Args, []string{"/some/where/containing/jujuagentd", "whatever"})
	// Patch the ssh executable so that it would cause an error if we
	// were to call it.
	testhelpers.PatchExecutable(c, s, "ssh", "#!/bin/sh\nhead -n1 > /dev/null; echo abc >&2; exit 1")
	instances, err := s.env.ControllerInstances(c.Context(), "not-used")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(instances, tc.DeepEquals, []instance.Id{BootstrapInstanceId})
}

// fakeSSHScript runs only the archive-related setup commands in a temporary
// directory. The rest of the machine setup is recorded but not executed.
func fakeSSHScript(root, log, setupScript, mode string) string {
	return fmt.Sprintf(`#!/bin/bash --norc
set -euo pipefail
export root=%q log=%q setupScript=%q mode=%q
args=()
found=0
for arg in "$@"; do
	if [[ $found -eq 0 ]]; then
		if [[ "$arg" == ubuntu@* ]]; then
			found=1
		fi
	else
		args+=("$arg")
	fi
done
[[ $found == 1 ]]
if [[ "${args[0]}" == sudo ]]; then
	echo setup >> "$log"
	trap 'echo setup-end >> "$log"' EXIT
	runSetup() {
		set -euo pipefail
		unset -f cat
		while IFS= read -r line; do
			case "$line" in
			"install -D -m 600 /dev/null "*archive.tar.gz.partial*) echo install >> "$log" ;;
			"cat "*archive.tar.gz.partial*) echo copy >> "$log" ;;
			recovery_sha256=*) echo checksum >> "$log" ;;
			*"recovery archive checksum mismatch"*) ;;
			"mv -f "*archive.tar.gz.partial*) echo publish >> "$log" ;;
			"install -D -m 600 /dev/null "*"/recovery/params'"*) eval "$line"; continue ;;
			"echo 'archive-path:"*)
				paramsCommand="$line"
				while [[ "$line" != *"/recovery/params'" ]]; do
					IFS= read -r line
					paramsCommand+=$'\n'"$line"
				done
				eval "$paramsCommand"
				echo params >> "$log"
				continue
				;;
			*"jujuagentd recover-state"*) echo recover >> "$log"; continue ;;
			*) continue ;;
			esac
			line="${line//\/tmp\/juju-/$root\/juju-}"
			eval "$line"
		done < "$setupScript"
	}
	# Give the real wrapper a script that executes just the archive steps.
	cat() {
		command cat > "$setupScript"
		echo runSetup
	}
	rm() {
		if [[ $1 == -rf && $3 == /tmp/juju-* ]]; then
			echo cleanup >> "$log"
			if [[ $mode == fail-cleanup ]]; then
				echo 'cannot remove staging' >&2
				return 1
			fi
			command rm -rf -- "$root/${3##*/}"
		else
			command rm "$@"
		fi
	}
	export -f runSetup cat rm
	"${args[@]:1}"
	exit
fi
command="${args[*]}"
command="${command//\/tmp\/juju-/$root\/juju-}"
case "${args[0]}" in
	mkdir) echo mkdir >> "$log" ;;
	umask) echo stream >> "$log" ;;
	rm) echo cleanup >> "$log" ;;
	*) echo "unexpected command: $command" >&2; exit 1 ;;
esac
if [[ "${args[0]}" == rm && $mode == fail-cleanup ]]; then
	echo 'cannot remove staging' >&2
	exit 1
fi
eval "$command"
if [[ "${args[0]}" == umask && $mode == corrupt ]]; then
	staged="${args[6]//\'/}"
	staged="${staged/\/tmp\/juju-/$root\/juju-}"
	echo CORRUPT >> "$staged"
fi
`, root, log, setupScript, mode)
}

// readInvocationLog returns the tags logged by the fake ssh executable,
// in the order the remote end saw the invocations.
func readInvocationLog(c *tc.C, path string) []string {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	c.Assert(err, tc.ErrorIsNil)
	log := strings.TrimSpace(string(data))
	if log == "" {
		return nil
	}
	return strings.Split(log, "\n")
}

// TestBootstrapFinaliserStagesRecoveryArchive exercises the real unmanaged
// finaliser through cloud-init staging and verifies the published archive.
func (s *environSuite) TestBootstrapFinaliserStagesRecoveryArchive(c *tc.C) {
	s.assertBootstrapRecoveryArchive(c, "")
}

func (s *environSuite) TestBootstrapFinaliserRejectsCorruptedRecoveryArchive(c *tc.C) {
	s.assertBootstrapRecoveryArchive(c, "corrupt")
}

func (s *environSuite) TestBootstrapFinaliserReportsStagingCleanupFailure(c *tc.C) {
	s.assertBootstrapRecoveryArchive(c, "fail-cleanup")
}

func (s *environSuite) TestBootstrapFinaliserFreshBootstrapDoesNotStageArchive(c *tc.C) {
	s.assertBootstrapRecoveryArchive(c, "fresh")
}

func (s *environSuite) assertBootstrapRecoveryArchive(c *tc.C, mode string) {
	s.PatchValue(&sshprovisioner.CheckProvisioned,
		func(string, string) (bool, error) { return false, nil })
	s.PatchValue(&initUbuntuUser,
		func(string, string, string, string, io.Reader, io.Writer) error { return nil })
	amd64 := arch.AMD64
	ubuntuBase := base.MustParseBaseFromString("ubuntu@24.04")
	s.PatchValue(&sshprovisioner.DetectBaseAndHardwareCharacteristics,
		func(string, string) (instance.HardwareCharacteristics, base.Base, error) {
			return instance.HardwareCharacteristics{Arch: &amd64}, ubuntuBase, nil
		})

	localArchive := filepath.Join(c.MkDir(), "backup.tar.gz")
	payload := []byte("recovery archive payload")
	c.Assert(os.WriteFile(localArchive, payload, 0o600), tc.ErrorIsNil)
	digest := sha256.Sum256(payload)

	remoteRoot := c.MkDir()
	logPath := filepath.Join(c.MkDir(), "ssh.log")
	setupScript := filepath.Join(c.MkDir(), "setup.sh")
	testhelpers.PatchExecutable(c, s, "ssh", fakeSSHScript(remoteRoot, logPath, setupScript, mode))

	ctx := envtesting.BootstrapTestContext(c)
	result, err := s.env.Bootstrap(ctx, environs.BootstrapParams{})
	c.Assert(err, tc.ErrorIsNil)

	icfg, err := instancecfg.NewBootstrapInstanceConfig(
		coretesting.FakeControllerConfig(), constraints.Value{}, constraints.Value{},
		ubuntuBase, "", map[string]string{"test": "true"},
	)
	c.Assert(err, tc.ErrorIsNil)
	icfg.DataDir = filepath.Join(remoteRoot, "var", "lib", "juju")
	// Match managed bootstrap: setup replaces the initial host keys, so
	// cleanup must not reconnect with those initial keys afterwards.
	icfg.Bootstrap.InitialSSHHostKeys = instancecfg.SSHHostKeys{{
		PublicKeyAlgorithm: "ssh-ed25519", Public: "ssh-ed25519 AAAA", Private: "private",
	}}
	icfg.APIInfo = &api.Info{
		ModelTag: names.NewModelTag(s.env.Config().UUID()),
		CACert:   coretesting.CACert,
		Password: "secret",
	}
	icfg.Bootstrap.ControllerModelConfig = s.env.Config()
	icfg.Bootstrap.ControllerConfig = coretesting.FakeControllerConfig()
	icfg.Bootstrap.ControllerCloud = cloud.Cloud{Name: "manual", Type: "manual"}
	icfg.Bootstrap.ControllerAgentInfo = controller.ControllerAgentInfo{
		Cert: coretesting.ServerCert, PrivateKey: coretesting.ServerKey,
		CAPrivateKey: coretesting.CAKey, APIPort: 17070,
	}
	icfg.Bootstrap.AgentVersion = coretesting.FakeVersionNumber
	c.Assert(icfg.SetTools(tools.List{{
		Version: semversion.Binary{Number: coretesting.FakeVersionNumber, Release: "ubuntu", Arch: arch.AMD64},
		URL:     "https://example.invalid/agent.tgz",
		SHA256:  strings.Repeat("0", 64),
	}}), tc.ErrorIsNil)
	if mode != "fresh" {
		icfg.Bootstrap.RecoveryMode = true
		icfg.Bootstrap.RecoveryArchivePath = localArchive
		icfg.Bootstrap.RecoveryArchiveSHA256 = "sha256-" + hex.EncodeToString(digest[:])
	}

	err = result.CloudBootstrapFinalizer(ctx, icfg, environs.BootstrapDialOpts{})
	if err != nil {
		c.Logf("configuration error output:\n%s", ctx.GetStderr())
	}
	destination := filepath.Join(icfg.DataDir, "recovery", "archive.tar.gz")
	if mode == "fresh" {
		c.Assert(err, tc.ErrorIsNil)
		c.Check(readInvocationLog(c, logPath), tc.DeepEquals, []string{"setup", "setup-end"})
		script, err := os.ReadFile(setupScript)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(strings.Contains(string(script), "Regenerating SSH host keys"), tc.IsTrue)
		c.Check(strings.Contains(string(script), "jujuagentd bootstrap-state"), tc.IsTrue)
		c.Check(strings.Contains(string(script), "jujuagentd recover-state"), tc.IsFalse)
		return
	}
	if mode == "corrupt" {
		c.Check(err, tc.ErrorMatches, "subprocess encountered error code 1")
		stderr, ok := ctx.GetStderr().(*bytes.Buffer)
		c.Assert(ok, tc.IsTrue)
		c.Check(strings.Contains(stderr.String(), "recovery archive checksum mismatch"), tc.IsTrue)
		c.Check(readInvocationLog(c, logPath), tc.DeepEquals,
			[]string{"mkdir", "stream", "setup", "install", "copy", "checksum", "cleanup", "setup-end"})
		_, statErr := os.Stat(destination)
		c.Check(os.IsNotExist(statErr), tc.IsTrue)
		return
	}
	wantLog := []string{"mkdir", "stream", "setup", "install", "copy", "checksum", "publish", "params", "recover", "cleanup", "setup-end"}
	if mode == "fail-cleanup" {
		c.Assert(err, tc.NotNil)
		c.Check(err.Error(), tc.Contains, "subprocess encountered error code 1")
		c.Check(err.Error(), tc.Contains, "cleaning up staged files: removing staged files: cannot remove staging")
		wantLog = append(wantLog, "cleanup")
		stderr, ok := ctx.GetStderr().(*bytes.Buffer)
		c.Assert(ok, tc.IsTrue)
		c.Check(strings.Contains(stderr.String(), "cleaning up staged files failed"), tc.IsTrue)
	} else {
		c.Assert(err, tc.ErrorIsNil)
	}
	c.Check(readInvocationLog(c, logPath), tc.DeepEquals, wantLog)
	installed, err := os.ReadFile(destination)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(installed, tc.DeepEquals, payload)
	info, err := os.Stat(destination)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info.Mode().Perm(), tc.Equals, os.FileMode(0600))
	params, err := internalrecovery.ReadParams(icfg.DataDir)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(params.ArchivePath, tc.Equals, destination)
	c.Check(params.SHA256, tc.Equals, icfg.Bootstrap.RecoveryArchiveSHA256)
	script, err := os.ReadFile(setupScript)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(strings.Contains(string(script), "Regenerating SSH host keys"), tc.IsTrue)
	c.Check(strings.Contains(string(script), "archive-path: "+destination), tc.IsTrue)
	c.Check(strings.Index(string(script), "archive-path: "+destination) <
		strings.Index(string(script), "jujuagentd recover-state"), tc.IsTrue)
}
