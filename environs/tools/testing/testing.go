// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v4"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/simplestreams"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/environs/sync"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/environs/tools"
	coretesting "github.com/juju/juju/internal/testing"
	coretools "github.com/juju/juju/internal/tools"
	"github.com/juju/juju/juju/names"
)

func GetMockBundleTools(expectedForceVersion semversion.Number) tools.BundleToolsFunc {
	return func(
		build bool, w io.Writer,
		getForceVersion func(semversion.Number) semversion.Number,
	) (semversion.Binary, semversion.Number, bool, string, error) {
		vers := coretesting.CurrentVersion()
		forceVersion := getForceVersion(vers.Number)
		if forceVersion.Compare(expectedForceVersion) != 0 {
			return semversion.Binary{}, semversion.Number{}, false, "", errors.Errorf("%#v != expected %#v", forceVersion, expectedForceVersion)
		}
		sha256Hash := fmt.Sprintf("%x", sha256.New().Sum(nil))
		return vers, forceVersion, false, sha256Hash, nil
	}
}

// GetMockBuildTools returns a sync.BuildAgentTarballFunc implementation which generates
// a fake tools tarball.
func GetMockBuildTools(c *gc.C) sync.BuildAgentTarballFunc {
	return func(
		build bool, stream string,
		getForceVersion func(semversion.Number) semversion.Number,
	) (*sync.BuiltAgent, error) {
		vers := coretesting.CurrentVersion()
		vers.Number = getForceVersion(vers.Number)

		tgz, checksum := coretesting.TarGz(
			coretesting.NewTarFile(names.Jujud, 0777, "jujud contents "+vers.String()))

		toolsDir, err := os.MkdirTemp("", "juju-tools-"+stream)
		c.Assert(err, jc.ErrorIsNil)
		name := "name"
		_ = os.WriteFile(filepath.Join(toolsDir, name), tgz, 0777)

		return &sync.BuiltAgent{
			Dir:         toolsDir,
			StorageName: name,
			Version:     vers,
			Size:        int64(len(tgz)),
			Sha256Hash:  checksum,
		}, nil
	}
}

// MakeTools creates some fake tools with the given version strings.
func MakeTools(c *gc.C, metadataDir, stream string, versionStrings []string) coretools.List {
	return makeTools(c, metadataDir, stream, versionStrings, false)
}

// MakeToolsWithCheckSum creates some fake tools (including checksums) with the given version strings.
func MakeToolsWithCheckSum(c *gc.C, metadataDir, stream string, versionStrings []string) coretools.List {
	return makeTools(c, metadataDir, stream, versionStrings, true)
}

func makeTools(c *gc.C, metadataDir, stream string, versionStrings []string, withCheckSum bool) coretools.List {
	toolsDir := filepath.Join(metadataDir, storage.BaseToolsPath, stream)
	c.Assert(os.MkdirAll(toolsDir, 0755), gc.IsNil)
	var toolsList coretools.List
	for _, versionString := range versionStrings {
		binary, err := semversion.ParseBinary(versionString)
		c.Assert(err, jc.ErrorIsNil)
		path := filepath.Join(toolsDir, fmt.Sprintf("juju-%s.tgz", binary))
		data := binary.String()
		err = os.WriteFile(path, []byte(data), 0644)
		c.Assert(err, jc.ErrorIsNil)
		tool := &coretools.Tools{
			Version: binary,
			URL:     path,
		}
		if withCheckSum {
			tool.Size, tool.SHA256 = SHA256sum(c, path)
		}
		toolsList = append(toolsList, tool)
	}
	// Write the tools metadata.
	store, err := filestorage.NewFileStorageWriter(metadataDir)
	c.Assert(err, jc.ErrorIsNil)

	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	err = tools.MergeAndWriteMetadata(context.Background(), ss, store, stream, stream, toolsList, false)
	c.Assert(err, jc.ErrorIsNil)

	// Sign metadata
	err = envtesting.SignTestTools(store)
	c.Assert(err, jc.ErrorIsNil)
	return toolsList
}

// SHA256sum creates the sha256 checksum for the specified file.
func SHA256sum(c *gc.C, path string) (int64, string) {
	path = strings.TrimPrefix(path, "file://")
	hash, size, err := utils.ReadFileSHA256(path)
	c.Assert(err, jc.ErrorIsNil)
	return size, hash
}

// ParseMetadataFromDir loads ToolsMetadata from the specified directory.
func ParseMetadataFromDir(c *gc.C, metadataDir, stream string, expectMirrors bool) []*tools.ToolsMetadata {
	stor, err := filestorage.NewFileStorageReader(metadataDir)
	c.Assert(err, jc.ErrorIsNil)
	return ParseMetadataFromStorage(c, stor, stream, expectMirrors)
}

// ParseMetadataFromStorage loads ToolsMetadata from the specified storage reader.
func ParseMetadataFromStorage(c *gc.C, stor storage.StorageReader, stream string, expectMirrors bool) []*tools.ToolsMetadata {
	source := storage.NewStorageSimpleStreamsDataSource("test storage reader", stor, "tools", simplestreams.CUSTOM_CLOUD_DATA, false)
	params := simplestreams.ValueParams{
		DataType:      tools.ContentDownload,
		ValueTemplate: tools.ToolsMetadata{},
	}

	const requireSigned = false
	indexPath := simplestreams.UnsignedIndex("v1", 2)
	mirrorsPath := simplestreams.MirrorsPath("v1")

	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	indexRef, err := ss.GetIndexWithFormat(
		context.Background(),
		source, indexPath, "index:1.0", mirrorsPath, requireSigned, simplestreams.CloudSpec{}, params)
	c.Assert(err, jc.ErrorIsNil)

	toolsIndexMetadata := indexRef.Indexes[tools.ToolsContentId(stream)]
	c.Assert(toolsIndexMetadata, gc.NotNil)

	// Read the products file contents.
	r, err := stor.Get(path.Join("tools", toolsIndexMetadata.ProductsFilePath))
	defer func() { _ = r.Close() }()
	c.Assert(err, jc.ErrorIsNil)
	data, err := io.ReadAll(r)
	c.Assert(err, jc.ErrorIsNil)

	url, err := source.URL(toolsIndexMetadata.ProductsFilePath)
	c.Assert(err, jc.ErrorIsNil)
	cloudMetadata, err := simplestreams.ParseCloudMetadata(data, "products:1.0", url, tools.ToolsMetadata{})
	c.Assert(err, jc.ErrorIsNil)

	toolsMetadataMap := make(map[string]*tools.ToolsMetadata)
	expectedProductIds := make(set.Strings)
	toolsVersions := make(set.Strings)
	for _, mc := range cloudMetadata.Products {
		for _, items := range mc.Items {
			for key, item := range items.Items {
				toolsMetadata := item.(*tools.ToolsMetadata)
				toolsMetadataMap[key] = toolsMetadata
				toolsVersions.Add(key)
				productId := fmt.Sprintf("com.ubuntu.juju:%s:%s", toolsMetadata.Release, toolsMetadata.Arch)
				expectedProductIds.Add(productId)
			}
		}
	}

	// Make sure index's product IDs are all represented in the products metadata.
	sort.Strings(toolsIndexMetadata.ProductIds)
	c.Assert(toolsIndexMetadata.ProductIds, gc.DeepEquals, expectedProductIds.SortedValues())

	toolsMetadata := make([]*tools.ToolsMetadata, len(toolsMetadataMap))
	for i, key := range toolsVersions.SortedValues() {
		toolsMetadata[i] = toolsMetadataMap[key]
	}

	if expectMirrors {
		r, err = stor.Get(path.Join("tools", simplestreams.UnsignedMirror("v1")))
		c.Assert(err, jc.ErrorIsNil)
		defer r.Close()
		data, err = io.ReadAll(r)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(string(data), jc.Contains, `"mirrors":`)
		c.Assert(string(data), jc.Contains, tools.ToolsContentId(stream))
		c.Assert(err, jc.ErrorIsNil)
	}
	return toolsMetadata
}

type metadataFile struct {
	path string
	data []byte
}

func generateMetadata(c *gc.C, streamVersions StreamVersions) []metadataFile {
	streamMetadata := map[string][]*tools.ToolsMetadata{}
	for stream, versions := range streamVersions {
		metadata := make([]*tools.ToolsMetadata, len(versions))
		for i, vers := range versions {
			basePath := fmt.Sprintf("%s/tools-%s.tar.gz", stream, vers.String())
			metadata[i] = &tools.ToolsMetadata{
				Release: vers.Release,
				Version: vers.Number.String(),
				Arch:    vers.Arch,
				Path:    basePath,
			}
		}
		streamMetadata[stream] = metadata
	}
	// TODO(perrito666) 2016-05-02 lp:1558657
	index, legacyIndex, products, err := tools.MarshalToolsMetadataJSON(streamMetadata, time.Now())
	c.Assert(err, jc.ErrorIsNil)

	objects := []metadataFile{}
	addTools := func(fileName string, content []byte) {
		// add unsigned
		objects = append(objects, metadataFile{fileName, content})

		signedFilename, signedContent, err := sstesting.SignMetadata(fileName, content)
		c.Assert(err, jc.ErrorIsNil)

		// add signed
		objects = append(objects, metadataFile{signedFilename, signedContent})
	}

	addTools(simplestreams.UnsignedIndex("v1", 2), index)
	if legacyIndex != nil {
		addTools(simplestreams.UnsignedIndex("v1", 1), legacyIndex)
	}
	for stream, metadata := range products {
		addTools(tools.ProductMetadataPath(stream), metadata)
	}
	return objects
}

// UploadToStorage uploads tools and metadata for the specified versions to storage.
func UploadToStorage(c *gc.C, stor storage.Storage, stream string, versions ...semversion.Binary) map[semversion.Binary]string {
	uploaded := map[semversion.Binary]string{}
	if len(versions) == 0 {
		return uploaded
	}
	var err error
	for _, vers := range versions {
		filename := fmt.Sprintf("tools/%s/tools-%s.tar.gz", stream, vers.String())
		// Put a file in images since the dummy storage provider requires a
		// file to exist before the URL can be found. This is to ensure it behaves
		// the same way as MAAS.
		err = stor.Put(filename, strings.NewReader("dummy"), 5)
		c.Assert(err, jc.ErrorIsNil)
		uploaded[vers], err = stor.URL(filename)
		c.Assert(err, jc.ErrorIsNil)
	}
	objects := generateMetadata(c, StreamVersions{stream: versions})
	for _, object := range objects {
		toolspath := path.Join("tools", object.path)
		err = stor.Put(toolspath, bytes.NewReader(object.data), int64(len(object.data)))
		c.Assert(err, jc.ErrorIsNil)
	}
	return uploaded
}

// StreamVersions is a map of stream name to binaries in that stream.
type StreamVersions map[string][]semversion.Binary

// UploadToDirectory uploads tools and metadata for the specified versions to dir.
func UploadToDirectory(c *gc.C, dir string, streamVersions StreamVersions) map[string]map[semversion.Binary]string {
	allUploaded := map[string]map[semversion.Binary]string{}
	if len(streamVersions) == 0 {
		return allUploaded
	}
	for stream, versions := range streamVersions {
		uploaded := map[semversion.Binary]string{}
		for _, vers := range versions {
			basePath := fmt.Sprintf("%s/tools-%s.tar.gz", stream, vers.String())
			uploaded[vers] = utils.MakeFileURL(fmt.Sprintf("%s/%s", dir, basePath))
		}
		allUploaded[stream] = uploaded
	}
	objects := generateMetadata(c, streamVersions)
	for _, object := range objects {
		path := filepath.Join(dir, object.path)
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil && !os.IsExist(err) {
			c.Assert(err, jc.ErrorIsNil)
		}
		err := os.WriteFile(path, object.data, 0644)
		c.Assert(err, jc.ErrorIsNil)
	}
	return allUploaded
}
