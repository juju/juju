// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"io/ioutil"
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/sync"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/environs/tools"
	toolstesting "github.com/juju/juju/environs/tools/testing"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

type UpgradeJujuSuite struct {
	testing.JujuConnSuite
	toolsDir string
}

var _ = gc.Suite(&UpgradeJujuSuite{})

var upgradeJujuTests = []struct {
	about          string
	tools          []string
	currentVersion string
	agentVersion   string

	args           []string
	expectInitErr  string
	expectErr      string
	expectVersion  string
	expectUploaded []string
}{{
	about:          "unwanted extra argument",
	currentVersion: "1.0.0-quantal-amd64",
	args:           []string{"foo"},
	expectInitErr:  "unrecognized args:.*",
}, {
	about:          "removed arg --dev specified",
	currentVersion: "1.0.0-quantal-amd64",
	args:           []string{"--dev"},
	expectInitErr:  "flag provided but not defined: --dev",
}, {
	about:          "invalid --version value",
	currentVersion: "1.0.0-quantal-amd64",
	args:           []string{"--version", "invalid-version"},
	expectInitErr:  "invalid version .*",
}, {
	about:          "just major version, no minor specified",
	currentVersion: "4.2.0-quantal-amd64",
	args:           []string{"--version", "4"},
	expectInitErr:  `invalid version "4"`,
}, {
	about:          "major version upgrade to incompatible version",
	currentVersion: "2.0.0-quantal-amd64",
	args:           []string{"--version", "5.2.0"},
	expectInitErr:  "cannot upgrade to version incompatible with CLI",
}, {
	about:          "major version downgrade to incompatible version",
	currentVersion: "4.2.0-quantal-amd64",
	args:           []string{"--version", "3.2.0"},
	expectInitErr:  "cannot upgrade to version incompatible with CLI",
}, {
	about:          "invalid --series",
	currentVersion: "4.2.0-quantal-amd64",
	args:           []string{"--series", "precise&quantal"},
	expectInitErr:  `invalid value "precise&quantal" for flag --series: .*`,
}, {
	about:          "--series without --upload-tools",
	currentVersion: "4.2.0-quantal-amd64",
	args:           []string{"--series", "precise,quantal"},
	expectInitErr:  "--series requires --upload-tools",
}, {
	about:          "--upload-tools with inappropriate version 1",
	currentVersion: "4.2.0-quantal-amd64",
	args:           []string{"--upload-tools", "--version", "3.1.0"},
	expectInitErr:  "cannot upgrade to version incompatible with CLI",
}, {
	about:          "--upload-tools with inappropriate version 2",
	currentVersion: "3.2.7-quantal-amd64",
	args:           []string{"--upload-tools", "--version", "3.2.8.4"},
	expectInitErr:  "cannot specify build number when uploading tools",
}, {
	about:          "latest supported stable release",
	tools:          []string{"2.1.0-quantal-amd64", "2.1.2-quantal-i386", "2.1.3-quantal-amd64", "2.1-dev1-quantal-amd64"},
	currentVersion: "2.0.0-quantal-amd64",
	agentVersion:   "2.0.0",
	expectVersion:  "2.1.3",
}, {
	about:          "latest current release",
	tools:          []string{"2.0.5-quantal-amd64", "2.0.1-quantal-i386", "2.3.3-quantal-amd64"},
	currentVersion: "2.0.0-quantal-amd64",
	agentVersion:   "2.0.0",
	expectVersion:  "2.0.5",
}, {
	about:          "latest current release matching CLI, major version",
	tools:          []string{"3.2.0-quantal-amd64"},
	currentVersion: "3.2.0-quantal-amd64",
	agentVersion:   "2.8.2",
	expectVersion:  "3.2.0",
}, {
	about:          "latest current release matching CLI, major version, no matching major tools",
	tools:          []string{"2.8.2-quantal-amd64"},
	currentVersion: "3.2.0-quantal-amd64",
	agentVersion:   "2.8.2",
	expectErr:      "no matching tools available",
}, {
	about:          "latest current release matching CLI, major version, no matching tools",
	tools:          []string{"3.3.0-quantal-amd64"},
	currentVersion: "3.2.0-quantal-amd64",
	agentVersion:   "2.8.2",
	expectErr:      "no compatible tools available",
}, {
	about:          "no next supported available",
	tools:          []string{"2.2.0-quantal-amd64", "2.2.5-quantal-i386", "2.3.3-quantal-amd64", "2.1-dev1-quantal-amd64"},
	currentVersion: "2.0.0-quantal-amd64",
	agentVersion:   "2.0.0",
	expectErr:      "no more recent supported versions available",
}, {
	about:          "latest supported stable, when client is dev",
	tools:          []string{"2.1-dev1-quantal-amd64", "2.1.0-quantal-amd64", "2.3-dev0-quantal-amd64", "3.0.1-quantal-amd64"},
	currentVersion: "2.1-dev0-quantal-amd64",
	agentVersion:   "2.0.0",
	expectVersion:  "2.1.0",
}, {
	about:          "latest current, when agent is dev",
	tools:          []string{"2.1-dev1-quantal-amd64", "2.2.0-quantal-amd64", "2.3-dev0-quantal-amd64", "3.0.1-quantal-amd64"},
	currentVersion: "2.0.0-quantal-amd64",
	agentVersion:   "2.1-dev0",
	expectVersion:  "2.2.0",
}, {
	about:          "specified version",
	tools:          []string{"2.3-dev0-quantal-amd64"},
	currentVersion: "2.0.0-quantal-amd64",
	agentVersion:   "2.0.0",
	args:           []string{"--version", "2.3-dev0"},
	expectVersion:  "2.3-dev0",
}, {
	about:          "specified major version",
	tools:          []string{"3.2.0-quantal-amd64"},
	currentVersion: "3.2.0-quantal-amd64",
	agentVersion:   "2.8.2",
	args:           []string{"--version", "3.2.0"},
	expectVersion:  "3.2.0",
}, {
	about:          "specified version missing, but already set",
	currentVersion: "3.0.0-quantal-amd64",
	agentVersion:   "3.0.0",
	args:           []string{"--version", "3.0.0"},
	expectVersion:  "3.0.0",
}, {
	about:          "specified version, no tools",
	currentVersion: "3.0.0-quantal-amd64",
	agentVersion:   "3.0.0",
	args:           []string{"--version", "3.2.0"},
	expectErr:      "no tools available",
}, {
	about:          "specified version, no matching major version",
	tools:          []string{"4.2.0-quantal-amd64"},
	currentVersion: "3.0.0-quantal-amd64",
	agentVersion:   "3.0.0",
	args:           []string{"--version", "3.2.0"},
	expectErr:      "no matching tools available",
}, {
	about:          "specified version, no matching minor version",
	tools:          []string{"3.4.0-quantal-amd64"},
	currentVersion: "3.0.0-quantal-amd64",
	agentVersion:   "3.0.0",
	args:           []string{"--version", "3.2.0"},
	expectErr:      "no matching tools available",
}, {
	about:          "specified version, no matching patch version",
	tools:          []string{"3.2.5-quantal-amd64"},
	currentVersion: "3.0.0-quantal-amd64",
	agentVersion:   "3.0.0",
	args:           []string{"--version", "3.2.0"},
	expectErr:      "no matching tools available",
}, {
	about:          "specified version, no matching build version",
	tools:          []string{"3.2.0.2-quantal-amd64"},
	currentVersion: "3.0.0-quantal-amd64",
	agentVersion:   "3.0.0",
	args:           []string{"--version", "3.2.0"},
	expectErr:      "no matching tools available",
}, {
	about:          "major version downgrade to incompatible version",
	tools:          []string{"3.2.0-quantal-amd64"},
	currentVersion: "3.2.0-quantal-amd64",
	agentVersion:   "4.2.0",
	args:           []string{"--version", "3.2.0"},
	expectErr:      "cannot change version from 4.2.0 to 3.2.0",
}, {
	about:          "minor version downgrade to incompatible version",
	tools:          []string{"3.2.0-quantal-amd64"},
	currentVersion: "3.2.0-quantal-amd64",
	agentVersion:   "3.3-dev0",
	args:           []string{"--version", "3.2.0"},
	expectErr:      "cannot change version from 3.3-dev0 to 3.2.0",
}, {
	about:          "nothing available",
	currentVersion: "2.0.0-quantal-amd64",
	agentVersion:   "2.0.0",
	expectVersion:  "2.0.0",
}, {
	about:          "nothing available 2",
	currentVersion: "2.0.0-quantal-amd64",
	tools:          []string{"3.2.0-quantal-amd64"},
	agentVersion:   "2.0.0",
	expectVersion:  "2.0.0",
}, {
	about:          "upload with default series",
	currentVersion: "2.2.0-quantal-amd64",
	agentVersion:   "2.0.0",
	args:           []string{"--upload-tools"},
	expectVersion:  "2.2.0.1",
	expectUploaded: []string{"2.2.0.1-quantal-amd64", "2.2.0.1-%LTS%-amd64", "2.2.0.1-raring-amd64"},
}, {
	about:          "upload with explicit version",
	currentVersion: "2.2.0-quantal-amd64",
	agentVersion:   "2.0.0",
	args:           []string{"--upload-tools", "--version", "2.7.3"},
	expectVersion:  "2.7.3.1",
	expectUploaded: []string{"2.7.3.1-quantal-amd64", "2.7.3.1-%LTS%-amd64", "2.7.3.1-raring-amd64"},
}, {
	about:          "upload with explicit series",
	currentVersion: "2.2.0-quantal-amd64",
	agentVersion:   "2.0.0",
	args:           []string{"--upload-tools", "--series", "raring"},
	expectVersion:  "2.2.0.1",
	expectUploaded: []string{"2.2.0.1-quantal-amd64", "2.2.0.1-raring-amd64"},
}, {
	about:          "upload dev version, currently on release version",
	currentVersion: "2.1.0-quantal-amd64",
	agentVersion:   "2.0.0",
	args:           []string{"--upload-tools"},
	expectVersion:  "2.1.0.1",
	expectUploaded: []string{"2.1.0.1-quantal-amd64", "2.1.0.1-%LTS%-amd64", "2.1.0.1-raring-amd64"},
}, {
	about:          "upload bumps version when necessary",
	tools:          []string{"2.4.6-quantal-amd64", "2.4.8-quantal-amd64"},
	currentVersion: "2.4.6-quantal-amd64",
	agentVersion:   "2.4.0",
	args:           []string{"--upload-tools"},
	expectVersion:  "2.4.6.1",
	expectUploaded: []string{"2.4.6.1-quantal-amd64", "2.4.6.1-%LTS%-amd64", "2.4.6.1-raring-amd64"},
}, {
	about:          "upload re-bumps version when necessary",
	tools:          []string{"2.4.6-quantal-amd64", "2.4.6.2-saucy-i386", "2.4.8-quantal-amd64"},
	currentVersion: "2.4.6-quantal-amd64",
	agentVersion:   "2.4.6.2",
	args:           []string{"--upload-tools"},
	expectVersion:  "2.4.6.3",
	expectUploaded: []string{"2.4.6.3-quantal-amd64", "2.4.6.3-%LTS%-amd64", "2.4.6.3-raring-amd64"},
}, {
	about:          "upload with explicit version bumps when necessary",
	currentVersion: "2.2.0-quantal-amd64",
	tools:          []string{"2.7.3.1-quantal-amd64"},
	agentVersion:   "2.0.0",
	args:           []string{"--upload-tools", "--version", "2.7.3"},
	expectVersion:  "2.7.3.2",
	expectUploaded: []string{"2.7.3.2-quantal-amd64", "2.7.3.2-%LTS%-amd64", "2.7.3.2-raring-amd64"},
}}

func (s *UpgradeJujuSuite) TestUpgradeJuju(c *gc.C) {
	oldVersion := version.Current
	defer func() {
		version.Current = oldVersion
	}()

	for i, test := range upgradeJujuTests {
		c.Logf("\ntest %d: %s", i, test.about)
		s.Reset(c)
		tools.DefaultBaseURL = ""

		// Set up apparent CLI version and initialize the command.
		version.Current = version.MustParseBinary(test.currentVersion)
		com := &UpgradeJujuCommand{}
		if err := coretesting.InitCommand(envcmd.Wrap(com), test.args); err != nil {
			if test.expectInitErr != "" {
				c.Check(err, gc.ErrorMatches, test.expectInitErr)
			} else {
				c.Check(err, gc.IsNil)
			}
			continue
		}

		// Set up state and environ, and run the command.
		toolsDir := c.MkDir()
		updateAttrs := map[string]interface{}{
			"agent-version":      test.agentVersion,
			"agent-metadata-url": "file://" + toolsDir + "/tools",
		}
		err := s.State.UpdateEnvironConfig(updateAttrs, nil, nil)
		c.Assert(err, gc.IsNil)
		versions := make([]version.Binary, len(test.tools))
		for i, v := range test.tools {
			versions[i] = version.MustParseBinary(v)
		}
		if len(versions) > 0 {
			stor, err := filestorage.NewFileStorageWriter(toolsDir)
			c.Assert(err, gc.IsNil)
			envtesting.MustUploadFakeToolsVersions(stor, s.Environ.Config().AgentStream(), versions...)
		}

		err = com.Run(coretesting.Context(c))
		if test.expectErr != "" {
			c.Check(err, gc.ErrorMatches, test.expectErr)
			continue
		} else if !c.Check(err, gc.IsNil) {
			continue
		}

		// Check expected changes to environ/state.
		cfg, err := s.State.EnvironConfig()
		c.Check(err, gc.IsNil)
		agentVersion, ok := cfg.AgentVersion()
		c.Check(ok, gc.Equals, true)
		c.Check(agentVersion, gc.Equals, version.MustParse(test.expectVersion))

		for _, uploaded := range test.expectUploaded {
			// Substitute latest LTS for placeholder in expected series for uploaded tools
			uploaded = strings.Replace(uploaded, "%LTS%", config.LatestLtsSeries(), 1)
			vers := version.MustParseBinary(uploaded)
			s.checkToolsUploaded(c, vers, agentVersion)
		}
	}
}

func (s *UpgradeJujuSuite) checkToolsUploaded(c *gc.C, vers version.Binary, agentVersion version.Number) {
	storage, err := s.State.ToolsStorage()
	c.Assert(err, gc.IsNil)
	defer storage.Close()
	_, r, err := storage.Tools(vers)
	if !c.Check(err, gc.IsNil) {
		return
	}
	data, err := ioutil.ReadAll(r)
	r.Close()
	c.Check(err, gc.IsNil)
	expectContent := version.Current
	expectContent.Number = agentVersion
	checkToolsContent(c, data, "jujud contents "+expectContent.String())
}

func checkToolsContent(c *gc.C, data []byte, uploaded string) {
	zr, err := gzip.NewReader(bytes.NewReader(data))
	c.Check(err, gc.IsNil)
	defer zr.Close()
	tr := tar.NewReader(zr)
	found := false
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		c.Check(err, gc.IsNil)
		if strings.ContainsAny(hdr.Name, "/\\") {
			c.Fail()
		}
		if hdr.Typeflag != tar.TypeReg {
			c.Fail()
		}
		content, err := ioutil.ReadAll(tr)
		c.Check(err, gc.IsNil)
		c.Check(string(content), gc.Equals, uploaded)
		found = true
	}
	c.Check(found, jc.IsTrue)
}

// JujuConnSuite very helpfully uploads some default
// tools to the environment's storage. We don't want
// 'em there; but we do want a consistent default-series
// in the environment state.
func (s *UpgradeJujuSuite) Reset(c *gc.C) {
	s.JujuConnSuite.Reset(c)
	envtesting.RemoveTools(c, s.DefaultToolsStorage, s.Environ.Config().AgentStream())
	updateAttrs := map[string]interface{}{
		"default-series": "raring",
		"agent-version":  "1.2.3",
	}
	err := s.State.UpdateEnvironConfig(updateAttrs, nil, nil)
	c.Assert(err, gc.IsNil)
	s.PatchValue(&sync.BuildToolsTarball, toolstesting.GetMockBuildTools(c))

	// Set API host ports so FindTools works.
	hostPorts := [][]network.HostPort{{{
		Address: network.NewAddress("0.1.2.3", network.ScopeUnknown),
		Port:    1234,
	}}}
	err = s.State.SetAPIHostPorts(hostPorts)
	c.Assert(err, gc.IsNil)
}

func (s *UpgradeJujuSuite) TestUpgradeJujuWithRealUpload(c *gc.C) {
	s.Reset(c)
	cmd := envcmd.Wrap(&UpgradeJujuCommand{})
	_, err := coretesting.RunCommand(c, cmd, "--upload-tools")
	c.Assert(err, gc.IsNil)
	vers := version.Current
	vers.Build = 1
	s.checkToolsUploaded(c, vers, vers.Number)
	//_, err = envtools.FindExactTools(s.Environ, vers.Number, vers.Series, vers.Arch)
	//c.Assert(err, gc.IsNil)
}

type DryRunTest struct {
	about             string
	cmdArgs           []string
	tools             []string
	currentVersion    string
	agentVersion      string
	expectedCmdOutput string
}

func (s *UpgradeJujuSuite) TestUpgradeDryRun(c *gc.C) {
	tests := []DryRunTest{
		DryRunTest{
			about:          "dry run outputs and doesn't change anything when uploading tools",
			cmdArgs:        []string{"--upload-tools", "--dry-run"},
			tools:          []string{"2.1.0-quantal-amd64", "2.1.2-quantal-i386", "2.1.3-quantal-amd64", "2.1-dev1-quantal-amd64", "2.2.3-quantal-amd64"},
			currentVersion: "2.0.0-quantal-amd64",
			agentVersion:   "2.0.0",
			expectedCmdOutput: `available tools:
    2.1-dev1-quantal-amd64
    2.1.0-quantal-amd64
    2.1.2-quantal-i386
    2.1.3-quantal-amd64
    2.2.3-quantal-amd64
best version:
    2.1.3
upgrade to this version by running
    juju upgrade-juju --version="2.1.3"
`,
		},
		DryRunTest{
			about:          "dry run outputs and doesn't change anything",
			cmdArgs:        []string{"--dry-run"},
			tools:          []string{"2.1.0-quantal-amd64", "2.1.2-quantal-i386", "2.1.3-quantal-amd64", "2.1-dev1-quantal-amd64", "2.2.3-quantal-amd64"},
			currentVersion: "2.0.0-quantal-amd64",
			agentVersion:   "2.0.0",
			expectedCmdOutput: `available tools:
    2.1-dev1-quantal-amd64
    2.1.0-quantal-amd64
    2.1.2-quantal-i386
    2.1.3-quantal-amd64
    2.2.3-quantal-amd64
best version:
    2.1.3
upgrade to this version by running
    juju upgrade-juju --version="2.1.3"
`,
		},
	}

	for i, test := range tests {
		c.Logf("\ntest %d: %s", i, test.about)
		s.Reset(c)
		tools.DefaultBaseURL = ""

		s.PatchValue(&version.Current, version.MustParseBinary(test.currentVersion))
		com := &UpgradeJujuCommand{}
		err := coretesting.InitCommand(envcmd.Wrap(com), test.cmdArgs)
		c.Assert(err, gc.IsNil)
		toolsDir := c.MkDir()
		updateAttrs := map[string]interface{}{
			"agent-version":      test.agentVersion,
			"agent-metadata-url": "file://" + toolsDir + "/tools",
		}

		err = s.State.UpdateEnvironConfig(updateAttrs, nil, nil)
		c.Assert(err, gc.IsNil)
		versions := make([]version.Binary, len(test.tools))
		for i, v := range test.tools {
			versions[i] = version.MustParseBinary(v)
		}
		if len(versions) > 0 {
			stor, err := filestorage.NewFileStorageWriter(toolsDir)
			c.Assert(err, gc.IsNil)
			envtesting.MustUploadFakeToolsVersions(stor, s.Environ.Config().AgentStream(), versions...)
		}

		ctx := coretesting.Context(c)
		err = com.Run(ctx)
		c.Assert(err, gc.IsNil)

		// Check agent version doesn't change
		cfg, err := s.State.EnvironConfig()
		c.Assert(err, gc.IsNil)
		agentVer, ok := cfg.AgentVersion()
		c.Assert(ok, gc.Equals, true)
		c.Assert(agentVer, gc.Equals, version.MustParse(test.agentVersion))
		output := coretesting.Stderr(ctx)
		c.Assert(output, gc.Equals, test.expectedCmdOutput)
	}
}

func (s *UpgradeJujuSuite) TestUpgradeInProgress(c *gc.C) {
	fakeAPI := NewFakeUpgradeJujuAPI(c, s.State)
	fakeAPI.setVersionErr = &params.Error{
		Message: "a message from the server about the problem",
		Code:    params.CodeUpgradeInProgress,
	}
	fakeAPI.patch(s)
	cmd := &UpgradeJujuCommand{}
	err := coretesting.InitCommand(envcmd.Wrap(cmd), []string{})
	c.Assert(err, gc.IsNil)

	err = cmd.Run(coretesting.Context(c))
	c.Assert(err, gc.ErrorMatches, "a message from the server about the problem\n"+
		"\n"+
		"Please wait for the upgrade to complete or if there was a problem with\n"+
		"the last upgrade that has been resolved, consider running the\n"+
		"upgrade-juju command with the --reset-previous-upgrade flag.",
	)
}

func (s *UpgradeJujuSuite) TestResetPreviousUpgrade(c *gc.C) {
	fakeAPI := NewFakeUpgradeJujuAPI(c, s.State)
	fakeAPI.patch(s)

	ctx := coretesting.Context(c)
	var stdin bytes.Buffer
	ctx.Stdin = &stdin

	run := func(answer string, expect bool, args ...string) {
		stdin.Reset()
		if answer != "" {
			stdin.WriteString(answer)
		}

		fakeAPI.reset()

		cmd := &UpgradeJujuCommand{}
		err := coretesting.InitCommand(envcmd.Wrap(cmd),
			append([]string{"--reset-previous-upgrade"}, args...))
		c.Assert(err, gc.IsNil)
		err = cmd.Run(ctx)
		if expect {
			c.Assert(err, gc.IsNil)
		} else {
			c.Assert(err, gc.ErrorMatches, "previous upgrade not reset and no new upgrade triggered")
		}

		c.Assert(fakeAPI.abortCurrentUpgradeCalled, gc.Equals, expect)
		expectedVersion := version.Number{}
		if expect {
			expectedVersion = fakeAPI.nextVersion.Number
		}
		c.Assert(fakeAPI.setVersionCalledWith, gc.Equals, expectedVersion)
	}

	const expectUpgrade = true
	const expectNoUpgrade = false

	// EOF on stdin - equivalent to answering no.
	run("", expectNoUpgrade)

	// -y on command line - no confirmation required
	run("", expectUpgrade, "-y")

	// --yes on command line - no confirmation required
	run("", expectUpgrade, "--yes")

	// various ways of saying "yes" to the prompt
	for _, answer := range []string{"y", "Y", "yes", "YES"} {
		run(answer, expectUpgrade)
	}

	// various ways of saying "no" to the prompt
	for _, answer := range []string{"n", "N", "no", "foo"} {
		run(answer, expectNoUpgrade)
	}
}

func NewFakeUpgradeJujuAPI(c *gc.C, st *state.State) *fakeUpgradeJujuAPI {
	nextVersion := version.Current
	nextVersion.Minor++
	return &fakeUpgradeJujuAPI{
		c:           c,
		st:          st,
		nextVersion: nextVersion,
	}
}

type fakeUpgradeJujuAPI struct {
	c                         *gc.C
	st                        *state.State
	nextVersion               version.Binary
	setVersionErr             error
	abortCurrentUpgradeCalled bool
	setVersionCalledWith      version.Number
}

func (a *fakeUpgradeJujuAPI) reset() {
	a.setVersionErr = nil
	a.abortCurrentUpgradeCalled = false
	a.setVersionCalledWith = version.Number{}
}

func (a *fakeUpgradeJujuAPI) patch(s *UpgradeJujuSuite) {
	s.PatchValue(&getUpgradeJujuAPI, func(*UpgradeJujuCommand) (upgradeJujuAPI, error) {
		return a, nil
	})
}

func (a *fakeUpgradeJujuAPI) EnvironmentGet() (map[string]interface{}, error) {
	config, err := a.st.EnvironConfig()
	if err != nil {
		return make(map[string]interface{}), err
	}
	return config.AllAttrs(), nil
}

func (a *fakeUpgradeJujuAPI) FindTools(majorVersion, minorVersion int, series, arch string) (
	result params.FindToolsResult, err error,
) {
	tools := toolstesting.MakeTools(a.c, a.c.MkDir(), "released", []string{a.nextVersion.String()})
	return params.FindToolsResult{
		List:  tools,
		Error: nil,
	}, nil
}

func (a *fakeUpgradeJujuAPI) UploadTools(r io.Reader, vers version.Binary, additionalSeries ...string) (
	*coretools.Tools, error,
) {
	panic("not implemented")
}

func (a *fakeUpgradeJujuAPI) AbortCurrentUpgrade() error {
	a.abortCurrentUpgradeCalled = true
	return nil
}

func (a *fakeUpgradeJujuAPI) SetEnvironAgentVersion(v version.Number) error {
	a.setVersionCalledWith = v
	return a.setVersionErr
}

func (a *fakeUpgradeJujuAPI) Close() error {
	return nil
}
