// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"path"
	"runtime"
	"strings"

	"github.com/golang/mock/gomock"
	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/os/v2/series"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3/arch"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/modelcmd"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facades/client/client"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/sync"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/environs/tools"
	toolstesting "github.com/juju/juju/environs/tools/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/upgrades"
	jujuversion "github.com/juju/juju/version"
)

type UpgradeBaseSuite struct {
	jujutesting.JujuConnSuite

	resources  *common.Resources
	authoriser apiservertesting.FakeAuthorizer

	coretesting.CmdBlockHelper
}

func (s *UpgradeBaseSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	client.SkipReplicaCheck(s)

	s.resources = common.NewResources()
	s.authoriser = apiservertesting.FakeAuthorizer{
		Tag: s.AdminUserTag(c),
	}

	s.CmdBlockHelper = coretesting.NewCmdBlockHelper(s.APIState)
	c.Assert(s.CmdBlockHelper, gc.NotNil)
	s.AddCleanup(func(*gc.C) { s.CmdBlockHelper.Close() })
}

type UpgradeJujuSuite struct {
	UpgradeBaseSuite
}

func (s *UpgradeJujuSuite) SetUpTest(c *gc.C) {
	if runtime.GOOS == "darwin" {
		c.Skip("Mongo failures on macOS")
	}
	s.UpgradeBaseSuite.SetUpTest(c)
	err := s.ControllerStore.UpdateModel(jujutesting.ControllerName, "admin/dummy-model", jujuclient.ModelDetails{
		ModelType: model.IAAS,
		ModelUUID: coretesting.ModelTag.Id(),
	})
	c.Assert(err, jc.ErrorIsNil)
}

var _ = gc.Suite(&UpgradeJujuSuite{})

type upgradeTest struct {
	about          string
	available      []string
	currentVersion string
	agentVersion   string

	args           []string
	expectInitErr  string
	expectErr      string
	expectVersion  string
	expectUploaded []string
	upgradeMap     map[int]version.Number
}

var upgradeJujuTests = []upgradeTest{{
	about:          "unwanted extra argument",
	currentVersion: "1.0.0-ubuntu-amd64",
	args:           []string{"foo"},
	expectInitErr:  "unrecognized args:.*",
}, {
	about:          "removed arg --dev specified",
	currentVersion: "1.0.0-ubuntu-amd64",
	args:           []string{"--dev"},
	expectInitErr:  "option provided but not defined: --dev",
}, {
	about:          "invalid --agent-version value",
	currentVersion: "1.0.0-ubuntu-amd64",
	args:           []string{"--agent-version", "invalid-version"},
	expectInitErr:  "invalid version .*",
}, {
	about:          "just major version, no minor specified",
	currentVersion: "4.2.0-ubuntu-amd64",
	args:           []string{"--agent-version", "4"},
	expectInitErr:  `invalid version "4"`,
}, {
	about:          "major version upgrade to incompatible version",
	currentVersion: "2.0.0-ubuntu-amd64",
	agentVersion:   "2.0.0",
	args:           []string{"--agent-version", "5.2.0"},
	expectErr:      `unknown version "5.2.0"`,
}, {
	about:          "version downgrade",
	available:      []string{"4.2-beta2-ubuntu-amd64"},
	currentVersion: "4.2.0-ubuntu-amd64",
	agentVersion:   "4.2.0",
	args:           []string{"--agent-version", "4.2-beta2"},
	expectErr:      "cannot change version from 4.2.0 to lower version 4.2-beta2",
}, {
	about:          "--build-agent with inappropriate version 1",
	currentVersion: "4.2.0-ubuntu-amd64",
	agentVersion:   "4.2.0",
	args:           []string{"--build-agent", "--agent-version", "3.1.0"},
	expectErr:      "cannot change version from 4.2.0 to lower version 3.1.0",
}, {
	about:          "--build-agent with inappropriate version 2",
	currentVersion: "3.2.7-ubuntu-amd64",
	args:           []string{"--build-agent", "--agent-version", "3.2.8.4"},
	expectInitErr:  "cannot specify build number when building an agent",
}, {
	about:          "latest supported stable release",
	available:      []string{"2.1.0-ubuntu-amd64", "2.1.2-ubuntu-i386", "2.1.3-ubuntu-amd64", "2.1-dev1-ubuntu-amd64"},
	currentVersion: "2.0.0-ubuntu-amd64",
	agentVersion:   "2.0.0",
	expectVersion:  "2.1.3",
}, {
	about:          "latest current release",
	available:      []string{"2.0.5-ubuntu-amd64", "2.0.1-ubuntu-i386", "2.3.3-ubuntu-amd64"},
	currentVersion: "2.0.0-ubuntu-amd64",
	agentVersion:   "2.0.0",
	expectVersion:  "2.0.5",
}, {
	about:          "latest current release with tag",
	available:      []string{"2.2.0-ubuntu-amd64", "2.2.5-ubuntu-i386", "2.3.3-ubuntu-amd64", "2.1-dev1-ubuntu-amd64"},
	currentVersion: "2.0.0-ubuntu-amd64",
	agentVersion:   "2.0.0",
	expectVersion:  "2.1-dev1",
}, {
	about:          "latest current release matching CLI, major version, no matching major agent binaries",
	available:      []string{"2.8.2-ubuntu-amd64"},
	currentVersion: "3.0.2-ubuntu-amd64",
	agentVersion:   "2.8.2",
	expectVersion:  "2.8.2",
}, {
	about:          "latest current release matching CLI, major version, no matching agent binaries",
	available:      []string{"3.3.0-ubuntu-amd64"},
	currentVersion: "3.0.2-ubuntu-amd64",
	agentVersion:   "2.8.2",
	expectErr:      "no compatible agent versions available",
}, {
	about:          "latest supported stable, when client is dev, explicit upload",
	available:      []string{"2.1-dev1-ubuntu-amd64", "2.1.0-ubuntu-amd64", "2.3-dev0-ubuntu-amd64", "3.0.1-ubuntu-amd64"},
	currentVersion: "2.1-dev0-ubuntu-amd64",
	agentVersion:   "2.0.0",
	args:           []string{"--build-agent"},
	expectVersion:  "2.1-dev0.1",
}, {
	about:          "latest current, when agent is dev",
	available:      []string{"2.1-dev1-ubuntu-amd64", "2.2.0-ubuntu-amd64", "2.3-dev0-ubuntu-amd64", "3.0.1-ubuntu-amd64"},
	currentVersion: "2.0.0-ubuntu-amd64",
	agentVersion:   "2.1-dev0",
	expectVersion:  "2.2.0",
}, {
	about:          "specified version",
	available:      []string{"2.3-dev0-ubuntu-amd64"},
	currentVersion: "2.0.0-ubuntu-amd64",
	agentVersion:   "2.0.0",
	args:           []string{"--agent-version", "2.3-dev0"},
	expectVersion:  "2.3-dev0",
}, {
	about:          "specified major version",
	available:      []string{"3.0.2-ubuntu-amd64"},
	currentVersion: "3.0.2-ubuntu-amd64",
	agentVersion:   "2.8.2",
	args:           []string{"--agent-version", "3.0.2"},
	expectVersion:  "3.0.2",
	upgradeMap:     map[int]version.Number{3: version.MustParse("2.8.2")},
}, {
	about:          "specified major version, later client",
	available:      []string{"3.0.2-ubuntu-amd64"},
	currentVersion: "3.9.2-ubuntu-amd64",
	agentVersion:   "2.8.2",
	args:           []string{"--agent-version", "3.0.2"},
	expectVersion:  "3.0.2",
	upgradeMap:     map[int]version.Number{3: version.MustParse("2.8.2")},
}, {
	about:          "specified version missing, but already set",
	currentVersion: "3.0.0-ubuntu-amd64",
	agentVersion:   "3.0.0",
	args:           []string{"--agent-version", "3.0.0"},
	expectVersion:  "3.0.0",
}, {
	about:          "specified version, no agent binaries",
	currentVersion: "3.0.0-ubuntu-amd64",
	agentVersion:   "3.0.0",
	args:           []string{"--agent-version", "3.2.0"},
	expectErr:      "no matching agent versions available",
}, {
	about:          "specified version, no matching major version",
	available:      []string{"4.2.0-ubuntu-amd64"},
	currentVersion: "3.0.0-ubuntu-amd64",
	agentVersion:   "3.0.0",
	args:           []string{"--agent-version", "3.2.0"},
	expectErr:      "no matching agent versions available",
}, {
	about:          "specified version, no matching minor version",
	available:      []string{"3.4.0-ubuntu-amd64"},
	currentVersion: "3.0.0-ubuntu-amd64",
	agentVersion:   "3.0.0",
	args:           []string{"--agent-version", "3.2.0"},
	expectErr:      "no matching agent versions available",
}, {
	about:          "specified version, no matching patch version",
	available:      []string{"3.2.5-ubuntu-amd64"},
	currentVersion: "3.0.0-ubuntu-amd64",
	agentVersion:   "3.0.0",
	args:           []string{"--agent-version", "3.2.0"},
	expectErr:      "no matching agent versions available",
}, {
	about:          "specified version, no matching build version",
	available:      []string{"3.2.0.2-ubuntu-amd64"},
	currentVersion: "3.0.0-ubuntu-amd64",
	agentVersion:   "3.0.0",
	args:           []string{"--agent-version", "3.2.0"},
	expectErr:      "no matching agent versions available",
}, {
	about:          "incompatible version (model major > client major)",
	available:      []string{"3.2.0-ubuntu-amd64"},
	currentVersion: "3.2.0-ubuntu-amd64",
	agentVersion:   "4.2.0",
	args:           []string{"--agent-version", "3.2.0"},
	expectErr:      "cannot upgrade a 4.2.0 model with a 3.2.0 client",
}, {
	about:          "incompatible version (model major < client major - 1)",
	available:      []string{"3.2.0-ubuntu-amd64"},
	currentVersion: "4.0.2-ubuntu-amd64",
	agentVersion:   "2.0.0",
	args:           []string{"--agent-version", "3.2.0"},
	expectErr:      "cannot upgrade a 2.0.0 model with a 4.0.2 client",
}, {
	about:          "minor version downgrade to incompatible version",
	available:      []string{"3.2.0-ubuntu-amd64"},
	currentVersion: "3.2.0-ubuntu-amd64",
	agentVersion:   "3.3-dev0",
	args:           []string{"--agent-version", "3.2.0"},
	expectErr:      "cannot change version from 3.3-dev0 to lower version 3.2.0",
}, {
	about:          "nothing available",
	currentVersion: "2.0.0-ubuntu-amd64",
	agentVersion:   "2.0.0",
	expectVersion:  "2.0.0",
}, {
	about:          "nothing available 2",
	currentVersion: "2.0.0-ubuntu-amd64",
	available:      []string{"3.2.0-ubuntu-amd64"},
	agentVersion:   "2.0.0",
	expectVersion:  "2.0.0",
}, {
	about:          "upload with default os type",
	currentVersion: "2.2.0-ubuntu-amd64",
	agentVersion:   "2.0.0",
	args:           []string{"--build-agent"},
	expectVersion:  "2.2.0.1",
	expectUploaded: []string{"2.2.0.1-ubuntu-amd64"},
}, {
	about:          "upload with explicit version",
	currentVersion: "2.2.0-ubuntu-amd64",
	agentVersion:   "2.0.0",
	args:           []string{"--build-agent", "--agent-version", "2.7.3"},
	expectVersion:  "2.7.3.1",
	expectUploaded: []string{"2.7.3.1-ubuntu-amd64"},
}, {
	about:          "upload dev version, currently on release version",
	currentVersion: "2.1.0-ubuntu-amd64",
	agentVersion:   "2.0.0",
	args:           []string{"--build-agent"},
	expectVersion:  "2.1.0.1",
	expectUploaded: []string{"2.1.0.1-ubuntu-amd64"},
}, {
	about:          "upload bumps version when necessary",
	available:      []string{"2.4.6-ubuntu-amd64", "2.4.8-ubuntu-amd64"},
	currentVersion: "2.4.6-ubuntu-amd64",
	agentVersion:   "2.4.0",
	args:           []string{"--build-agent"},
	expectVersion:  "2.4.6.1",
	expectUploaded: []string{"2.4.6.1-ubuntu-amd64"},
}, {
	about:          "upload re-bumps version when necessary",
	available:      []string{"2.4.6-ubuntu-amd64", "2.4.6.2-ubuntu-i386", "2.4.8-ubuntu-amd64"},
	currentVersion: "2.4.6-ubuntu-amd64",
	agentVersion:   "2.4.6.2",
	args:           []string{"--build-agent"},
	expectVersion:  "2.4.6.3",
	expectUploaded: []string{"2.4.6.3-ubuntu-amd64"},
}, {
	about:          "upload with explicit version bumps when necessary",
	currentVersion: "2.2.0-ubuntu-amd64",
	available:      []string{"2.7.3.1-ubuntu-amd64"},
	agentVersion:   "2.0.0",
	args:           []string{"--build-agent", "--agent-version", "2.7.3"},
	expectVersion:  "2.7.3.2",
	expectUploaded: []string{"2.7.3.2-ubuntu-amd64"},
}, {
	about:          "latest supported stable release increments by one minor version number",
	available:      []string{"1.21.3-ubuntu-amd64", "1.22.1-ubuntu-amd64"},
	currentVersion: "1.22.1-ubuntu-amd64",
	agentVersion:   "1.20.14",
	expectVersion:  "1.21.3",
}, {
	about:          "latest supported stable release from custom version",
	available:      []string{"1.21.3-ubuntu-amd64", "1.22.1-ubuntu-amd64"},
	currentVersion: "1.22.1-ubuntu-amd64",
	agentVersion:   "1.20.14.1",
	expectVersion:  "1.21.3",
}}

type upgradeCommandFunc func(*upgradeTest) cmd.Command

func (s *UpgradeJujuSuite) upgradeJujuCommand(
	jujuClientAPI ClientAPI,
	modelConfigAPI modelConfigAPI,
	modelManagerAPI modelManagerAPI,
	controllerAPI ControllerAPI,
) cmd.Command {
	return newUpgradeJujuCommandForTest(s.ControllerStore, jujuClientAPI, modelConfigAPI, modelManagerAPI, controllerAPI)
}

func (s *UpgradeJujuSuite) upgradeJujuCommandNoAPI(*upgradeTest) cmd.Command {
	return newUpgradeJujuCommandForTest(s.ControllerStore, nil, nil, nil, nil)
}

func (s *UpgradeJujuSuite) TestUpgradeJuju(c *gc.C) {
	s.assertUpgradeTests(c, upgradeJujuTests, s.upgradeJujuCommandNoAPI)
}

func (s *UpgradeBaseSuite) TestFormatVersions(c *gc.C) {
	toolIt := func(name string) *coretools.Tools {
		return &coretools.Tools{
			Version: version.MustParseBinary(name),
		}
	}

	for i, t := range []struct {
		desc     string
		versions []string
		expected string
	}{
		{
			desc:     "different versions",
			versions: []string{"1.21.3-ubuntu-amd64", "1.22.1-ubuntu-amd64"},
			expected: "    1.21.3\n    1.22.1",
		},
		{
			desc:     "different versions, funny ordering",
			versions: []string{"1.21.3-ubuntu-amd64", "2.6.0-ubuntu-amd64", "1.24.3-ubuntu-amd64", "1.22.1-ubuntu-amd64"},
			expected: "    1.21.3\n    1.22.1\n    1.24.3\n    2.6.0",
		},
		{
			desc:     "same versions, same release, diff arch",
			versions: []string{"1.21.3-ubuntu-amd64", "1.21.3-ubuntu-arm64"},
			expected: "    1.21.3",
		},
	} {
		c.Logf("test %d: %v", i, t.desc)
		versions := make(coretools.Versions, len(t.versions))
		for i, v := range t.versions {
			versions[i] = toolIt(v)
		}
		obtained := formatVersions(versions)
		c.Assert(obtained, gc.DeepEquals, t.expected)
	}
}

func (s *UpgradeBaseSuite) assertUpgradeTests(c *gc.C, tests []upgradeTest, upgradeJujuCommand upgradeCommandFunc) {
	for i, test := range tests {
		c.Logf("\ntest %d: %s", i, test.about)
		s.Reset(c)
		tools.DefaultBaseURL = ""

		// Set up apparent CLI version and initialize the command.
		current := version.MustParseBinary(test.currentVersion)
		s.PatchValue(&jujuversion.Current, current.Number)
		s.PatchValue(&arch.HostArch, func() string { return current.Arch })
		s.PatchValue(&coreos.HostOS, func() coreos.OSType { return coreos.Ubuntu })
		s.PatchValue(&upgrades.MinMajorUpgradeVersion, test.upgradeMap)
		com := upgradeJujuCommand(&test)
		if err := cmdtesting.InitCommand(com, test.args); err != nil {
			if test.expectInitErr != "" {
				c.Check(err, gc.ErrorMatches, test.expectInitErr)
			} else {
				c.Check(err, jc.ErrorIsNil)
			}
			continue
		}

		// Set up state and environ, and run the command.
		testDir := c.MkDir()
		updateAttrs := map[string]interface{}{
			"agent-version":      test.agentVersion,
			"agent-metadata-url": path.Join(testDir, "tools"),
		}
		err := s.Model.UpdateModelConfig(updateAttrs, nil)
		c.Assert(err, jc.ErrorIsNil)
		versions := make([]version.Binary, len(test.available))
		for i, v := range test.available {
			versions[i] = version.MustParseBinary(v)
		}
		if len(versions) > 0 {
			stor, err := filestorage.NewFileStorageWriter(testDir)
			c.Assert(err, jc.ErrorIsNil)
			envtesting.MustUploadFakeToolsVersions(stor, s.Environ.Config().AgentStream(), versions...)
		}

		err = com.Run(cmdtesting.Context(c))
		if test.expectErr != "" {
			c.Check(err, gc.ErrorMatches, test.expectErr)
			continue
		} else if !c.Check(err, jc.ErrorIsNil) {
			continue
		}

		// Check expected changes to environ/state.
		cfg, err := s.Model.ModelConfig()
		c.Check(err, jc.ErrorIsNil)
		agentVersion, ok := cfg.AgentVersion()
		c.Check(ok, jc.IsTrue)
		c.Check(agentVersion, gc.Equals, version.MustParse(test.expectVersion))

		for _, uploaded := range test.expectUploaded {
			vers := version.MustParseBinary(uploaded)
			s.checkToolsUploaded(c, vers, agentVersion)
		}
	}
}

func (s *UpgradeBaseSuite) checkToolsUploaded(c *gc.C, vers version.Binary, agentVersion version.Number) {
	storage, err := s.State.ToolsStorage()
	c.Assert(err, jc.ErrorIsNil)
	defer storage.Close()
	_, r, err := storage.Open(vers.String())
	if !c.Check(err, jc.ErrorIsNil) {
		return
	}
	data, err := ioutil.ReadAll(r)
	r.Close()
	c.Check(err, jc.ErrorIsNil)
	expectContent := version.Binary{
		Number:  agentVersion,
		Arch:    arch.HostArch(),
		Release: coreos.HostOSTypeName(),
	}
	checkToolsContent(c, data, "jujud contents "+expectContent.String())
}

func checkToolsContent(c *gc.C, data []byte, uploaded string) {
	zr, err := gzip.NewReader(bytes.NewReader(data))
	c.Check(err, jc.ErrorIsNil)
	defer zr.Close()
	tr := tar.NewReader(zr)
	found := false
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		c.Check(err, jc.ErrorIsNil)
		if strings.ContainsAny(hdr.Name, "/\\") {
			c.Fail()
		}
		if hdr.Typeflag != tar.TypeReg {
			c.Fail()
		}
		content, err := ioutil.ReadAll(tr)
		c.Check(err, jc.ErrorIsNil)
		c.Check(string(content), gc.Equals, uploaded)
		found = true
	}
	c.Check(found, jc.IsTrue)
}

// JujuConnSuite very helpfully uploads some default
// tools to the environment's storage. We don't want
// 'em there; but we do want a consistent default-series
// in the environment state.
func (s *UpgradeBaseSuite) Reset(c *gc.C) {
	s.JujuConnSuite.Reset(c)
	envtesting.RemoveTools(c, s.DefaultToolsStorage, s.Environ.Config().AgentStream())
	updateAttrs := map[string]interface{}{
		"default-series": "xenial",
		"agent-version":  "1.2.3",
	}
	err := s.Model.UpdateModelConfig(updateAttrs, nil)
	c.Assert(err, jc.ErrorIsNil)
	s.PatchValue(&sync.BuildAgentTarball, toolstesting.GetMockBuildTools(c))

	// Set API host ports so FindTools works.
	hostPorts := []network.SpaceHostPorts{
		network.NewSpaceHostPorts(1234, "0.1.2.3"),
	}
	err = s.State.SetAPIHostPorts(hostPorts)
	c.Assert(err, jc.ErrorIsNil)

	s.CmdBlockHelper = coretesting.NewCmdBlockHelper(s.APIState)
	c.Assert(s.CmdBlockHelper, gc.NotNil)
	s.AddCleanup(func(*gc.C) { s.CmdBlockHelper.Close() })
}

func (s *UpgradeJujuSuite) TestUpgradeJujuWithRealUpload(c *gc.C) {
	s.Reset(c)
	s.PatchValue(&jujuversion.Current, version.MustParse("1.99.99"))
	command := s.upgradeJujuCommandNoAPI(nil)
	_, err := cmdtesting.RunCommand(c, command, "--build-agent")
	c.Assert(err, jc.ErrorIsNil)
	vers := coretesting.CurrentVersion(c)
	vers.Build = 1
	s.checkToolsUploaded(c, vers, vers.Number)
}

func (s *UpgradeJujuSuite) TestUpgradeJujuWithImplicitUploadDevAgent(c *gc.C) {
	s.Reset(c)
	fakeAPI := &fakeUpgradeJujuAPINoState{
		name:           "dummy-model",
		uuid:           "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		controllerUUID: "deadbeef-1bad-500d-9000-4b1d0d06f00d",
		agentVersion:   "1.99.99.1",
	}
	s.PatchValue(&jujuversion.Current, version.MustParse("1.99.99"))
	command := s.upgradeJujuCommand(fakeAPI, fakeAPI, fakeAPI, nil)
	_, err := cmdtesting.RunCommand(c, command)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fakeAPI.tools, gc.Not(gc.HasLen), 0)
	c.Assert(fakeAPI.tools[0].Version.Number, gc.Equals, version.MustParse("1.99.99.2"))
}

func (s *UpgradeJujuSuite) TestUpgradeJujuWithImplicitUploadNewerClient(c *gc.C) {
	s.Reset(c)
	fakeAPI := &fakeUpgradeJujuAPINoState{
		name:           "dummy-model",
		uuid:           "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		controllerUUID: "deadbeef-1bad-500d-9000-4b1d0d06f00d",
		agentVersion:   "1.99.99",
	}
	s.PatchValue(&jujuversion.Current, version.MustParse("1.100.0"))
	command := s.upgradeJujuCommand(fakeAPI, fakeAPI, fakeAPI, nil)
	_, err := cmdtesting.RunCommand(c, command)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fakeAPI.tools, gc.Not(gc.HasLen), 0)
	c.Assert(fakeAPI.tools[0].Version.Number, gc.Equals, version.MustParse("1.100.0.1"))
	c.Assert(fakeAPI.modelAgentVersion, gc.Equals, fakeAPI.tools[0].Version.Number)
	c.Assert(fakeAPI.ignoreAgentVersions, jc.IsFalse)
}

func (s *UpgradeJujuSuite) TestUpgradeJujuWithImplicitUploadNonController(c *gc.C) {
	s.Reset(c)
	fakeAPI := &fakeUpgradeJujuAPINoState{
		name:           "dummy-model",
		uuid:           "deadbeef-0000-400d-8000-4b1d0d06f00d",
		controllerUUID: "deadbeef-1bad-500d-9000-4b1d0d06f00d",
		agentVersion:   "1.99.99.1",
	}
	s.PatchValue(&jujuversion.Current, version.MustParse("1.99.99"))
	command := s.upgradeJujuCommand(fakeAPI, fakeAPI, fakeAPI, nil)
	_, err := cmdtesting.RunCommand(c, command)
	c.Assert(err, gc.ErrorMatches, "no more recent supported versions available")
	c.Assert(fakeAPI.ignoreAgentVersions, jc.IsFalse)
}

func (s *UpgradeJujuSuite) TestBlockUpgradeJujuWithRealUpload(c *gc.C) {
	s.Reset(c)
	s.PatchValue(&jujuversion.Current, version.MustParse("1.99.99"))
	command := s.upgradeJujuCommandNoAPI(nil)
	// Block operation
	s.BlockAllChanges(c, "TestBlockUpgradeJujuWithRealUpload")
	_, err := cmdtesting.RunCommand(c, command, "--build-agent")
	coretesting.AssertOperationWasBlocked(c, err, ".*TestBlockUpgradeJujuWithRealUpload.*")
}

func (s *UpgradeJujuSuite) TestFailUploadOnNonController(c *gc.C) {
	fakeAPI := &fakeUpgradeJujuAPINoState{
		name:           "dummy-model",
		uuid:           "deadbeef-0000-400d-8000-4b1d0d06f00d",
		controllerUUID: "deadbeef-1bad-500d-9000-4b1d0d06f00d",
		agentVersion:   "1.99.99",
	}
	command := s.upgradeJujuCommand(fakeAPI, fakeAPI, fakeAPI, nil)
	_, err := cmdtesting.RunCommand(c, command, "--build-agent", "-m", "dummy-model")
	c.Assert(err, gc.ErrorMatches, "--build-agent can only be used with the controller model")
}

func (s *UpgradeJujuSuite) TestFailUploadNoControllerModelPermission(c *gc.C) {
	fakeAPI := NewFakeUpgradeJujuAPI(c, s.State)
	fakeAPI.modelConfigErr = params.Error{Code: params.CodeUnauthorized}
	command := s.upgradeJujuCommand(nil, nil, nil, fakeAPI)
	_, err := cmdtesting.RunCommand(c, command, "--build-agent")
	c.Assert(err, gc.ErrorMatches, "--build-agent can only be used with the controller model but you don't have permission to access that model")
}

func (s *UpgradeJujuSuite) TestUpgradeJujuWithIgnoreAgentVersions(c *gc.C) {
	s.Reset(c)
	fakeAPI := &fakeUpgradeJujuAPINoState{
		name:           "dummy-model",
		uuid:           "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		controllerUUID: "deadbeef-1bad-500d-9000-4b1d0d06f00d",
		agentVersion:   "1.99.99",
	}
	s.PatchValue(&jujuversion.Current, version.MustParse("1.100.0"))
	command := s.upgradeJujuCommand(fakeAPI, fakeAPI, fakeAPI, nil)
	_, err := cmdtesting.RunCommand(c, command, "--ignore-agent-versions")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fakeAPI.tools, gc.Not(gc.HasLen), 0)
	c.Assert(fakeAPI.tools[0].Version.Number, gc.Equals, version.MustParse("1.100.0.1"))
	c.Assert(fakeAPI.modelAgentVersion, gc.Equals, fakeAPI.tools[0].Version.Number)
	c.Assert(fakeAPI.ignoreAgentVersions, jc.IsTrue)
}

func (s *UpgradeJujuSuite) TestUpgradeJujuWithAgentStream(c *gc.C) {
	s.Reset(c)
	fakeAPI := &fakeUpgradeJujuAPINoState{
		name:           "dummy-model",
		uuid:           "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		controllerUUID: "deadbeef-1bad-500d-9000-4b1d0d06f00d",
		agentVersion:   "1.99.99",
	}
	s.PatchValue(&jujuversion.Current, version.MustParse("1.100.0"))
	command := s.upgradeJujuCommand(fakeAPI, fakeAPI, fakeAPI, nil)
	_, err := cmdtesting.RunCommand(c, command, "--agent-stream=proposed")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fakeAPI.tools, gc.Not(gc.HasLen), 0)
	c.Assert(fakeAPI.tools[0].Version.Number, gc.Equals, version.MustParse("1.100.0.1"))
	c.Assert(fakeAPI.modelAgentVersion, gc.Equals, fakeAPI.tools[0].Version.Number)
	c.Assert(fakeAPI.stream, gc.Equals, "proposed")
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
	s.assertUpgradeDryRun(c, "upgrade-model", s.upgradeJujuCommandNoAPI)
}

func (s *UpgradeBaseSuite) assertUpgradeDryRun(c *gc.C, command string, upgradeJujuCommand upgradeCommandFunc) {

	tests := []DryRunTest{
		{
			about:          "dry run outputs and doesn't change anything when uploading agent binaries",
			cmdArgs:        []string{"--build-agent", "--dry-run"},
			tools:          []string{"2.1.0-ubuntu-amd64", "2.1.2-ubuntu-i386", "2.1.3-ubuntu-amd64", "2.1-dev1-ubuntu-amd64", "2.2.3-ubuntu-amd64"},
			currentVersion: "2.1.3-ubuntu-amd64",
			agentVersion:   "2.0.0",
			expectedCmdOutput: fmt.Sprintf(`best version:
    2.1.3.1
upgrade to this version by running
    juju %s --build-agent
`, command),
		},
		{
			about:          "dry run outputs and doesn't change anything",
			cmdArgs:        []string{"--dry-run"},
			tools:          []string{"2.1.0-ubuntu-amd64", "2.1.2-ubuntu-i386", "2.1.3-ubuntu-amd64", "2.1-dev1-ubuntu-amd64", "2.2.3-ubuntu-amd64"},
			currentVersion: "2.0.0-ubuntu-amd64",
			agentVersion:   "2.0.0",
			expectedCmdOutput: fmt.Sprintf(`best version:
    2.1.3
upgrade to this version by running
    juju %s
`, command),
		},
		{
			about:          "dry run ignores unknown series",
			cmdArgs:        []string{"--dry-run"},
			tools:          []string{"2.1.0-ubuntu-amd64", "2.1.2-ubuntu-i386", "2.1.3-ubuntu-amd64", "1.2.3-myawesomeseries-amd64"},
			currentVersion: "2.0.0-ubuntu-amd64",
			agentVersion:   "2.0.0",
			expectedCmdOutput: fmt.Sprintf(`best version:
    2.1.3
upgrade to this version by running
    juju %s
`, command),
		},
	}

	for i, test := range tests {
		c.Logf("\ntest %d: %s", i, test.about)
		s.Reset(c)
		tools.DefaultBaseURL = ""

		s.setUpEnvAndTools(c, test.currentVersion, test.agentVersion, test.tools)

		com := upgradeJujuCommand(nil)
		err := cmdtesting.InitCommand(com, test.cmdArgs)
		c.Assert(err, jc.ErrorIsNil)

		ctx := cmdtesting.Context(c)
		err = com.Run(ctx)
		c.Assert(err, jc.ErrorIsNil)

		// Check agent version doesn't change
		cfg, err := s.Model.ModelConfig()
		c.Assert(err, jc.ErrorIsNil)
		agentVer, ok := cfg.AgentVersion()
		c.Assert(ok, jc.IsTrue)
		c.Assert(agentVer, gc.Equals, version.MustParse(test.agentVersion))
		output := cmdtesting.Stderr(ctx)
		c.Assert(output, gc.Equals, test.expectedCmdOutput)
	}
}

func (s *UpgradeBaseSuite) setUpEnvAndTools(c *gc.C, currentVersion string, agentVersion string, tools []string) {
	current := version.MustParseBinary(currentVersion)
	s.PatchValue(&jujuversion.Current, current.Number)
	s.PatchValue(&arch.HostArch, func() string { return current.Arch })
	s.PatchValue(&coreos.HostOS, func() coreos.OSType { return coreos.Ubuntu })

	tmpDir := c.MkDir()
	updateAttrs := map[string]interface{}{
		"agent-version":      agentVersion,
		"agent-metadata-url": path.Join(tmpDir, "tools"),
	}

	err := s.Model.UpdateModelConfig(updateAttrs, nil)
	c.Assert(err, jc.ErrorIsNil)
	versions := make([]version.Binary, len(tools))
	for i, v := range tools {
		versions[i], err = version.ParseBinary(v)
		if err != nil {
			c.Assert(err, jc.Satisfies, series.IsUnknownOSForSeriesError)
		}
	}
	if len(versions) > 0 {
		stor, err := filestorage.NewFileStorageWriter(tmpDir)
		c.Assert(err, jc.ErrorIsNil)
		envtesting.MustUploadFakeToolsVersions(stor, s.Environ.Config().AgentStream(), versions...)
	}
}

func (s *UpgradeJujuSuite) TestUpgradesDifferentMajor(c *gc.C) {
	tests := []struct {
		about             string
		cmdArgs           []string
		tools             []string
		currentVersion    string
		agentVersion      string
		expectedVersion   string
		expectedCmdOutput string
		expectedLogOutput string
		excludedLogOutput string
		expectedErr       string
		upgradeMap        map[int]version.Number
	}{{
		about:           "upgrade previous major to latest previous major",
		tools:           []string{"5.0.1-ubuntu-amd64", "4.9.0-ubuntu-amd64"},
		currentVersion:  "5.0.0-ubuntu-amd64",
		agentVersion:    "4.8.5",
		expectedVersion: "4.9.0",
	}, {
		about:           "upgrade previous major to latest previous major --dry-run still warns",
		tools:           []string{"5.0.1-ubuntu-amd64", "4.9.0-ubuntu-amd64"},
		currentVersion:  "5.0.1-ubuntu-amd64",
		agentVersion:    "4.8.5",
		expectedVersion: "4.9.0",
	}, {
		about:           "upgrade previous major to latest previous major with --agent-version",
		cmdArgs:         []string{"--agent-version=4.9.0"},
		tools:           []string{"5.0.2-ubuntu-amd64", "4.9.0-ubuntu-amd64", "4.8.0-ubuntu-amd64"},
		currentVersion:  "5.0.2-ubuntu-amd64",
		agentVersion:    "4.7.5",
		expectedVersion: "4.9.0",
	}, {
		about:             "can upgrade lower major version to current major version at minimum level",
		cmdArgs:           []string{"--agent-version=6.0.5"},
		tools:             []string{"6.0.5-ubuntu-amd64", "5.9.9-ubuntu-amd64"},
		currentVersion:    "6.0.0-ubuntu-amd64",
		agentVersion:      "5.9.8",
		expectedVersion:   "6.0.5",
		excludedLogOutput: `incompatible with this client (6.0.0)`,
		upgradeMap:        map[int]version.Number{6: version.MustParse("5.9.8")},
	}, {
		about:             "can upgrade lower major version to current major version above minimum level",
		cmdArgs:           []string{"--agent-version=6.0.5"},
		tools:             []string{"6.0.5-ubuntu-amd64", "5.11.0-ubuntu-amd64"},
		currentVersion:    "6.0.1-ubuntu-amd64",
		agentVersion:      "5.10.8",
		expectedVersion:   "6.0.5",
		excludedLogOutput: `incompatible with this client (6.0.1)`,
		upgradeMap:        map[int]version.Number{6: version.MustParse("5.9.8")},
	}, {
		about:           "can upgrade current to next major version",
		cmdArgs:         []string{"--agent-version=6.0.5"},
		tools:           []string{"6.0.5-ubuntu-amd64", "5.11.0-ubuntu-amd64"},
		currentVersion:  "5.10.8-ubuntu-amd64",
		agentVersion:    "5.10.8",
		expectedVersion: "6.0.5",
		upgradeMap:      map[int]version.Number{6: version.MustParse("5.9.8")},
	}, {
		about:             "upgrade fails if not at minimum version",
		cmdArgs:           []string{"--agent-version=7.0.1"},
		tools:             []string{"7.0.1-ubuntu-amd64"},
		currentVersion:    "7.0.1-ubuntu-amd64",
		agentVersion:      "6.0.0",
		expectedVersion:   "6.0.0",
		expectedCmdOutput: "upgrades to a new major version must first go through 6.7.8\n",
		expectedErr:       "unable to upgrade to requested version",
		upgradeMap:        map[int]version.Number{7: version.MustParse("6.7.8")},
	}}
	for i, test := range tests {
		c.Logf("\ntest %d: %s", i, test.about)
		s.Reset(c)
		tools.DefaultBaseURL = ""

		s.setUpEnvAndTools(c, test.currentVersion, test.agentVersion, test.tools)

		s.PatchValue(&upgrades.MinMajorUpgradeVersion, test.upgradeMap)
		command := s.upgradeJujuCommandNoAPI(nil)
		err := cmdtesting.InitCommand(command, test.cmdArgs)
		c.Assert(err, jc.ErrorIsNil)

		ctx := cmdtesting.Context(c)
		err = command.Run(ctx)
		if test.expectedErr != "" {
			c.Check(err, gc.ErrorMatches, test.expectedErr)
		} else if !c.Check(err, jc.ErrorIsNil) {
			continue
		}

		// Check agent version doesn't change
		cfg, err := s.Model.ModelConfig()
		c.Assert(err, jc.ErrorIsNil)
		agentVer, ok := cfg.AgentVersion()
		c.Assert(ok, jc.IsTrue)
		c.Check(agentVer, gc.Equals, version.MustParse(test.expectedVersion))
		output := cmdtesting.Stderr(ctx)
		if test.expectedCmdOutput != "" {
			c.Check(output, gc.Equals, test.expectedCmdOutput)
		}
		if test.expectedLogOutput != "" {
			c.Check(strings.Replace(c.GetTestLog(), "\n", " ", -1), gc.Matches, test.expectedLogOutput)
		}
		if test.excludedLogOutput != "" {
			c.Check(c.GetTestLog(), gc.Not(jc.Contains), test.excludedLogOutput)
		}
	}
}

func (s *UpgradeJujuSuite) TestUpgradeUnknownSeriesInStreams(c *gc.C) {
	fakeAPI := NewFakeUpgradeJujuAPI(c, s.State)
	fakeAPI.addTools("2.1.0-weird-amd64")

	command := s.upgradeJujuCommand(fakeAPI, fakeAPI, fakeAPI, fakeAPI)
	err := cmdtesting.InitCommand(command, []string{})
	c.Assert(err, jc.ErrorIsNil)

	err = command.Run(cmdtesting.Context(c))
	c.Assert(err, gc.IsNil)

	// ensure find tools was called
	c.Assert(fakeAPI.findToolsCalled, jc.IsTrue)
	c.Assert(fakeAPI.tools, gc.DeepEquals, []string{"2.1.0-weird-amd64", fakeAPI.nextVersion.String()})
}

func (s *UpgradeJujuSuite) TestUpgradeValidateModel(c *gc.C) {
	fakeAPI := NewFakeUpgradeJujuAPI(c, s.State)
	fakeAPI.setUpgradeErr = &params.Error{
		Message: "a message from the server about the problem",
		Code:    params.CodeAlreadyExists,
	}

	command := s.upgradeJujuCommand(fakeAPI, fakeAPI, fakeAPI, fakeAPI)
	err := cmdtesting.InitCommand(command, []string{})
	c.Assert(err, jc.ErrorIsNil)

	err = command.Run(cmdtesting.Context(c))
	c.Assert(err, gc.ErrorMatches, `a message from the server about the problem`)
}

func (s *UpgradeJujuSuite) TestUpgradeValidateModelNotImplementedNoError(c *gc.C) {
	fakeAPI := NewFakeUpgradeJujuAPI(c, s.State)

	fakeAPI.setUpgradeErr = errors.NotImplementedf("")

	command := s.upgradeJujuCommand(fakeAPI, fakeAPI, fakeAPI, fakeAPI)
	err := cmdtesting.InitCommand(command, []string{})
	c.Assert(err, jc.ErrorIsNil)

	err = command.Run(cmdtesting.Context(c))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UpgradeJujuSuite) TestUpgradeInProgress(c *gc.C) {
	fakeAPI := NewFakeUpgradeJujuAPI(c, s.State)
	fakeAPI.setVersionErr = &params.Error{
		Message: "a message from the server about the problem",
		Code:    params.CodeUpgradeInProgress,
	}

	command := s.upgradeJujuCommand(fakeAPI, fakeAPI, fakeAPI, fakeAPI)
	err := cmdtesting.InitCommand(command, []string{})
	c.Assert(err, jc.ErrorIsNil)

	err = command.Run(cmdtesting.Context(c))
	c.Assert(err, gc.ErrorMatches, "a message from the server about the problem\n"+
		"\n"+
		"Please wait for the upgrade to complete or if there was a problem with\n"+
		"the last upgrade that has been resolved, consider running the\n"+
		"upgrade-model command with the --reset-previous-upgrade option.",
	)
}

func (s *UpgradeJujuSuite) TestBlockUpgradeInProgress(c *gc.C) {
	fakeAPI := NewFakeUpgradeJujuAPI(c, s.State)
	fakeAPI.setVersionErr = apiservererrors.OperationBlockedError("the operation has been blocked")

	command := s.upgradeJujuCommand(fakeAPI, fakeAPI, fakeAPI, fakeAPI)
	err := cmdtesting.InitCommand(command, []string{})
	c.Assert(err, jc.ErrorIsNil)

	// Block operation
	s.BlockAllChanges(c, "TestBlockUpgradeInProgress")
	err = command.Run(cmdtesting.Context(c))
	s.AssertBlocked(c, err, ".*To enable changes.*")
}

func (s *UpgradeJujuSuite) TestResetPreviousUpgrade(c *gc.C) {
	fakeAPI := NewFakeUpgradeJujuAPI(c, s.State)

	ctx := cmdtesting.Context(c)
	var stdin bytes.Buffer
	ctx.Stdin = &stdin

	run := func(answer string, expect bool, args ...string) {
		stdin.Reset()
		if answer != "" {
			stdin.WriteString(answer)
		}

		fakeAPI.reset()

		command := s.upgradeJujuCommand(fakeAPI, fakeAPI, fakeAPI, fakeAPI)
		err := cmdtesting.InitCommand(command,
			append([]string{"--reset-previous-upgrade"}, args...))
		c.Assert(err, jc.ErrorIsNil)
		err = command.Run(ctx)
		if expect {
			c.Assert(err, jc.ErrorIsNil)
		} else {
			c.Assert(err, gc.ErrorMatches, "previous upgrade not reset and no new upgrade triggered")
		}

		c.Assert(fakeAPI.abortCurrentUpgradeCalled, gc.Equals, expect)
		expectedVersion := version.Number{}
		if expect {
			expectedVersion = fakeAPI.nextVersion.Number
		}
		c.Assert(fakeAPI.setVersionCalledWith, gc.Equals, expectedVersion)
		c.Assert(fakeAPI.setIgnoreCalledWith, gc.Equals, false)
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
	nextVersion := coretesting.CurrentVersion(c)
	nextVersion.Minor++
	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	return &fakeUpgradeJujuAPI{
		c:           c,
		st:          st,
		m:           m,
		nextVersion: nextVersion,
	}
}

type fakeUpgradeJujuAPI struct {
	ControllerAPI
	c                         *gc.C
	st                        *state.State
	m                         *state.Model
	nextVersion               version.Binary
	setVersionErr             error
	setUpgradeErr             error
	modelConfigErr            error
	abortCurrentUpgradeCalled bool
	setVersionCalledWith      version.Number
	setIgnoreCalledWith       bool
	setStreamCalledWith       string
	tools                     []string
	findToolsCalled           bool
}

func (a *fakeUpgradeJujuAPI) reset() {
	a.setVersionErr = nil
	a.abortCurrentUpgradeCalled = false
	a.setVersionCalledWith = version.Number{}
	a.setIgnoreCalledWith = false
	a.tools = []string{}
	a.findToolsCalled = false
}

func (a *fakeUpgradeJujuAPI) ControllerConfig() (controller.Config, error) {
	return map[string]interface{}{
		"caas-image-repo": "image-repo",
	}, nil
}

func (a *fakeUpgradeJujuAPI) ModelConfig() (map[string]interface{}, error) {
	return map[string]interface{}{
		"uuid": a.st.ControllerModelUUID(),
	}, a.modelConfigErr
}

func (a *fakeUpgradeJujuAPI) addTools(tools ...string) {
	a.tools = append(a.tools, tools...)
}

func (a *fakeUpgradeJujuAPI) ModelGet() (map[string]interface{}, error) {

	config, err := a.m.ModelConfig()
	if err != nil {
		return make(map[string]interface{}), err
	}
	return config.AllAttrs(), nil
}

func (a *fakeUpgradeJujuAPI) FindTools(majorVersion, minorVersion int, osType, arch, stream string) (
	result params.FindToolsResult, err error,
) {
	a.findToolsCalled = true
	a.tools = append(a.tools, a.nextVersion.String())
	testTools := toolstesting.MakeTools(a.c, a.c.MkDir(), "released", a.tools)
	return params.FindToolsResult{
		List:  testTools,
		Error: nil,
	}, nil
}

func (a *fakeUpgradeJujuAPI) UploadTools(r io.ReadSeeker, vers version.Binary, additionalSeries ...string) (coretools.List, error) {
	return nil, errors.New("not implemented")
}

func (a *fakeUpgradeJujuAPI) Status(patterns []string) (*params.FullStatus, error) {
	return nil, errors.New("not implemented")
}

func (a *fakeUpgradeJujuAPI) AbortCurrentUpgrade() error {
	a.abortCurrentUpgradeCalled = true
	return nil
}

func (a *fakeUpgradeJujuAPI) SetModelAgentVersion(v version.Number, stream string, ignoreAgentVersions bool) error {
	a.setVersionCalledWith = v
	a.setIgnoreCalledWith = ignoreAgentVersions
	a.setStreamCalledWith = stream
	return a.setVersionErr
}

func (a *fakeUpgradeJujuAPI) ValidateModelUpgrade(tag names.ModelTag, force bool) error {
	return a.setUpgradeErr
}

func (a *fakeUpgradeJujuAPI) Close() error {
	return nil
}

// Mock an API with no state
type fakeUpgradeJujuAPINoState struct {
	upgradeJujuAPI
	name                string
	uuid                string
	controllerUUID      string
	agentVersion        string
	tools               coretools.List
	modelAgentVersion   version.Number
	ignoreAgentVersions bool
	stream              string
}

func (a *fakeUpgradeJujuAPINoState) Close() error {
	return nil
}

func (a *fakeUpgradeJujuAPINoState) FindTools(majorVersion, minorVersion int, osType, arch, stream string) (params.FindToolsResult, error) {
	var result params.FindToolsResult
	if len(a.tools) == 0 {
		result.Error = apiservererrors.ServerError(errors.NotFoundf("tools"))
	} else {
		result.List = a.tools
	}
	return result, nil
}

func (a *fakeUpgradeJujuAPINoState) UploadTools(r io.ReadSeeker, vers version.Binary, additionalSeries ...string) (coretools.List, error) {
	a.tools = coretools.List{&coretools.Tools{Version: vers}}
	for _, s := range additionalSeries {
		v := vers
		v.Release = s
		a.tools = append(a.tools, &coretools.Tools{Version: v})
	}
	return a.tools, nil
}

func (a *fakeUpgradeJujuAPINoState) Status(patterns []string) (*params.FullStatus, error) {
	return &params.FullStatus{
		Machines: map[string]params.MachineStatus{
			"0": {Series: "focal"},
		},
	}, nil
}

func (a *fakeUpgradeJujuAPINoState) SetModelAgentVersion(version version.Number, stream string, ignoreAgentVersions bool) error {
	a.modelAgentVersion = version
	a.ignoreAgentVersions = ignoreAgentVersions
	a.stream = stream
	return nil
}

func (a *fakeUpgradeJujuAPINoState) ModelGet() (map[string]interface{}, error) {
	return dummy.SampleConfig().Merge(map[string]interface{}{
		"name":            a.name,
		"uuid":            a.uuid,
		"controller-uuid": a.controllerUUID,
		"agent-version":   a.agentVersion,
	}), nil
}

func (a *fakeUpgradeJujuAPINoState) ValidateModelUpgrade(tag names.ModelTag, force bool) error {
	return nil
}

type UpgradeCAASModelSuite struct {
	UpgradeBaseSuite
}

func (s *UpgradeCAASModelSuite) SetUpTest(c *gc.C) {
	if runtime.GOOS == "darwin" {
		c.Skip("Mongo failures on macOS")
	}
	s.UpgradeBaseSuite.SetUpTest(c)
	err := s.ControllerStore.UpdateModel(jujutesting.ControllerName, "admin/dummy-model", jujuclient.ModelDetails{
		ModelType: model.CAAS,
		ModelUUID: coretesting.ModelTag.Id(),
	})
	c.Assert(err, jc.ErrorIsNil)
}

var _ = gc.Suite(&UpgradeCAASModelSuite{})

var upgradeCAASModelTests = []upgradeTest{{
	about:          "unwanted extra argument",
	currentVersion: "1.0.0-ubuntu-amd64",
	agentVersion:   "1.0.0",
	args:           []string{"foo"},
	expectInitErr:  "unrecognized args:.*",
}, {
	about:          "invalid --agent-version value",
	currentVersion: "1.0.0-ubuntu-amd64",
	agentVersion:   "1.0.0",
	args:           []string{"--agent-version", "invalid-version"},
	expectInitErr:  "invalid version .*",
}, {
	about:          "latest supported stable release",
	available:      []string{"2.1.0-ubuntu-amd64", "2.1.2-ubuntu-amd64", "2.1.3-ubuntu-amd64", "2.1-dev1-ubuntu-amd64"},
	currentVersion: "2.0.0-ubuntu-amd64",
	agentVersion:   "2.0.0",
	expectVersion:  "2.1.3",
}, {
	about:          "latest supported stable release increments by one minor version number",
	available:      []string{"1.21.3-ubuntu-amd64", "1.22.1-ubuntu-amd64"},
	currentVersion: "1.22.1-ubuntu-amd64",
	agentVersion:   "1.20.14",
	expectVersion:  "1.21.3",
}, {
	about:          "latest supported stable release from custom version",
	available:      []string{"1.21.3-ubuntu-amd64", "1.22.1-ubuntu-amd64"},
	currentVersion: "1.22.1-ubuntu-amd64",
	agentVersion:   "1.20.14.1",
	expectVersion:  "1.21.3",
}}

func (s *UpgradeCAASModelSuite) upgradeModelCommand(*upgradeTest) cmd.Command {
	return newUpgradeJujuCommandForTest(s.ControllerStore, nil, nil, nil, nil)
}

func (s *UpgradeCAASModelSuite) TestUpgrade(c *gc.C) {
	s.UpgradeBaseSuite.assertUpgradeTests(c, upgradeCAASModelTests, s.upgradeModelCommand)
}

type upgradePrecheckSuite struct {
	testing.CleanupSuite

	upgradeContext *upgradeContext
	ops            []environs.PrecheckJujuUpgradeOperation

	upgradeEnv *MockUpgradePrecheckEnviron
	env        *MockEnviron
	broker     caas.Broker
}

var _ = gc.Suite(&upgradePrecheckSuite{})

func (s *upgradePrecheckSuite) SetUpTest(c *gc.C) {
	s.CleanupSuite.SetUpTest(c)
	s.upgradeContext = &upgradeContext{chosen: version.MustParse("2.7.6")}
}

func (s *upgradePrecheckSuite) TestPrecheckCAAS(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.patchCAASBroker(s.broker)

	err := doPrecheckEnviron(model.CAAS, environConfigGetter{}, s.upgradeContext, version.MustParse("2.7.4"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *upgradePrecheckSuite) TestPrecheckEnvironNoUpgradePrecheckEnviron(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.patchEnviron(s.env)

	err := doPrecheckEnviron(model.IAAS, environConfigGetter{}, s.upgradeContext, version.MustParse("2.7.4"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *upgradePrecheckSuite) TestPrecheckEnvironPrepareFail(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectPreparePrechecker(errors.NotSupportedf("testing"))
	s.patchEnviron(s.upgradeEnv)

	err := doPrecheckEnviron(model.IAAS, environConfigGetter{}, s.upgradeContext, version.MustParse("2.7.4"))
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
}

func (s *upgradePrecheckSuite) TestPrecheckEnvironRunNoSteps(c *gc.C) {
	// upgrade from 2.7.4 to 2.7.6,
	// step at 2.8.0 not run or test would blow up.
	s.ops = []environs.PrecheckJujuUpgradeOperation{
		{TargetVersion: version.MustParse("2.8.0")},
	}

	defer s.setupMocks(c).Finish()
	s.expectPreparePrechecker(nil)
	s.expectPrecheckUpgradeOperations()
	s.patchEnviron(s.upgradeEnv)

	err := doPrecheckEnviron(model.IAAS, environConfigGetter{}, s.upgradeContext, version.MustParse("2.7.4"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *upgradePrecheckSuite) TestPrecheckEnvironRunOneOfTwoSteps(c *gc.C) {
	// upgrade from 2.7.4 to 2.7.6, run step at 2.7.6
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.expectPreparePrechecker(nil)

	stepPass := s.setupUpgradeStep(ctrl, "test 2.7.6 step", nil)
	stepNotRun := NewMockPrecheckJujuUpgradeStep(ctrl)
	s.ops = []environs.PrecheckJujuUpgradeOperation{
		{TargetVersion: version.MustParse("2.8.0"), Steps: []environs.PrecheckJujuUpgradeStep{stepNotRun}},
		{TargetVersion: version.MustParse("2.7.6"), Steps: []environs.PrecheckJujuUpgradeStep{stepPass}},
	}

	s.expectPrecheckUpgradeOperations()
	s.patchEnviron(s.upgradeEnv)

	err := doPrecheckEnviron(model.IAAS, environConfigGetter{}, s.upgradeContext, version.MustParse("2.7.4"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *upgradePrecheckSuite) TestPrecheckEnvironRunStepRC(c *gc.C) {
	// upgrade from 2.7.4 to 2.8-rc1, run step at 2.8.0
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.expectPreparePrechecker(nil)
	s.upgradeContext.chosen = version.MustParse("2.8-rc1")

	stepPass := s.setupUpgradeStep(ctrl, "test 2.8.0 step", nil)
	s.ops = []environs.PrecheckJujuUpgradeOperation{
		{TargetVersion: version.MustParse("2.8.0"), Steps: []environs.PrecheckJujuUpgradeStep{stepPass}},
	}

	s.expectPrecheckUpgradeOperations()
	s.patchEnviron(s.upgradeEnv)

	err := doPrecheckEnviron(model.IAAS, environConfigGetter{}, s.upgradeContext, version.MustParse("2.7.4"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *upgradePrecheckSuite) TestPrecheckEnvironRunStepFail(c *gc.C) {
	// upgrade from 2.7.4 to 2.7.6, fail step at 2.7.6
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.expectPreparePrechecker(nil)

	stepPass := s.setupUpgradeStep(ctrl, "test 2.7.6 step", errors.NotSupportedf("test fail"))
	s.ops = []environs.PrecheckJujuUpgradeOperation{
		{TargetVersion: version.MustParse("2.7.6"), Steps: []environs.PrecheckJujuUpgradeStep{stepPass}},
	}

	s.expectPrecheckUpgradeOperations()
	s.patchEnviron(s.upgradeEnv)

	err := doPrecheckEnviron(model.IAAS, environConfigGetter{}, s.upgradeContext, version.MustParse("2.7.4"))
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
}

func (s *upgradePrecheckSuite) patchEnviron(env environs.Environ) {
	s.PatchValue(&getEnviron,
		func(_ environs.EnvironConfigGetter, _ environs.NewEnvironFunc) (environs.Environ, error) {
			return env, nil
		},
	)
}

func (s *upgradePrecheckSuite) patchCAASBroker(b caas.Broker) {
	s.PatchValue(&getCAASBroker,
		func(_ environs.EnvironConfigGetter) (caas.Broker, error) {
			return b, nil
		},
	)
}

func (s *upgradePrecheckSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.upgradeEnv = NewMockUpgradePrecheckEnviron(ctrl)
	s.env = NewMockEnviron(ctrl)
	return ctrl
}

func (s *upgradePrecheckSuite) setupUpgradeStep(ctrl *gomock.Controller, msg string, err error) *MockPrecheckJujuUpgradeStep {
	step := NewMockPrecheckJujuUpgradeStep(ctrl)
	exp := step.EXPECT()
	exp.Description().Return(msg)
	exp.Run().Return(err)
	return step
}

func (s *upgradePrecheckSuite) expectPreparePrechecker(err error) {
	s.upgradeEnv.EXPECT().PreparePrechecker().Return(err)
}

func (s *upgradePrecheckSuite) expectPrecheckUpgradeOperations() {
	s.upgradeEnv.EXPECT().PrecheckUpgradeOperations().Return(s.ops)
}

func newUpgradeJujuCommandForTest(
	store jujuclient.ClientStore,
	jujuClientAPI ClientAPI,
	modelConfigAPI modelConfigAPI,
	modelManagerAPI modelManagerAPI,
	controllerAPI ControllerAPI,
	options ...modelcmd.WrapOption,
) cmd.Command {
	command := &upgradeJujuCommand{
		baseUpgradeCommand: baseUpgradeCommand{
			modelConfigAPI:  modelConfigAPI,
			modelManagerAPI: modelManagerAPI,
			controllerAPI:   controllerAPI,
		},
		jujuClientAPI: jujuClientAPI,
	}
	command.SetClientStore(store)
	return modelcmd.Wrap(command, options...)
}
