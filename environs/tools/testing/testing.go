// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/environs/sync"
	"github.com/juju/juju/environs/tools"
	coretesting "github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

func GetMockBundleTools(c *gc.C) tools.BundleToolsFunc {
	return func(w io.Writer, forceVersion *version.Number) (vers version.Binary, sha256Hash string, err error) {
		vers = version.Current
		if forceVersion != nil {
			vers.Number = *forceVersion
		}
		sha256Hash = fmt.Sprintf("%x", sha256.New().Sum(nil))
		return vers, sha256Hash, err
	}
}

// GetMockBuildTools returns a sync.BuildToolsTarballFunc implementation which generates
// a fake tools tarball.
func GetMockBuildTools(c *gc.C) sync.BuildToolsTarballFunc {
	return func(forceVersion *version.Number, stream string) (*sync.BuiltTools, error) {
		vers := version.Current
		if forceVersion != nil {
			vers.Number = *forceVersion
		}

		tgz, checksum := coretesting.TarGz(
			coretesting.NewTarFile("jujud", 0777, "jujud contents "+vers.String()))

		toolsDir, err := ioutil.TempDir("", "juju-tools")
		c.Assert(err, gc.IsNil)
		name := "name"
		ioutil.WriteFile(filepath.Join(toolsDir, name), tgz, 0777)

		return &sync.BuiltTools{
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
		binary := version.MustParseBinary(versionString)
		path := filepath.Join(toolsDir, fmt.Sprintf("juju-%s.tgz", binary))
		data := binary.String()
		err := ioutil.WriteFile(path, []byte(data), 0644)
		c.Assert(err, gc.IsNil)
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
	stor, err := filestorage.NewFileStorageWriter(metadataDir)
	c.Assert(err, gc.IsNil)
	err = tools.MergeAndWriteMetadata(stor, stream, toolsList, false)
	c.Assert(err, gc.IsNil)
	return toolsList
}

// SHA256sum creates the sha256 checksum for the specified file.
func SHA256sum(c *gc.C, path string) (int64, string) {
	if strings.HasPrefix(path, "file://") {
		path = path[len("file://"):]
	}
	hash, size, err := utils.ReadFileSHA256(path)
	c.Assert(err, gc.IsNil)
	return size, hash
}

// ParseMetadataFromDir loads ToolsMetadata from the specified directory.
func ParseMetadataFromDir(c *gc.C, stream, metadataDir string, expectMirrors bool) []*tools.ToolsMetadata {
	stor, err := filestorage.NewFileStorageReader(metadataDir)
	c.Assert(err, gc.IsNil)
	return ParseMetadataFromStorage(c, stor, stream, expectMirrors)
}

// ParseMetadataFromStorage loads ToolsMetadata from the specified storage reader.
func ParseMetadataFromStorage(c *gc.C, stor storage.StorageReader, stream string, expectMirrors bool) []*tools.ToolsMetadata {
	source := storage.NewStorageSimpleStreamsDataSource("test storage reader", stor, "tools")
	params := simplestreams.ValueParams{
		DataType:      tools.ContentDownload,
		ValueTemplate: tools.ToolsMetadata{},
	}

	const requireSigned = false
	indexPath := simplestreams.UnsignedIndex("v1")
	mirrorsPath := simplestreams.MirrorsPath("v1")
	indexRef, err := simplestreams.GetIndexWithFormat(
		source, indexPath, "index:1.0", mirrorsPath, requireSigned, simplestreams.CloudSpec{}, params)
	c.Assert(err, gc.IsNil)
	c.Assert(indexRef.Indexes, gc.HasLen, 1)

	toolsIndexMetadata := indexRef.Indexes[tools.ToolsContentId(stream)]
	c.Assert(toolsIndexMetadata, gc.NotNil)

	// Read the products file contents.
	r, err := stor.Get(path.Join("tools", toolsIndexMetadata.ProductsFilePath))
	defer r.Close()
	c.Assert(err, gc.IsNil)
	data, err := ioutil.ReadAll(r)
	c.Assert(err, gc.IsNil)

	url, err := source.URL(toolsIndexMetadata.ProductsFilePath)
	c.Assert(err, gc.IsNil)
	cloudMetadata, err := simplestreams.ParseCloudMetadata(data, "products:1.0", url, tools.ToolsMetadata{})
	c.Assert(err, gc.IsNil)

	toolsMetadataMap := make(map[string]*tools.ToolsMetadata)
	var expectedProductIds set.Strings
	var toolsVersions set.Strings
	for _, mc := range cloudMetadata.Products {
		for _, items := range mc.Items {
			for key, item := range items.Items {
				toolsMetadata := item.(*tools.ToolsMetadata)
				toolsMetadataMap[key] = toolsMetadata
				toolsVersions.Add(key)
				seriesVersion, err := version.SeriesVersion(toolsMetadata.Release)
				c.Assert(err, gc.IsNil)
				productId := fmt.Sprintf("com.ubuntu.juju:%s:%s", seriesVersion, toolsMetadata.Arch)
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
		c.Assert(err, gc.IsNil)
		defer r.Close()
		data, err = ioutil.ReadAll(r)
		c.Assert(err, gc.IsNil)
		c.Assert(string(data), jc.Contains, `"mirrors":`)
		c.Assert(err, gc.IsNil)
	}
	return toolsMetadata
}

type metadataFile struct {
	path string
	data []byte
}

func generateMetadata(c *gc.C, stream string, versions ...version.Binary) []metadataFile {
	var metadata = make([]*tools.ToolsMetadata, len(versions))
	for i, vers := range versions {
		basePath := fmt.Sprintf("%s/tools-%s.tar.gz", stream, vers.String())
		metadata[i] = &tools.ToolsMetadata{
			Release: vers.Series,
			Version: vers.Number.String(),
			Arch:    vers.Arch,
			Path:    basePath,
		}
	}
	index, products, err := tools.MarshalToolsMetadataJSON(metadata, stream, time.Now())
	c.Assert(err, gc.IsNil)
	objects := []metadataFile{
		{simplestreams.UnsignedIndex("v1"), index},
		{tools.ProductMetadataPath(stream), products},
	}
	return objects
}

// UploadToStorage uploads tools and metadata for the specified versions to storage.
func UploadToStorage(c *gc.C, stor storage.Storage, stream string, versions ...version.Binary) map[version.Binary]string {
	uploaded := map[version.Binary]string{}
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
		c.Assert(err, gc.IsNil)
		uploaded[vers], err = stor.URL(filename)
		c.Assert(err, gc.IsNil)
	}
	objects := generateMetadata(c, stream, versions...)
	for _, object := range objects {
		toolspath := path.Join("tools", object.path)
		err = stor.Put(toolspath, bytes.NewReader(object.data), int64(len(object.data)))
		c.Assert(err, gc.IsNil)
	}
	return uploaded
}

// UploadToDirectory uploads tools and metadata for the specified versions to dir.
func UploadToDirectory(c *gc.C, stream, dir string, versions ...version.Binary) map[version.Binary]string {
	uploaded := map[version.Binary]string{}
	if len(versions) == 0 {
		return uploaded
	}
	for _, vers := range versions {
		basePath := fmt.Sprintf("%s/tools-%s.tar.gz", stream, vers.String())
		uploaded[vers] = fmt.Sprintf("file://%s/%s", dir, basePath)
	}
	objects := generateMetadata(c, stream, versions...)
	for _, object := range objects {
		path := filepath.Join(dir, object.path)
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil && !os.IsExist(err) {
			c.Assert(err, gc.IsNil)
		}
		err := ioutil.WriteFile(path, object.data, 0644)
		c.Assert(err, gc.IsNil)
	}
	return uploaded
}
