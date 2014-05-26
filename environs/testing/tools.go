// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"bytes"
	"os"
	"path"
	"path/filepath"

	gc "launchpad.net/gocheck"

	agenttools "launchpad.net/juju-core/agent/tools"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/bootstrap"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/environs/storage"
	envtools "launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
	coretools "launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/version"
	"launchpad.net/juju-core/worker/upgrader"
)

// ToolsFixture is used as a fixture to stub out the default tools URL so we
// don't hit the real internet during tests.
type ToolsFixture struct {
	origDefaultURL string
	DefaultBaseURL string

	// UploadArches holds the architectures of tools to
	// upload in UploadFakeTools. If empty, it will default
	// to just version.Current.Arch.
	UploadArches []string
}

func (s *ToolsFixture) SetUpTest(c *gc.C) {
	s.origDefaultURL = envtools.DefaultBaseURL
	envtools.DefaultBaseURL = s.DefaultBaseURL
}

func (s *ToolsFixture) TearDownTest(c *gc.C) {
	envtools.DefaultBaseURL = s.origDefaultURL
}

// UploadFakeTools uploads fake tools of the architectures in
// s.UploadArches for each LTS release to the specified storage.
func (s *ToolsFixture) UploadFakeTools(c *gc.C, stor storage.Storage) {
	arches := s.UploadArches
	if len(arches) == 0 {
		arches = []string{version.Current.Arch}
	}
	var versions []version.Binary
	for _, arch := range arches {
		v := version.Current
		v.Arch = arch
		for _, series := range bootstrap.ToolsLtsSeries {
			v.Series = series
			versions = append(versions, v)
		}
	}
	_, err := UploadFakeToolsVersions(stor, versions...)
	c.Assert(err, gc.IsNil)
}

// RemoveFakeToolsMetadata deletes the fake simplestreams tools metadata from the supplied storage.
func RemoveFakeToolsMetadata(c *gc.C, stor storage.Storage) {
	files := []string{simplestreams.UnsignedIndex, envtools.ProductMetadataPath}
	for _, file := range files {
		toolspath := path.Join("tools", file)
		err := stor.Remove(toolspath)
		c.Check(err, gc.IsNil)
	}
}

// CheckTools ensures the obtained and expected tools are equal, allowing for the fact that
// the obtained tools may not have size and checksum set.
func CheckTools(c *gc.C, obtained, expected *coretools.Tools) {
	c.Assert(obtained.Version, gc.Equals, expected.Version)
	// TODO(dimitern) 2013-10-02 bug #1234217
	// Are these used at at all? If not we should drop them.
	if obtained.URL != "" {
		c.Assert(obtained.URL, gc.Equals, expected.URL)
	}
	if obtained.Size > 0 {
		c.Assert(obtained.Size, gc.Equals, expected.Size)
		c.Assert(obtained.SHA256, gc.Equals, expected.SHA256)
	}
}

// CheckUpgraderReadyError ensures the obtained and expected errors are equal.
func CheckUpgraderReadyError(c *gc.C, obtained error, expected *upgrader.UpgradeReadyError) {
	c.Assert(obtained, gc.FitsTypeOf, &upgrader.UpgradeReadyError{})
	err := obtained.(*upgrader.UpgradeReadyError)
	c.Assert(err.AgentName, gc.Equals, expected.AgentName)
	c.Assert(err.DataDir, gc.Equals, expected.DataDir)
	c.Assert(err.OldTools, gc.Equals, expected.OldTools)
	c.Assert(err.NewTools, gc.Equals, expected.NewTools)
}

// PrimeTools sets up the current version of the tools to vers and
// makes sure that they're available in the dataDir.
func PrimeTools(c *gc.C, stor storage.Storage, dataDir string, vers version.Binary) *coretools.Tools {
	err := os.RemoveAll(filepath.Join(dataDir, "tools"))
	c.Assert(err, gc.IsNil)
	agentTools, err := uploadFakeToolsVersion(stor, vers)
	c.Assert(err, gc.IsNil)
	resp, err := utils.GetValidatingHTTPClient().Get(agentTools.URL)
	c.Assert(err, gc.IsNil)
	defer resp.Body.Close()
	err = agenttools.UnpackTools(dataDir, agentTools, resp.Body)
	c.Assert(err, gc.IsNil)
	return agentTools
}

func uploadFakeToolsVersion(stor storage.Storage, vers version.Binary) (*coretools.Tools, error) {
	logger.Infof("uploading FAKE tools %s", vers)
	tgz, checksum := coretesting.TarGz(
		coretesting.NewTarFile("jujud", 0777, "jujud contents "+vers.String()))
	size := int64(len(tgz))
	name := envtools.StorageName(vers)
	if err := stor.Put(name, bytes.NewReader(tgz), size); err != nil {
		return nil, err
	}
	url, err := stor.URL(name)
	if err != nil {
		return nil, err
	}
	return &coretools.Tools{URL: url, Version: vers, Size: size, SHA256: checksum}, nil
}

// UploadFakeToolsVersions puts fake tools in the supplied storage for the supplied versions.
func UploadFakeToolsVersions(stor storage.Storage, versions ...version.Binary) ([]*coretools.Tools, error) {
	// Leave existing tools alone.
	existingTools := make(map[version.Binary]*coretools.Tools)
	existing, _ := envtools.ReadList(stor, 1, -1)
	for _, tools := range existing {
		existingTools[tools.Version] = tools
	}
	var agentTools coretools.List = make(coretools.List, len(versions))
	for i, version := range versions {
		if tools, ok := existingTools[version]; ok {
			agentTools[i] = tools
		} else {
			t, err := uploadFakeToolsVersion(stor, version)
			if err != nil {
				return nil, err
			}
			agentTools[i] = t
		}
	}
	if err := envtools.MergeAndWriteMetadata(stor, agentTools, envtools.DoNotWriteMirrors); err != nil {
		return nil, err
	}
	return agentTools, nil
}

// AssertUploadFakeToolsVersions puts fake tools in the supplied storage for the supplied versions.
func AssertUploadFakeToolsVersions(c *gc.C, stor storage.Storage, versions ...version.Binary) []*coretools.Tools {
	agentTools, err := UploadFakeToolsVersions(stor, versions...)
	c.Assert(err, gc.IsNil)
	err = envtools.MergeAndWriteMetadata(stor, agentTools, envtools.DoNotWriteMirrors)
	c.Assert(err, gc.IsNil)
	return agentTools
}

// MustUploadFakeToolsVersions acts as UploadFakeToolsVersions, but panics on failure.
func MustUploadFakeToolsVersions(stor storage.Storage, versions ...version.Binary) []*coretools.Tools {
	var agentTools coretools.List = make(coretools.List, len(versions))
	for i, version := range versions {
		t, err := uploadFakeToolsVersion(stor, version)
		if err != nil {
			panic(err)
		}
		agentTools[i] = t
	}
	err := envtools.MergeAndWriteMetadata(stor, agentTools, envtools.DoNotWriteMirrors)
	if err != nil {
		panic(err)
	}
	return agentTools
}

func uploadFakeTools(stor storage.Storage) error {
	var versions []version.Binary
	for _, series := range bootstrap.ToolsLtsSeries {
		vers := version.Current
		vers.Series = series
		versions = append(versions, vers)
	}
	if _, err := UploadFakeToolsVersions(stor, versions...); err != nil {
		return err
	}
	return nil
}

// UploadFakeTools puts fake tools into the supplied storage with a binary
// version matching version.Current; if version.Current's series is different
// to coretesting.FakeDefaultSeries, matching fake tools will be uploaded for that
// series.  This is useful for tests that are kinda casual about specifying
// their environment.
func UploadFakeTools(c *gc.C, stor storage.Storage) {
	c.Assert(uploadFakeTools(stor), gc.IsNil)
}

// MustUploadFakeTools acts as UploadFakeTools, but panics on failure.
func MustUploadFakeTools(stor storage.Storage) {
	if err := uploadFakeTools(stor); err != nil {
		panic(err)
	}
}

// RemoveFakeTools deletes the fake tools from the supplied storage.
func RemoveFakeTools(c *gc.C, stor storage.Storage) {
	c.Logf("removing fake tools")
	toolsVersion := version.Current
	name := envtools.StorageName(toolsVersion)
	err := stor.Remove(name)
	c.Check(err, gc.IsNil)
	defaultSeries := coretesting.FakeDefaultSeries
	if version.Current.Series != defaultSeries {
		toolsVersion.Series = defaultSeries
		name := envtools.StorageName(toolsVersion)
		err := stor.Remove(name)
		c.Check(err, gc.IsNil)
	}
	RemoveFakeToolsMetadata(c, stor)
}

// RemoveTools deletes all tools from the supplied storage.
func RemoveTools(c *gc.C, stor storage.Storage) {
	names, err := storage.List(stor, "tools/releases/juju-")
	c.Assert(err, gc.IsNil)
	c.Logf("removing files: %v", names)
	for _, name := range names {
		err = stor.Remove(name)
		c.Check(err, gc.IsNil)
	}
	RemoveFakeToolsMetadata(c, stor)
}

// RemoveAllTools deletes all tools from the supplied environment.
func RemoveAllTools(c *gc.C, env environs.Environ) {
	c.Logf("clearing private storage")
	RemoveTools(c, env.Storage())
}

var (
	V100    = version.MustParse("1.0.0")
	V100p64 = version.MustParseBinary("1.0.0-precise-amd64")
	V100p32 = version.MustParseBinary("1.0.0-precise-i386")
	V100p   = []version.Binary{V100p64, V100p32}

	V100q64 = version.MustParseBinary("1.0.0-quantal-amd64")
	V100q32 = version.MustParseBinary("1.0.0-quantal-i386")
	V100q   = []version.Binary{V100q64, V100q32}
	V100all = append(V100p, V100q...)

	V1001    = version.MustParse("1.0.0.1")
	V1001p64 = version.MustParseBinary("1.0.0.1-precise-amd64")
	V100Xall = append(V100all, V1001p64)

	V110    = version.MustParse("1.1.0")
	V110p64 = version.MustParseBinary("1.1.0-precise-amd64")
	V110p32 = version.MustParseBinary("1.1.0-precise-i386")
	V110p   = []version.Binary{V110p64, V110p32}

	V110q64 = version.MustParseBinary("1.1.0-quantal-amd64")
	V110q32 = version.MustParseBinary("1.1.0-quantal-i386")
	V110q   = []version.Binary{V110q64, V110q32}
	V110all = append(V110p, V110q...)

	V1101p64 = version.MustParseBinary("1.1.0.1-precise-amd64")
	V110Xall = append(V110all, V1101p64)

	V120    = version.MustParse("1.2.0")
	V120p64 = version.MustParseBinary("1.2.0-precise-amd64")
	V120p32 = version.MustParseBinary("1.2.0-precise-i386")
	V120p   = []version.Binary{V120p64, V120p32}

	V120q64 = version.MustParseBinary("1.2.0-quantal-amd64")
	V120q32 = version.MustParseBinary("1.2.0-quantal-i386")
	V120q   = []version.Binary{V120q64, V120q32}
	V120all = append(V120p, V120q...)
	V1all   = append(V100Xall, append(V110all, V120all...)...)

	V220    = version.MustParse("2.2.0")
	V220p32 = version.MustParseBinary("2.2.0-precise-i386")
	V220p64 = version.MustParseBinary("2.2.0-precise-amd64")
	V220q32 = version.MustParseBinary("2.2.0-quantal-i386")
	V220q64 = version.MustParseBinary("2.2.0-quantal-amd64")
	V220all = []version.Binary{V220p64, V220p32, V220q64, V220q32}
	VAll    = append(V1all, V220all...)

	V310qppc64  = version.MustParseBinary("3.1.0-quantal-ppc64")
	V3101qppc64 = version.MustParseBinary("3.1.0.1-quantal-ppc64")
)

type BootstrapToolsTest struct {
	Info          string
	Available     []version.Binary
	CliVersion    version.Binary
	DefaultSeries string
	AgentVersion  version.Number
	Development   bool
	Arch          string
	Expect        []version.Binary
	Err           string
}

var noToolsMessage = "Juju cannot bootstrap because no tools are available for your environment.*"

var BootstrapToolsTests = []BootstrapToolsTest{
	{
		Info:          "no tools at all",
		CliVersion:    V100p64,
		DefaultSeries: "precise",
		Err:           noToolsMessage,
	}, {
		Info:          "released cli: use newest compatible release version",
		Available:     VAll,
		CliVersion:    V100p64,
		DefaultSeries: "precise",
		Expect:        V100p,
	}, {
		Info:          "released cli: cli Arch ignored",
		Available:     VAll,
		CliVersion:    V100p32,
		DefaultSeries: "precise",
		Expect:        V100p,
	}, {
		Info:          "released cli: cli series ignored",
		Available:     VAll,
		CliVersion:    V100q64,
		DefaultSeries: "precise",
		Expect:        V100p,
	}, {
		Info:          "released cli: series taken from default-series",
		Available:     V120all,
		CliVersion:    V120p64,
		DefaultSeries: "quantal",
		Expect:        V120q,
	}, {
		Info:          "released cli: ignore close dev match",
		Available:     V100Xall,
		CliVersion:    V100p64,
		DefaultSeries: "precise",
		Expect:        V100p,
	}, {
		Info:          "released cli: filter by arch constraints",
		Available:     V120all,
		CliVersion:    V120p64,
		DefaultSeries: "precise",
		Arch:          "i386",
		Expect:        []version.Binary{V120p32},
	}, {
		Info:          "released cli: specific released version",
		Available:     VAll,
		CliVersion:    V100p64,
		AgentVersion:  V100,
		DefaultSeries: "precise",
		Expect:        V100p,
	}, {
		Info:          "released cli: specific dev version",
		Available:     VAll,
		CliVersion:    V110p64,
		AgentVersion:  V110,
		DefaultSeries: "precise",
		Expect:        V110p,
	}, {
		Info:          "released cli: major upgrades bad",
		Available:     V220all,
		CliVersion:    V100p64,
		DefaultSeries: "precise",
		Err:           noToolsMessage,
	}, {
		Info:          "released cli: minor upgrades bad",
		Available:     V120all,
		CliVersion:    V100p64,
		DefaultSeries: "precise",
		Err:           noToolsMessage,
	}, {
		Info:          "released cli: major downgrades bad",
		Available:     V100Xall,
		CliVersion:    V220p64,
		DefaultSeries: "precise",
		Err:           noToolsMessage,
	}, {
		Info:          "released cli: minor downgrades bad",
		Available:     V100Xall,
		CliVersion:    V120p64,
		DefaultSeries: "quantal",
		Err:           noToolsMessage,
	}, {
		Info:          "released cli: no matching series",
		Available:     VAll,
		CliVersion:    V100p64,
		DefaultSeries: "raring",
		Err:           noToolsMessage,
	}, {
		Info:          "released cli: no matching arches",
		Available:     VAll,
		CliVersion:    V100p64,
		DefaultSeries: "precise",
		Arch:          "armhf",
		Err:           noToolsMessage,
	}, {
		Info:          "released cli: specific bad major 1",
		Available:     VAll,
		CliVersion:    V220p64,
		AgentVersion:  V120,
		DefaultSeries: "precise",
		Err:           noToolsMessage,
	}, {
		Info:          "released cli: specific bad major 2",
		Available:     VAll,
		CliVersion:    V120p64,
		AgentVersion:  V220,
		DefaultSeries: "precise",
		Err:           noToolsMessage,
	}, {
		Info:          "released cli: ignore dev tools 1",
		Available:     V110all,
		CliVersion:    V100p64,
		DefaultSeries: "precise",
		Err:           noToolsMessage,
	}, {
		Info:          "released cli: ignore dev tools 2",
		Available:     V110all,
		CliVersion:    V120p64,
		DefaultSeries: "precise",
		Err:           noToolsMessage,
	}, {
		Info:          "released cli: ignore dev tools 3",
		Available:     []version.Binary{V1001p64},
		CliVersion:    V100p64,
		DefaultSeries: "precise",
		Err:           noToolsMessage,
	}, {
		Info:          "released cli with dev setting respects agent-version",
		Available:     VAll,
		CliVersion:    V100q32,
		AgentVersion:  V1001,
		DefaultSeries: "precise",
		Development:   true,
		Expect:        []version.Binary{V1001p64},
	}, {
		Info:          "dev cli respects agent-version",
		Available:     VAll,
		CliVersion:    V100q32,
		AgentVersion:  V1001,
		DefaultSeries: "precise",
		Expect:        []version.Binary{V1001p64},
	}, {
		Info:          "released cli with dev setting respects agent-version",
		Available:     V1all,
		CliVersion:    V100q32,
		AgentVersion:  V1001,
		DefaultSeries: "precise",
		Development:   true,
		Expect:        []version.Binary{V1001p64},
	}, {
		Info:          "dev cli respects agent-version",
		Available:     V1all,
		CliVersion:    V100q32,
		AgentVersion:  V1001,
		DefaultSeries: "precise",
		Expect:        []version.Binary{V1001p64},
	}}

func SetSSLHostnameVerification(c *gc.C, st *state.State, SSLHostnameVerification bool) {
	err := st.UpdateEnvironConfig(map[string]interface{}{"ssl-hostname-verification": SSLHostnameVerification}, nil, nil)
	c.Assert(err, gc.IsNil)
}
