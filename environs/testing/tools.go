// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"bytes"
	"io/ioutil"
	"strings"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/series"
	"github.com/juju/utils/set"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	agenttools "github.com/juju/juju/agent/tools"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	"github.com/juju/juju/environs/storage"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/juju/names"
	jujunames "github.com/juju/juju/juju/names"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/binarystorage"
	coretesting "github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/juju/worker/upgrader"
)

// toolsLtsSeries records the known Ubuntu LTS series.
var toolsLtsSeries = []string{"precise", "trusty"}

// ToolsFixture is used as a fixture to stub out the default tools URL so we
// don't hit the real internet during tests.
type ToolsFixture struct {
	origDefaultURL string
	DefaultBaseURL string

	// UploadArches holds the architectures of tools to
	// upload in UploadFakeTools. If empty, it will default
	// to just arch.HostArch()
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
func (s *ToolsFixture) UploadFakeTools(c *gc.C, stor binarystorage.Storage, toolsDir, stream string) {
	arches := s.UploadArches
	if len(arches) == 0 {
		arches = []string{arch.HostArch()}
	}
	var versions []version.Binary
	for _, arch := range arches {
		v := version.Binary{
			Number: jujuversion.Current,
			Arch:   arch,
		}
		for _, series := range toolsLtsSeries {
			v.Series = series
			versions = append(versions, v)
		}
	}
	c.Logf("uploading fake tool versions: %v", versions)
	_, err := UploadFakeToolsVersions(stor, versions...)
	c.Assert(err, jc.ErrorIsNil)
}

// RemoveFakeToolsMetadata deletes the fake simplestreams tools metadata from the supplied storage.
func RemoveFakeToolsMetadata(c *gc.C, stor storage.Storage) {
	files, err := stor.List("tools/streams")
	c.Assert(err, jc.ErrorIsNil)
	for _, file := range files {
		err = stor.Remove(file)
		c.Check(err, jc.ErrorIsNil)
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
func PrimeTools(c *gc.C, stor binarystorage.Storage, dataDir string, vers version.Binary) *coretools.Tools {
	tgz, checksum := coretesting.TarGz(
		coretesting.NewTarFile(jujunames.Jujud, 0777, "jujud contents "+vers.String()))
	meta := binarystorage.Metadata{
		Version: vers.String(),
		Size:    int64(len(tgz)),
		SHA256:  checksum,
	}
	err := stor.Add(bytes.NewReader(tgz), meta)
	c.Assert(err, jc.ErrorIsNil)

	agentTools := &coretools.Tools{
		Version: version.MustParseBinary(meta.Version),
		SHA256:  meta.SHA256,
		Size:    meta.Size,
	}

	err = agenttools.UnpackTools(dataDir, agentTools, bytes.NewReader(tgz))
	c.Assert(err, jc.ErrorIsNil)
	return agentTools
}

func uploadFakeToolsVersion(stor binarystorage.Storage, vers version.Binary) (*coretools.Tools, error) {
	logger.Infof("uploading FAKE tools %s", vers)
	tgz, checksum := makeFakeTools(vers)
	size := int64(len(tgz))
	newMeta := binarystorage.Metadata{
		Version: vers.String(),
		Size:    int64(len(tgz)),
		SHA256:  checksum,
	}
	err := stor.Add(bytes.NewReader(tgz), newMeta)
	if err != nil {
		return nil, err
	}

	return &coretools.Tools{Version: vers, Size: size, SHA256: checksum}, nil
}

// InstallFakeDownloadedTools creates and unpacks fake tools of the
// given version into the data directory specified.
func InstallFakeDownloadedTools(c *gc.C, dataDir string, vers version.Binary) *coretools.Tools {
	tgz, checksum := makeFakeTools(vers)
	agentTools := &coretools.Tools{
		Version: vers,
		Size:    int64(len(tgz)),
		SHA256:  checksum,
	}
	err := agenttools.UnpackTools(dataDir, agentTools, bytes.NewReader(tgz))
	c.Assert(err, jc.ErrorIsNil)
	return agentTools
}

func makeFakeTools(vers version.Binary) ([]byte, string) {
	return coretesting.TarGz(
		coretesting.NewTarFile(names.Jujud, 0777, "jujud contents "+vers.String()))
}

// NewUploadFakeToolsVersions puts fake tools in the supplied storage for the supplied versions.
func UploadFakeToolsVersions(stor binarystorage.Storage, versions ...version.Binary) ([]*coretools.Tools, error) {
	// Leave existing tools alone.
	var agentTools coretools.List = make(coretools.List, len(versions))
	for i, ver := range versions {
		meta, err := stor.Metadata(ver.String())
		ctool := &coretools.Tools{
			Version: ver,
			Size:    meta.Size,
			SHA256:  meta.SHA256,
		}
		if errors.IsNotFound(err) {
			var tool *coretools.Tools
			tool, err = uploadFakeToolsVersion(stor, ver)
			if err != nil {
				return nil, errors.Trace(err)
			}
			ctool = tool
		}
		if err != nil {
			return nil, errors.Trace(err)
		}
		agentTools[i] = ctool
	}
	return agentTools, nil
}

func SignTestTools(stor storage.Storage) error {
	files, err := stor.List("")
	if err != nil {
		return err
	}
	for _, file := range files {
		if strings.HasSuffix(file, sstesting.UnsignedJsonSuffix) {
			// only sign .json files and data
			if err := SignFileData(stor, file); err != nil {
				return err
			}
		}
	}
	return nil
}

func SignFileData(stor storage.Storage, fileName string) error {
	r, err := stor.Get(fileName)
	if err != nil {
		return err
	}
	defer r.Close()

	fileData, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}

	signedName, signedContent, err := sstesting.SignMetadata(fileName, fileData)
	if err != nil {
		return err
	}

	err = stor.Put(signedName, strings.NewReader(string(signedContent)), int64(len(string(signedContent))))
	if err != nil {
		return err
	}
	return nil
}

// AssertUploadFakeToolsVersions puts fake tools in the supplied storage for the supplied versions.
func AssertUploadFakeToolsVersions(c *gc.C, stor binarystorage.Storage, versions ...version.Binary) []*coretools.Tools {
	agentTools, err := UploadFakeToolsVersions(stor, versions...)
	c.Assert(err, jc.ErrorIsNil)
	return agentTools
}

// MustUploadFakeToolsVersions acts as UploadFakeToolsVersions, but panics on failure.
func MustUploadFakeToolsVersions(stor binarystorage.Storage, stream string, versions ...version.Binary) []*coretools.Tools {
	var agentTools coretools.List = make(coretools.List, len(versions))
	for i, version := range versions {
		t, err := uploadFakeToolsVersion(stor, version)
		if err != nil {
			panic(err)
		}
		agentTools[i] = t
	}
	return agentTools
}

func uploadFakeTools(stor binarystorage.Storage, toolsDir, stream string) error {
	toolsSeries := set.NewStrings(toolsLtsSeries...)
	toolsSeries.Add(series.HostSeries())
	var versions []version.Binary
	for _, series := range toolsSeries.Values() {
		vers := version.Binary{
			Number: jujuversion.Current,
			Arch:   arch.HostArch(),
			Series: series,
		}
		versions = append(versions, vers)
	}
	if _, err := UploadFakeToolsVersions(stor, versions...); err != nil {
		return err
	}
	return nil
}

// UploadFakeTools puts fake tools into the supplied storage with a binary
// version matching jujuversion.Current; if jujuversion.Current's series is different
// to coretesting.FakeDefaultSeries, matching fake tools will be uploaded for that
// series.  This is useful for tests that are kinda casual about specifying
// their environment.
func UploadFakeTools(c *gc.C, stor binarystorage.Storage, toolsDir, stream string) {
	c.Assert(uploadFakeTools(stor, toolsDir, stream), gc.IsNil)
}

// MustUploadFakeTools acts as UploadFakeTools, but panics on failure.
func MustUploadFakeTools(stor binarystorage.Storage, toolsDir, stream string) {
	if err := uploadFakeTools(stor, toolsDir, stream); err != nil {
		panic(err)
	}
}

// RemoveFakeToolsFromSimpleStream deletes the fake tools from the supplied storage.
func RemoveFakeToolsFromSimpleStream(c *gc.C, stor storage.Storage, toolsDir string) {
	c.Logf("removing fake tools")
	toolsVersion := version.Binary{
		Number: jujuversion.Current,
		Arch:   arch.HostArch(),
		Series: series.HostSeries(),
	}
	name := envtools.StorageName(toolsVersion, toolsDir)
	err := stor.Remove(name)
	c.Check(err, jc.ErrorIsNil)
	defaultSeries := coretesting.FakeDefaultSeries
	if series.HostSeries() != defaultSeries {
		toolsVersion.Series = defaultSeries
		name := envtools.StorageName(toolsVersion, toolsDir)
		err := stor.Remove(name)
		c.Check(err, jc.ErrorIsNil)
	}
	RemoveFakeToolsMetadata(c, stor)
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

	V120t64 = version.MustParseBinary("1.2.0-trusty-amd64")
	V120t32 = version.MustParseBinary("1.2.0-trusty-i386")
	V120t   = []version.Binary{V120t64, V120t32}

	V120all = append(append(V120p, V120q...), V120t...)
	V1all   = append(V100Xall, append(V110all, V120all...)...)

	V220    = version.MustParse("2.2.0")
	V220p32 = version.MustParseBinary("2.2.0-precise-i386")
	V220p64 = version.MustParseBinary("2.2.0-precise-amd64")
	V220q32 = version.MustParseBinary("2.2.0-quantal-i386")
	V220q64 = version.MustParseBinary("2.2.0-quantal-amd64")
	V220all = []version.Binary{V220p64, V220p32, V220q64, V220q32}
	VAll    = append(V1all, V220all...)

	V31d0qppc64  = version.MustParseBinary("3.1-dev0-quantal-ppc64el")
	V31d01qppc64 = version.MustParseBinary("3.1-dev0.1-quantal-ppc64el")
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

var noToolsMessage = "Juju cannot bootstrap because no tools are available for your model.*"

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
	err := st.UpdateModelConfig(map[string]interface{}{"ssl-hostname-verification": SSLHostnameVerification}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
}

// AssertUploadFakeToolsVersionsToSimplestreams puts fake tools in the supplied storage for the supplied versions.
func AssertUploadFakeToolsVersionsToSimplestreams(c *gc.C, stor storage.Storage, toolsDir, stream string, versions ...version.Binary) []*coretools.Tools {
	agentTools, err := UploadFakeToolsVersionsToSimplestreams(stor, toolsDir, stream, versions...)
	c.Assert(err, jc.ErrorIsNil)
	err = envtools.MergeAndWriteMetadata(stor, toolsDir, stream, agentTools, envtools.DoNotWriteMirrors)
	c.Assert(err, jc.ErrorIsNil)
	return agentTools
}

// UploadFakeToolsVersionsToSimplestreams puts fake tools in the supplied storage for the supplied versions.
func UploadFakeToolsVersionsToSimplestreams(stor storage.Storage, toolsDir, stream string, versions ...version.Binary) ([]*coretools.Tools, error) {
	// Leave existing tools alone.
	existingTools := make(map[version.Binary]*coretools.Tools)
	existing, _ := envtools.ReadList(stor, toolsDir, 1, -1)
	for _, tools := range existing {
		existingTools[tools.Version] = tools
	}
	var agentTools = make(coretools.List, len(versions))
	for i, version := range versions {
		if tools, ok := existingTools[version]; ok {
			agentTools[i] = tools
		} else {
			t, err := uploadFakeToolsVersionToSimpleStreams(stor, toolsDir, version)
			if err != nil {
				return nil, err
			}
			agentTools[i] = t
		}
	}
	if err := envtools.MergeAndWriteMetadata(stor, toolsDir, stream, agentTools, envtools.DoNotWriteMirrors); err != nil {
		return nil, err
	}
	err := SignTestTools(stor)
	if err != nil {
		return nil, err
	}
	return agentTools, nil
}

// UploadFakeToolsToSimpleStreams uploads lts tools to a fake simple streams folder.
func UploadFakeToolsToSimpleStreams(stor storage.Storage, toolsDir, stream string) error {
	toolsSeries := set.NewStrings(toolsLtsSeries...)
	toolsSeries.Add(series.HostSeries())
	var versions []version.Binary
	for _, series := range toolsSeries.Values() {
		vers := version.Binary{
			Number: jujuversion.Current,
			Arch:   arch.HostArch(),
			Series: series,
		}
		versions = append(versions, vers)
	}
	if _, err := UploadFakeToolsVersionsToSimplestreams(stor, toolsDir, stream, versions...); err != nil {
		return err
	}
	return nil
}

func uploadFakeToolsVersionToSimpleStreams(stor storage.Storage, toolsDir string, vers version.Binary) (*coretools.Tools, error) {
	logger.Infof("uploading FAKE tools %s", vers)
	tgz, checksum := makeFakeTools(vers)
	size := int64(len(tgz))
	name := envtools.StorageName(vers, toolsDir)
	if err := stor.Put(name, bytes.NewReader(tgz), size); err != nil {
		return nil, err
	}
	url, err := stor.URL(name)
	if err != nil {
		return nil, err
	}
	return &coretools.Tools{URL: url, Version: vers, Size: size, SHA256: checksum}, nil
}

// MustUploadFakeToolsToDirectory acts as UploadFakeToolsToSimpleStreams, but panics on failure.
func MustUploadFakeToolsToDirectory(stor storage.Storage, toolsDir, stream string) {
	if err := UploadFakeToolsToSimpleStreams(stor, toolsDir, stream); err != nil {
		panic(err)
	}
}
