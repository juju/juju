// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"bytes"
	"fmt"
	"path"
	"strings"
	"time"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/environs/storage"
	envtools "launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/log"
	coretools "launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/version"
)

// ToolsFixture is used as a fixture to stub out the default tools URL so we
// don't hit the real internet during tests.
type ToolsFixture struct {
	origDefaultURL string
	DefaultBaseURL string
}

func (s *ToolsFixture) SetUpTest(c *gc.C) {
	s.origDefaultURL = envtools.DefaultBaseURL
	envtools.DefaultBaseURL = s.DefaultBaseURL
}

func (s *ToolsFixture) TearDownTest(c *gc.C) {
	envtools.DefaultBaseURL = s.origDefaultURL
}

// GenerateFakeToolsMetadata puts fake tools metadata into the supplied storage,
// containing a record for tools with a binary version matching version.Current;
// if version.Current's series is different to config.DefaultSeries, matching fake
// metadata will be included for that series.
// This is useful for tests that are kinda casual about specifying their environment.
func GenerateFakeToolsMetadata(c *gc.C, stor storage.Storage) {
	toolsVersion := version.Current
	versions := []version.Binary{toolsVersion}
	if toolsVersion.Series != config.DefaultSeries {
		toolsVersion.Series = config.DefaultSeries
		versions = append(versions, toolsVersion)
	}

	var metadata = make([]*envtools.ToolsMetadata, len(versions))
	for i, vers := range versions {
		basePath := fmt.Sprintf("releases/tools-%s.tar.gz", vers.String())
		metadata[i] = &envtools.ToolsMetadata{
			Release: vers.Series,
			Version: vers.Number.String(),
			Arch:    vers.Arch,
			Path:    basePath,
		}
	}
	index, products, err := envtools.MarshalToolsMetadataJSON(metadata, time.Now())
	c.Assert(err, gc.IsNil)
	objects := []struct {
		path string
		data []byte
	}{
		{simplestreams.DefaultIndexPath + simplestreams.UnsignedSuffix, index},
		{envtools.ProductMetadataPath, products},
	}
	for _, object := range objects {
		path := path.Join("tools", object.path)
		err = stor.Put(path, bytes.NewReader(object.data), int64(len(object.data)))
		c.Assert(err, gc.IsNil)
	}
}

// RemoveFakeMetadata deletes the fake simplestreams tools metadata from the supplied storage.
func RemoveFakeToolsMetadata(c *gc.C, stor storage.Storage) {
	files := []string{simplestreams.DefaultIndexPath + simplestreams.UnsignedSuffix, envtools.ProductMetadataPath}
	for _, file := range files {
		toolspath := path.Join("tools", file)
		err := stor.Remove(toolspath)
		c.Check(err, gc.IsNil)
	}
}

func uploadFakeToolsVersion(stor storage.Storage, vers version.Binary) (*coretools.Tools, error) {
	data := vers.String()
	name := envtools.StorageName(vers)
	log.Noticef("environs/testing: uploading FAKE tools %s", vers)
	if err := stor.Put(name, strings.NewReader(data), int64(len(data))); err != nil {
		return nil, err
	}
	url, err := stor.URL(name)
	if err != nil {
		return nil, err
	}
	return &coretools.Tools{Version: vers, URL: url}, nil
}

// UploadFakeToolsVersion puts fake tools in the supplied storage for the
// supplied version.
func UploadFakeToolsVersion(c *gc.C, storage storage.Storage, vers version.Binary) *coretools.Tools {
	t, err := uploadFakeToolsVersion(storage, vers)
	c.Assert(err, gc.IsNil)
	return t
}

// MustUploadFakeToolsVersion acts as UploadFakeToolsVersion, but panics on failure.
func MustUploadFakeToolsVersion(storage storage.Storage, vers version.Binary) *coretools.Tools {
	t, err := uploadFakeToolsVersion(storage, vers)
	if err != nil {
		panic(err)
	}
	return t
}

func uploadFakeTools(storage storage.Storage) error {
	toolsVersion := version.Current
	if _, err := uploadFakeToolsVersion(storage, toolsVersion); err != nil {
		return err
	}
	if toolsVersion.Series == config.DefaultSeries {
		return nil
	}
	toolsVersion.Series = config.DefaultSeries
	_, err := uploadFakeToolsVersion(storage, toolsVersion)
	return err
}

// UploadFakeTools puts fake tools into the supplied storage with a binary
// version matching version.Current; if version.Current's series is different
// to config.DefaultSeries, matching fake tools will be uploaded for that series.
// This is useful for tests that are kinda casual about specifying their
// environment.
func UploadFakeTools(c *gc.C, storage storage.Storage) {
	c.Assert(uploadFakeTools(storage), gc.IsNil)
}

// MustUploadFakeTools acts as UploadFakeTools, but panics on failure.
func MustUploadFakeTools(storage storage.Storage) {
	if err := uploadFakeTools(storage); err != nil {
		panic(err)
	}
}

// RemoveFakeTools deletes the fake tools from the supplied storage.
func RemoveFakeTools(c *gc.C, stor storage.Storage) {
	toolsVersion := version.Current
	name := envtools.StorageName(toolsVersion)
	err := stor.Remove(name)
	c.Check(err, gc.IsNil)
	if version.Current.Series != config.DefaultSeries {
		toolsVersion.Series = config.DefaultSeries
		name := envtools.StorageName(toolsVersion)
		err := stor.Remove(name)
		c.Check(err, gc.IsNil)
	}
}

// RemoveTools deletes all tools from the supplied storage.
func RemoveTools(c *gc.C, stor storage.Storage) {
	names, err := storage.List(stor, "tools/juju-")
	c.Assert(err, gc.IsNil)
	c.Logf("removing files: %v", names)
	for _, name := range names {
		err = stor.Remove(name)
		c.Check(err, gc.IsNil)
	}
}

// RemoveAllTools deletes all tools from the supplied environment.
func RemoveAllTools(c *gc.C, env environs.Environ) {
	c.Logf("clearing private storage")
	RemoveTools(c, env.Storage())
	c.Logf("clearing public storage")
	RemoveTools(c, env.PublicStorage().(storage.Storage))
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
	Err           error
}

var BootstrapToolsTests = []BootstrapToolsTest{
	{
		Info:          "no tools at all",
		CliVersion:    V100p64,
		DefaultSeries: "precise",
		Err:           envtools.ErrNoTools,
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
		Err:           coretools.ErrNoMatches,
	}, {
		Info:          "released cli: minor upgrades bad",
		Available:     V120all,
		CliVersion:    V100p64,
		DefaultSeries: "precise",
		Err:           coretools.ErrNoMatches,
	}, {
		Info:          "released cli: major downgrades bad",
		Available:     V100Xall,
		CliVersion:    V220p64,
		DefaultSeries: "precise",
		Err:           coretools.ErrNoMatches,
	}, {
		Info:          "released cli: minor downgrades bad",
		Available:     V100Xall,
		CliVersion:    V120p64,
		DefaultSeries: "quantal",
		Err:           coretools.ErrNoMatches,
	}, {
		Info:          "released cli: no matching series",
		Available:     VAll,
		CliVersion:    V100p64,
		DefaultSeries: "raring",
		Err:           coretools.ErrNoMatches,
	}, {
		Info:          "released cli: no matching arches",
		Available:     VAll,
		CliVersion:    V100p64,
		DefaultSeries: "precise",
		Arch:          "arm",
		Err:           coretools.ErrNoMatches,
	}, {
		Info:          "released cli: specific bad major 1",
		Available:     VAll,
		CliVersion:    V220p64,
		AgentVersion:  V120,
		DefaultSeries: "precise",
		Err:           coretools.ErrNoMatches,
	}, {
		Info:          "released cli: specific bad major 2",
		Available:     VAll,
		CliVersion:    V120p64,
		AgentVersion:  V220,
		DefaultSeries: "precise",
		Err:           coretools.ErrNoMatches,
	}, {
		Info:          "released cli: ignore dev tools 1",
		Available:     V110all,
		CliVersion:    V100p64,
		DefaultSeries: "precise",
		Err:           coretools.ErrNoMatches,
	}, {
		Info:          "released cli: ignore dev tools 2",
		Available:     V110all,
		CliVersion:    V120p64,
		DefaultSeries: "precise",
		Err:           coretools.ErrNoMatches,
	}, {
		Info:          "released cli: ignore dev tools 3",
		Available:     []version.Binary{V1001p64},
		CliVersion:    V100p64,
		DefaultSeries: "precise",
		Err:           coretools.ErrNoMatches,
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
