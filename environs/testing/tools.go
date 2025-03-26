// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/collections/set"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	agenterrors "github.com/juju/juju/agent/errors"
	agenttools "github.com/juju/juju/agent/tools"
	"github.com/juju/juju/core/arch"
	coreos "github.com/juju/juju/core/os"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/simplestreams"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	"github.com/juju/juju/environs/storage"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/internal/http"
	coretesting "github.com/juju/juju/internal/testing"
	coretools "github.com/juju/juju/internal/tools"
	"github.com/juju/juju/internal/version"
	"github.com/juju/juju/juju/names"
)

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

// UploadFakeToolsToDirectory uploads fake tools of the architectures in
// s.UploadArches for each LTS release to the specified directory.
func (s *ToolsFixture) UploadFakeToolsToDirectory(c *gc.C, dir, toolsDir, stream string) {
	stor, err := filestorage.NewFileStorageWriter(dir)
	c.Assert(err, jc.ErrorIsNil)
	s.UploadFakeTools(c, stor, toolsDir, stream)
}

// UploadFakeTools uploads fake tools of the architectures in
// s.UploadArches for each LTS release to the specified storage.
func (s *ToolsFixture) UploadFakeTools(c *gc.C, stor storage.Storage, toolsDir, stream string) {
	UploadFakeTools(c, stor, toolsDir, stream, s.UploadArches...)
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
func CheckUpgraderReadyError(c *gc.C, obtained error, expected *agenterrors.UpgradeReadyError) {
	c.Assert(obtained, gc.FitsTypeOf, &agenterrors.UpgradeReadyError{})
	err := obtained.(*agenterrors.UpgradeReadyError)
	c.Assert(err.AgentName, gc.Equals, expected.AgentName)
	c.Assert(err.DataDir, gc.Equals, expected.DataDir)
	c.Assert(err.OldTools, gc.Equals, expected.OldTools)
	c.Assert(err.NewTools, gc.Equals, expected.NewTools)
}

// PrimeTools sets up the current version of the tools to vers and
// makes sure that they're available in the dataDir.
func PrimeTools(c *gc.C, stor storage.Storage, dataDir, toolsDir string, vers version.Binary) *coretools.Tools {
	err := os.RemoveAll(filepath.Join(dataDir, "tools"))
	c.Assert(err, jc.ErrorIsNil)
	agentTools, err := uploadFakeToolsVersion(stor, toolsDir, vers)
	c.Assert(err, jc.ErrorIsNil)
	client := http.NewClient()
	resp, err := client.Get(context.Background(), agentTools.URL)
	c.Assert(err, jc.ErrorIsNil)
	defer resp.Body.Close()
	err = agenttools.UnpackTools(dataDir, agentTools, resp.Body)
	c.Assert(err, jc.ErrorIsNil)
	return agentTools
}

func uploadFakeToolsVersion(stor storage.Storage, toolsDir string, vers version.Binary) (*coretools.Tools, error) {
	logger.Infof(context.Background(), "uploading FAKE tools %s", vers)
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

// UploadFakeToolsVersions puts fake tools in the supplied storage for the supplied versions.
func UploadFakeToolsVersions(store storage.Storage, toolsDir, stream string, versions ...version.Binary) ([]*coretools.Tools, error) {
	// Leave existing tools alone.
	existingTools := make(map[version.Binary]*coretools.Tools)
	existing, _ := envtools.ReadList(context.Background(), store, toolsDir, 1, -1)
	for _, tools := range existing {
		existingTools[tools.Version] = tools
	}
	var agentTools = make(coretools.List, len(versions))
	for i, version := range versions {
		if tools, ok := existingTools[version]; ok {
			agentTools[i] = tools
		} else {
			t, err := uploadFakeToolsVersion(store, toolsDir, version)
			if err != nil {
				return nil, err
			}
			agentTools[i] = t
		}
	}
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	if err := envtools.MergeAndWriteMetadata(context.Background(), ss, store, toolsDir, stream, agentTools, envtools.DoNotWriteMirrors); err != nil {
		return nil, err
	}
	err := SignTestTools(store)
	if err != nil {
		return nil, err
	}
	return agentTools, nil
}

func SignTestTools(store storage.Storage) error {
	files, err := store.List("")
	if err != nil {
		return err
	}
	for _, file := range files {
		if strings.HasSuffix(file, sstesting.UnsignedJsonSuffix) {
			// only sign .json files and data
			if err := SignFileData(store, file); err != nil {
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

	fileData, err := io.ReadAll(r)
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
func AssertUploadFakeToolsVersions(c *gc.C, stor storage.Storage, toolsDir, stream string, versions ...version.Binary) []*coretools.Tools {
	agentTools, err := UploadFakeToolsVersions(stor, toolsDir, stream, versions...)
	c.Assert(err, jc.ErrorIsNil)
	return agentTools
}

// UploadFakeTools puts fake tools into the supplied storage with a binary
// version matching jujuversion.Current; if jujuversion.Current's os type is different
// to the host os type, matching fake tools will be uploaded for that host os type.
func UploadFakeTools(c *gc.C, stor storage.Storage, toolsDir, stream string, arches ...string) {
	if len(arches) == 0 {
		arches = []string{arch.HostArch()}
	}
	toolsOS := set.NewStrings("ubuntu")
	toolsOS.Add(coreos.HostOSTypeName())
	var versions []version.Binary
	for _, arch := range arches {
		for _, osType := range toolsOS.Values() {
			v := version.Binary{
				Number:  jujuversion.Current,
				Arch:    arch,
				Release: osType,
			}
			versions = append(versions, v)
		}
	}
	c.Logf("uploading fake tool versions: %v", versions)
	_, err := UploadFakeToolsVersions(stor, toolsDir, stream, versions...)
	c.Assert(err, jc.ErrorIsNil)
}

// RemoveFakeTools deletes the fake tools from the supplied storage.
func RemoveFakeTools(c *gc.C, stor storage.Storage, toolsDir string) {
	c.Logf("removing fake tools")
	toolsVersion := coretesting.CurrentVersion()
	name := envtools.StorageName(toolsVersion, toolsDir)
	err := stor.Remove(name)
	c.Check(err, jc.ErrorIsNil)
	defaultBase := jujuversion.DefaultSupportedLTSBase()
	if !defaultBase.IsCompatible(coretesting.HostBase(c)) {
		toolsVersion.Release = "ubuntu"
		name := envtools.StorageName(toolsVersion, toolsDir)
		err := stor.Remove(name)
		c.Check(err, jc.ErrorIsNil)
	}
	RemoveFakeToolsMetadata(c, stor)
}

// RemoveTools deletes all tools from the supplied storage.
func RemoveTools(c *gc.C, stor storage.Storage, toolsDir string) {
	names, err := storage.List(stor, fmt.Sprintf("tools/%s/juju-", toolsDir))
	c.Assert(err, jc.ErrorIsNil)
	c.Logf("removing files: %v", names)
	for _, name := range names {
		err = stor.Remove(name)
		c.Check(err, jc.ErrorIsNil)
	}
	RemoveFakeToolsMetadata(c, stor)
}

var (
	V100    = version.MustParse("1.0.0")
	V100u64 = version.MustParseBinary("1.0.0-ubuntu-amd64")
	V100u32 = version.MustParseBinary("1.0.0-ubuntu-arm64")
	V100p   = []version.Binary{V100u64, V100u32}

	V1001    = version.MustParse("1.0.0.1")
	V1001u64 = version.MustParseBinary("1.0.0.1-ubuntu-amd64")

	V110    = version.MustParse("1.1.0")
	V110u64 = version.MustParseBinary("1.1.0-ubuntu-amd64")
	V110u32 = version.MustParseBinary("1.1.0-ubuntu-arm64")
	V110p   = []version.Binary{V110u64, V110u32}

	V120    = version.MustParse("1.2.0")
	V120u64 = version.MustParseBinary("1.2.0-ubuntu-amd64")
	V120u32 = version.MustParseBinary("1.2.0-ubuntu-arm64")
	V120all = []version.Binary{V120u64, V120u32}

	V1all = append(V100p, append(V110p, V120all...)...)

	V220    = version.MustParse("2.2.0")
	V220u32 = version.MustParseBinary("2.2.0-ubuntu-arm64")
	V220u64 = version.MustParseBinary("2.2.0-ubuntu-amd64")
	V220all = []version.Binary{V220u64, V220u32}
	VAll    = append(V1all, V220all...)
)
