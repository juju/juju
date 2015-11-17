// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"text/template"

	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/exec"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
)

type bridgeConfigSuite struct {
	coretesting.BaseSuite

	testConfig     string
	testConfigPath string
	testBridgeName string
	testBinPath    string
	testSBinPath   string
}

var _ = gc.Suite(&bridgeConfigSuite{})

func (s *bridgeConfigSuite) SetUpSuite(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("Skipping bridge config tests on windows")
	}
	s.BaseSuite.SetUpSuite(c)
}

func (s *bridgeConfigSuite) SetUpTest(c *gc.C) {
	s.testConfigPath = filepath.Join(c.MkDir(), "network-config")
	s.testConfig = "# test network config\n"
	s.testBridgeName = "test-bridge"
	s.testBinPath = c.MkDir()
	s.testSBinPath = c.MkDir()
	err := ioutil.WriteFile(s.testConfigPath, []byte(s.testConfig), 0644)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *bridgeConfigSuite) TestRenderScriptWhenParsingFails(c *gc.C) {
	result, err := renderScript(scriptArgs{}, "invalid", "{{foo}}")
	c.Assert(err, gc.ErrorMatches, `parsing template script "invalid": .* function "foo" not defined`)
	c.Assert(result, gc.Equals, "")
}

func (s *bridgeConfigSuite) TestRenderScriptWhenRenderingFails(c *gc.C) {
	result, err := renderScript(scriptArgs{}, "invalid", "{{.Extra}}")
	c.Assert(err, gc.ErrorMatches, `rendering script "invalid": .* Extra is not a field of struct .*`)
	c.Assert(result, gc.Equals, "")
}

func (s *bridgeConfigSuite) TestRenderScriptSucceedsEvenWithMissingOrExtraValues(c *gc.C) {
	args := scriptArgs{
		Config: "/my/conf",
		Commands: map[string]string{
			"cmd": "/path/good_cmd",
		},
		Scripts: map[string]string{
			"script": "some content",
		},
	}
	script := `
here is {{.Config}}!
I wish I had a >>{{.Bridge}}<< though.
calling {{.Commands.cmd}} is OK.
every script has {{.Scripts.script}}.
cannot call a {{.Commands.unknown}} command.
unknown scripts have {{.Scripts.extra}};
`
	expected := `
here is /my/conf!
I wish I had a >><< though.
calling /path/good_cmd is OK.
every script has some content.
cannot call a <no value> command.
unknown scripts have <no value>;
`
	result, err := renderScript(args, "sloppy", script)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.Equals, expected)
}

func (s *bridgeConfigSuite) TestPrepareScriptsAndArgs(c *gc.C) {
	args := s.makeTestArgs(c)
	c.Assert(args.Config, gc.Equals, s.testConfigPath)
	c.Assert(args.Bridge, gc.Equals, s.testBridgeName)
	// Just a cursory check if can find all the expected commands at the
	// expected paths.
	c.Assert(args.Commands, gc.HasLen, 5)
	for name, path := range args.Commands {
		if name == "Ping" {
			c.Check(path, gc.Equals, filepath.Join(s.testBinPath, "ping"))
		} else {
			c.Check(name, gc.Matches, `IP|IfConfig|IfUp|IfDown`,
				gc.Commentf("got unexpected command %q with path %q", name, path),
			)
			c.Check(path, jc.HasPrefix, s.testSBinPath)
			pathSuffix := strings.TrimPrefix(path, s.testSBinPath)
			c.Check(pathSuffix, gc.Matches, `/(ip|ifconfig|ifup|ifdown)`)
		}
	}
	// Only need to ensure the expected scripts are there and have been rendered
	// (i.e. they no longer have the same content as their templates). There are
	// separate tests for each script.
	c.Assert(args.Scripts, gc.HasLen, 7)
	for name, contents := range args.Scripts {
		switch name {
		case "get_gateway_and_primary_nic":
			c.Check(contents, gc.Not(gc.Equals), getGatewayAndPrimaryNICScript)
		case "dump_network_config":
			c.Check(contents, gc.Not(gc.Equals), dumpNetworkConfigScript)
		case "modify_network_config":
			c.Check(contents, gc.Not(gc.Equals), modifyNetworkConfigScript)
		case "revert_network_config":
			c.Check(contents, gc.Not(gc.Equals), revertNetworkConfigScript)
		case "setup_bridge_config":
			c.Check(contents, gc.Not(gc.Equals), setupBridgeConfigScript)
		case "ensure_bridge_connectivity":
			c.Check(contents, gc.Not(gc.Equals), ensureBridgeConnectivityScript)
		case "bridge_config_for_ip_version":
			c.Check(contents, gc.Not(gc.Equals), bridgeConfigForIPVersionScript)
		default:
			c.Errorf("got unexpected script %q with contents:\n%s", name, contents)
		}
	}
	// We fully test renderScript separately, including when parsing fails. The
	// only way prepareScriptsAndArgs could return an error is if any of the
	// scripts cannot be parsed, so no need to test this case again here.
}

func (s *bridgeConfigSuite) TestGetGatewayAndPrimaryNICScript(c *gc.C) {
	args := s.makeTestArgs(c)
	ipCommand := args.Commands["IP"]
	script := "get_gateway_and_primary_nic"

	// Run without parameters, verifying the script calls the commands as
	// expected.
	_, code := s.runScript(c, args, nil, script)
	c.Assert(code, gc.Equals, 0)
	gitjujutesting.AssertEchoArgs(c, ipCommand, "", "route", "list", "exact", "default")

	// Run with the expected parameters, verifying again how
	// the command was called.
	_, code = s.runScript(c, args, nil, script, "-4")
	c.Assert(code, gc.Equals, 0)
	gitjujutesting.AssertEchoArgs(c, ipCommand, "-4", "route", "list", "exact", "default")

	// Run with an empty command output to verify script handles that.
	patchExecutableAtPath(c, s, ipCommand, "", 0)
	output, code := s.runScript(c, args, nil, script)
	c.Assert(code, gc.Equals, 0)
	c.Check(output, gc.Equals, "")

	// Run with the expected command output to verify script processes it as
	// expected.
	patchExecutableAtPath(c, s, ipCommand, "default via 0.1.2.3 dev foo0", 0)
	output, code = s.runScript(c, args, nil, script, "-4")
	c.Assert(code, gc.Equals, 0)
	c.Check(output, gc.Equals, "0.1.2.3 foo0")

	// Finally, run the script and patch the command to return an error.
	patchExecutableAtPath(c, s, ipCommand, "one two three four five six", 255)
	output, code = s.runScript(c, args, nil, script)
	c.Assert(code, gc.Equals, 0)
	c.Check(output, gc.Equals, "three five")
}

func (s *bridgeConfigSuite) TestDumpNetworkConfigScript(c *gc.C) {
	args := s.makeTestArgs(c)
	ipCommand := args.Commands["IP"]
	ifconfigCommand := args.Commands["IfConfig"]
	script := "dump_network_config"

	assertCommandArgs := func() {
		gitjujutesting.AssertEchoArgs(c, ipCommand, "-B", "route", "show")
		gitjujutesting.AssertEchoArgs(c, ifconfigCommand, "-a")
		gitjujutesting.AssertEchoArgs(c, ipCommand, "-4", "address", "show")
		gitjujutesting.AssertEchoArgs(c, ipCommand, "-6", "address", "show")
	}

	// Run without parameters, verifying the script calls the commands as
	// expected.
	_, code := s.runScript(c, args, nil, script)
	c.Assert(code, gc.Equals, 0)
	assertCommandArgs()

	// Run with the unexpected extra arguments should not make a difference.
	_, code = s.runScript(c, args, nil, script, "foo", "bar", "baz")
	c.Assert(code, gc.Equals, 0)
	assertCommandArgs()

	normalOutputTemplate := `
Current networking configuration:
-------------------------------------------------------
Route table contents:
{{.IPOutput}}
-------------------------------------------------------
Network devices:
{{.IfConfigOutput}}
-------------------------------------------------------
Configured IPv4 addresses:
{{.IPOutput}}
-------------------------------------------------------
Configured IPv6 addresses:
{{.IPOutput}}
-------------------------------------------------------
Contents of {{.Config}}:
{{.ConfigContents}}
-------------------------------------------------------`[1:]
	outputTemplate := template.Must(template.New("output").Parse(normalOutputTemplate))
	outputArgs := map[string]string{
		"Config":         s.testConfigPath,
		"ConfigContents": s.testConfig,
	}

	assertScriptOutput := func(ipOutput, ifconfigOutput string, exitCode int) {
		patchExecutableAtPath(c, s, ipCommand, ipOutput, exitCode)
		patchExecutableAtPath(c, s, ifconfigCommand, ifconfigOutput, exitCode)
		outputArgs["IPOutput"] = ipOutput
		outputArgs["IfConfigOutput"] = ifconfigOutput

		output, code := s.runScript(c, args, nil, script)
		c.Assert(code, gc.Equals, 0)
		var expectedOutput bytes.Buffer
		err := outputTemplate.Execute(&expectedOutput, outputArgs)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(output, gc.Equals, expectedOutput.String())
	}

	// Run with empty command output to verify script handles that.
	assertScriptOutput("", "", 0)

	// Run with known command output to verify script processes it as expected.
	assertScriptOutput("ip command output", "ifconfig command output", 0)

	// Finally, run the script and patch the command to return an error to make
	// sure it's handled.
	assertScriptOutput("error: ip", "error: ifconfig", 42)
}

func (s *bridgeConfigSuite) TestRevertNetworkConfigScript(c *gc.C) {
	args := s.makeTestArgs(c)
	ipCommand := args.Commands["IP"]
	ifupCommand := args.Commands["IfUp"]
	script := "revert_network_config"

	// Patch ifup to return an error initially, to make sure the script
	// retries with --force the second time.
	patchExecutableAtPathAsEchoArgs(c, s, ifupCommand, 42)
	// Remove the config file to make sure the script still works.
	err := os.Remove(s.testConfigPath)
	c.Assert(err, jc.ErrorIsNil)

	// Run without parameters, verifying the script works as expected.
	_, code := s.runScript(c, args, nil, script)
	c.Assert(code, gc.Equals, 0)
	gitjujutesting.AssertEchoArgs(c, ipCommand, "link", "del", "dev", s.testBridgeName)
	gitjujutesting.AssertEchoArgs(c, ifupCommand, "-a", "-v")
	gitjujutesting.AssertEchoArgs(c, ifupCommand, "-a", "-v", "--force")

	// Create a backup config (ending with ".original") and the "modified"
	// config to verify the script restores the original and keeps the modified.
	err = ioutil.WriteFile(s.testConfigPath, []byte(s.testConfig), 0644)
	c.Assert(err, jc.ErrorIsNil)
	originalConfigPath := s.testConfigPath + ".original"
	originalConfig := s.testConfig + "\n# the original!"
	err = ioutil.WriteFile(originalConfigPath, []byte(originalConfig), 0644)
	c.Assert(err, jc.ErrorIsNil)

	// Run with extra arguments, verifying the script ignores them works the
	// same way, also check the output.
	output, code := s.runScript(c, args, nil, script, "foo", "bar", "baz")
	c.Assert(code, gc.Equals, 0)
	expectedOutput := fmt.Sprintf(`
Removing %[1]s, if it got created.
%[2]s "link" "del" "dev" "%[1]s"
Modified config saved to %[3]s.juju
Reverting changes to %[3]s to restore connectivity.
Bringing up all previously configured interfaces.
%[4]s "-a" "-v"
`[1:],
		s.testBridgeName, ipCommand, s.testConfigPath, ifupCommand,
	)
	c.Assert(output, gc.Equals, expectedOutput)

	// Verify the config files are where we expect.
	_, err = os.Stat(originalConfigPath)
	c.Assert(err, jc.Satisfies, os.IsNotExist)
	data, err := ioutil.ReadFile(s.testConfigPath)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, originalConfig)
	data, err = ioutil.ReadFile(s.testConfigPath + ".juju")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, s.testConfig)

	// Create again the ".original" config before running the script once more.
	err = ioutil.WriteFile(originalConfigPath, []byte(originalConfig), 0644)
	c.Assert(err, jc.ErrorIsNil)

	// Run with patched commands that always fail, verifying the script still
	// works and check the output.
	patchExecutableAtPath(c, s, ipCommand, "error: ip", 42)
	patchExecutableAtPath(c, s, ifupCommand, "error: ifup", 69)
	output, code = s.runScript(c, args, nil, script)
	c.Assert(code, gc.Equals, 0)

	expectedOutput = fmt.Sprintf(`
Removing %[1]s, if it got created.
error: ip
Modified config saved to %[2]s.juju
Reverting changes to %[2]s to restore connectivity.
Bringing up all previously configured interfaces.
error: ifup
error: ifup`[1:],
		s.testBridgeName, s.testConfigPath,
	)
	c.Assert(output, gc.Equals, expectedOutput)

	// Verify the ".original" got moved over testConfigPath.
	_, err = os.Stat(originalConfigPath)
	c.Assert(err, jc.Satisfies, os.IsNotExist)
	data, err = ioutil.ReadFile(s.testConfigPath)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, originalConfig)
}

func (s *bridgeConfigSuite) TestModifyNetworkConfigScriptInvalidParams(c *gc.C) {
	args := s.makeTestArgs(c)
	script := "modify_network_config"

	for i, test := range []struct {
		about  string
		params []string
	}{{
		about:  "no arguments",
		params: nil,
	}, {
		about:  "both arguments empty",
		params: []string{"", ""},
	}, {
		about:  "invalid address family, empty primary NIC",
		params: []string{"foo", ""},
	}, {
		about:  "empty address family, invalid primary NIC",
		params: []string{"", "bar"},
	}, {
		about:  "valid address familty, empty primary NIC",
		params: []string{"inet", ""},
	}, {
		about:  "valid address familty, invalid primary NIC",
		params: []string{"inet", "foo"},
	}, {
		about:  "valid, but mismatched address familty, valid primary NIC",
		params: []string{"inet6", "eth0"},
	}, {
		about:  "valid address familty, primary NIC has special characters",
		params: []string{"inet", ` eth !42@#$% ' \"`},
	}, {
		about:  "address family with special characters, valid primary NIC",
		params: []string{`!@ '$%^&*inet 69`, "eth0"},
	}, {
		about:  "both address family and primary NIC with special characters",
		params: []string{`!@ #'$%^&*\"inet 69`, ` eth !42@#$% ' \"`},
	}} {
		c.Logf("test #%d: %s", i, test.about)

		// Simple initial config.
		err := ioutil.WriteFile(s.testConfigPath, []byte(networkDHCPInitial), 0644)
		c.Check(err, jc.ErrorIsNil)

		// Run and check it fails.
		output, code := s.runScript(c, args, nil, script, test.params...)
		c.Check(code, gc.Equals, 1)
		c.Check(output, gc.Equals, "")

		// Verify the config was not modified.
		data, err := ioutil.ReadFile(s.testConfigPath)
		c.Check(err, jc.ErrorIsNil)
		c.Check(string(data), gc.Equals, networkDHCPInitial)
	}
}

func (s *bridgeConfigSuite) TestModifyNetworkConfigScriptModifications(c *gc.C) {
	args := s.makeTestArgs(c)
	script := "modify_network_config"

	renderConfigTemplate := func(configTemplate, addressFamily, primaryNIC, bridgeName string) string {
		t := template.Must(template.New("config").Parse(configTemplate))
		data := map[string]string{
			"AF":     addressFamily,
			"NIC":    primaryNIC,
			"Bridge": bridgeName,
		}
		var buf bytes.Buffer
		err := t.Execute(&buf, data)
		c.Check(err, jc.ErrorIsNil)
		return buf.String()
	}

	checkScript := func(initialTemplate, modifiedTemplate, addrFamily, nic string) {
		// Render the templates and save initial config.
		initial := renderConfigTemplate(initialTemplate, addrFamily, nic, s.testBridgeName)
		modified := renderConfigTemplate(modifiedTemplate, addrFamily, nic, s.testBridgeName)
		err := ioutil.WriteFile(s.testConfig, []byte(initial), 0644)
		c.Check(err, jc.ErrorIsNil)

		// Run the script and verify the modified config.
		output, code := s.runScript(c, args, nil, script, addrFamily, nic)
		c.Check(code, gc.Equals, 0)
		c.Check(output, gc.Equals, "")
		data, err := ioutil.ReadFile(s.testConfig)
		c.Check(err, jc.ErrorIsNil)
		c.Check(string(data), gc.Equals, modified)
	}

	for i, test := range []struct {
		about            string
		initialTemplate  string
		modifiedTemplate string
	}{{
		about: "simple single-NIC DHCP config",
		initialTemplate: `
auto lo
iface lo inet loopback

auto {{.NIC}}
iface {{.NIC}} {{.AF}} dhcp
`,
		modifiedTemplate: `
auto lo
iface lo inet loopback

iface {{.NIC}} {{.AF}} manual

auto {{.Bridge}}
iface {{.Bridge}} {{.AF}} dhcp
    bridge_ports {{.NIC}}
`,
	}} {
		c.Logf("test #%d: %s", i, test.about)

		// To cover more cases for each config, we need to check both "inet" and
		// "inet6" address families, as well as two different primary NIC names.
		checkScript(test.initialTemplate, test.modifiedTemplate, "inet", "eth0")
		checkScript(test.initialTemplate, test.modifiedTemplate, "inet", "eth1")
		checkScript(test.initialTemplate, test.modifiedTemplate, "inet6", "eth0")
		checkScript(test.initialTemplate, test.modifiedTemplate, "inet6", "eth1")
	}
}

func (s *bridgeConfigSuite) TestSetupBridgeConfigScript(c *gc.C) {
	args := s.makeTestArgs(c)
	ipCommand := args.Commands["IP"]
	//ifupCommand := args.Commands["IfUp"]
	//ifdownCommand := args.Commands["IfDown"]
	script := "setup_bridge_config"
	dependsOn := []string{"modify_network_config"}

	// Run without parameters, verifying the script works as expected.
	output, code := s.runScript(c, args, dependsOn, script)
	c.Assert(code, gc.Equals, 1)
	c.Assert(output, gc.Equals, "foo")
	gitjujutesting.AssertEchoArgs(c, ipCommand, "", "route", "list", "exact", "default")

	// Run with the expected parameters, verifying again how
	// the command was called.
	_, code = s.runScript(c, args, dependsOn, script, "-4")
	c.Assert(code, gc.Equals, 0)
	gitjujutesting.AssertEchoArgs(c, ipCommand, "-4", "route", "list", "exact", "default")

	// Run with an empty command output to verify script handles that.
	patchExecutableAtPath(c, s, ipCommand, "", 0)
	output, code = s.runScript(c, args, dependsOn, script)
	c.Assert(code, gc.Equals, 0)
	c.Check(output, gc.Equals, "")

	// Run with the expected command output to verify script processes it as
	// expected.
	patchExecutableAtPath(c, s, ipCommand, "default via 0.1.2.3 dev foo0", 0)
	output, code = s.runScript(c, args, dependsOn, script, "-4")
	c.Assert(code, gc.Equals, 0)
	c.Check(output, gc.Equals, "0.1.2.3 foo0")

	// Finally, run the script and patch the command to return an error.
	patchExecutableAtPath(c, s, ipCommand, "one two three four five six", 255)
	output, code = s.runScript(c, args, dependsOn, script)
	c.Assert(code, gc.Equals, 0)
	c.Check(output, gc.Equals, "three five")
}

func (s *bridgeConfigSuite) TestEnsureBridgeConnectivityScript(c *gc.C) {
	//args := s.makeTestArgs(c)
	//script := "ensure_bridge_connectivity"
}

func (s *bridgeConfigSuite) TestBridgeConfigForIPVersionScript(c *gc.C) {
	//args := s.makeTestArgs(c)
	//script := "bridge_config_for_ip_version"
}

func (s *bridgeConfigSuite) makeTestArgs(c *gc.C) scriptArgs {
	args, err := prepareScriptsAndArgs(s.testConfigPath, s.testBridgeName, s.testSBinPath, s.testBinPath)
	c.Assert(err, jc.ErrorIsNil)

	// Patch all commands to isolate tests properly, and to allow checking the
	// arguments the commands were executed with. All commands initially are
	// patched to always return exit code 0 (the default when not specified
	// explicitly).
	for _, path := range args.Commands {
		patchExecutableAtPathAsEchoArgs(c, s, path)
	}
	return args
}

// TODO(dimitern): Refactor those scripts and patch* functions below, making
// them more generic (where needed; also make them work on Windows if possible)
// and move all of it to juju/testing.

// This script needed patching, as EchoQuotedArgsUnix assumed the .out and
// .exitcodes files should be written in the CWD, rather than alonside the
// patched executables.
const patchedEchoQuotedArgsUnix = `#!/bin/bash --norc
dir=$(dirname $0)
name=$(basename $0)
argfile="$dir/$name.out"
exitcodesfile="$dir/$name.exitcodes"
printf "%s" "$dir/$name" | tee -a $argfile
for arg in "$@"; do
  printf " \"%s\""  "$arg" | tee -a $argfile
done
printf "\n" | tee -a $argfile
if [ -f $exitcodesfile ]
then
	exitcodes=$(cat $exitcodesfile)
	arr=(${exitcodes/;/ })
	echo ${arr[1]} | tee $exitcodesfile
	exit ${arr[0]}
fi
`

// This is a more generic version of the script used by PatchExecutableThrowError.
const simpleEchoAndExitScript = `#!/bin/bash --norc
echo %s
exit %d
`

func patchExecutableAtPathAsEchoArgs(
	c *gc.C,
	patcher gitjujutesting.CleanupPatcher,
	execFullPath string,
	exitCodes ...int,
) {
	// Ensure no output and exit codes files exist first.
	outputFilePath := execFullPath + ".out"
	exitCodesFilePath := execFullPath + ".exitcodes"
	os.Remove(outputFilePath)
	os.Remove(exitCodesFilePath)

	// Render a script compatible with AssertEchoArgs(), collecting the output
	// and exit codes when the executable is run.
	err := ioutil.WriteFile(execFullPath, []byte(patchedEchoQuotedArgsUnix), 0755)
	c.Assert(err, jc.ErrorIsNil)

	if len(exitCodes) > 0 {
		codes := make([]string, len(exitCodes))
		for i, code := range exitCodes {
			codes[i] = strconv.Itoa(code)
		}
		s := strings.Join(codes, ";") + ";"
		err = ioutil.WriteFile(exitCodesFilePath, []byte(s), 0644)
		c.Assert(err, gc.IsNil)
	}

	// Cleanup artifacts after the test.
	patcher.AddCleanup(func(*gc.C) {
		os.Remove(execFullPath)
		os.Remove(outputFilePath)
		os.Remove(exitCodesFilePath)
	})
}

func patchExecutableAtPath(
	c *gc.C,
	patcher gitjujutesting.CleanupPatcher,
	execFullPath string,
	scriptOutput string,
	scriptExitCode int,
) {
	// Render a simple script printing scriptOutput and returning the scriptExitCode.
	script := fmt.Sprintf(simpleEchoAndExitScript, scriptOutput, scriptExitCode)
	err := ioutil.WriteFile(execFullPath, []byte(script), 0755)
	c.Assert(err, jc.ErrorIsNil)

	// Cleanup artifacts after the test.
	patcher.AddCleanup(func(*gc.C) {
		os.Remove(execFullPath)
	})
}

func (s *bridgeConfigSuite) runScript(
	c *gc.C,
	args scriptArgs,
	dependsOnScripts []string,
	scriptName string,
	scriptParams ...string,
) (
	combinedOutput string,
	exitCode int,
) {
	scriptNames := []string{scriptName}
	if len(dependsOnScripts) > 0 {
		scriptNames = append(scriptNames, dependsOnScripts...)
	}

	// Ensure the script and any of its dependencies were rendered earlier.
	scriptsToRender := make([]string, len(scriptNames))
	c.Assert(args.Scripts, gc.Not(gc.HasLen), 0)
	for i, name := range scriptNames {
		script, ok := args.Scripts[name]
		c.Assert(ok, jc.IsTrue)
		c.Assert(script, gc.Not(gc.Equals), "")
		scriptsToRender[i] = script
	}

	// Ensure all commands the script might call exist, as they should have been
	// configured to return expected exit codes and/or outputs.
	c.Assert(args.Commands, gc.Not(gc.HasLen), 0)
	for command, path := range args.Commands {
		c.Assert(command, gc.Not(gc.Equals), "")
		c.Assert(path, jc.IsNonEmptyFile)
	}

	// Surround any params in double quotes before calling.
	callParams := []string{scriptName}
	for _, param := range scriptParams {
		callParams = append(callParams, utils.ShQuote(param))
	}

	testScript := strings.Join(scriptsToRender, "\n")
	testScript += "\n" + strings.Join(callParams, " ")
	result, err := exec.RunCommands(exec.RunParams{Commands: testScript})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("script %q failed unexpectedly", scriptName))
	// To simplify most cases, trim any trailing new lines, but still separate
	// the stdout and stderr (in that order) with a new line, if both are
	// non-empty.
	stdout := strings.TrimSuffix(string(result.Stdout), "\n")
	stderr := strings.TrimSuffix(string(result.Stderr), "\n")
	if stderr != "" {
		return stdout + "\n" + stderr, result.Code
	}
	return stdout, result.Code
}

// The rest of the file contains various forms of /etc/network/interfaces file -
// before and after modifying it by the scripts. Used in
// TestModifyNetworkConfigScriptModifications.

const networkStaticInitial = `auto lo
iface lo inet loopback

auto eth0
iface eth0 inet static
    address 1.2.3.4
    netmask 255.255.255.0
    gateway 4.3.2.1`

const networkStaticFinal = `auto lo
iface lo inet loopback

auto juju-br0
iface juju-br0 inet static
    bridge_ports eth0
    address 1.2.3.4
    netmask 255.255.255.0
    gateway 4.3.2.1
# Primary interface (defining the default route)
iface eth0 inet manual
`

const networkDHCPInitial = `auto lo
iface lo inet loopback

auto eth0
iface eth0 inet dhcp`

const networkDHCPFinal = `auto lo
iface lo inet loopback



# Primary interface (defining the default route)
iface eth0 inet manual

# Bridge to use for LXC/KVM containers
auto juju-br0
iface juju-br0 inet dhcp
    bridge_ports eth0
`

const networkMultipleInitial = networkStaticInitial + `
auto eth1
iface eth1 inet static
    address 1.2.3.5
    netmask 255.255.255.0
    gateway 4.3.2.1`

const networkMultipleFinal = `auto lo
iface lo inet loopback

auto juju-br0
iface juju-br0 inet static
    bridge_ports eth0
    address 1.2.3.4
    netmask 255.255.255.0
    gateway 4.3.2.1
auto eth1
iface eth1 inet static
    address 1.2.3.5
    netmask 255.255.255.0
    gateway 4.3.2.1
# Primary interface (defining the default route)
iface eth0 inet manual
`

const networkWithAliasInitial = networkStaticInitial + `
auto eth0:1
iface eth0:1 inet static
    address 1.2.3.5`

const networkWithAliasFinal = `auto lo
iface lo inet loopback

auto juju-br0
iface juju-br0 inet static
    bridge_ports eth0
    address 1.2.3.4
    netmask 255.255.255.0
    gateway 4.3.2.1
auto eth0:1
iface eth0:1 inet static
    address 1.2.3.5
# Primary interface (defining the default route)
iface eth0 inet manual
`
const networkDHCPWithAliasInitial = `auto lo
iface lo inet loopback

auto eth0
iface eth0 inet static
    gateway 10.14.0.1
    address 10.14.0.102/24

auto eth0:1
iface eth0:1 inet static
    address 10.14.0.103/24

auto eth0:2
iface eth0:2 inet static
    address 10.14.0.100/24

dns-nameserver 192.168.1.142`

const networkDHCPWithAliasFinal = `auto lo
iface lo inet loopback

auto juju-br0
iface juju-br0 inet static
    bridge_ports eth0
    gateway 10.14.0.1
    address 10.14.0.102/24

auto eth0:1
iface eth0:1 inet static
    address 10.14.0.103/24

auto eth0:2
iface eth0:2 inet static
    address 10.14.0.100/24

dns-nameserver 192.168.1.142
# Primary interface (defining the default route)
iface eth0 inet manual
`
