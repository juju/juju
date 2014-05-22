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
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/filestorage"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/environs/storage"
	"launchpad.net/juju-core/environs/sync"
	"launchpad.net/juju-core/environs/tools"
	coretesting "launchpad.net/juju-core/testing"
	coretools "launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/utils/set"
	"launchpad.net/juju-core/version"
	"launchpad.net/juju-core/version/ubuntu"
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
	return func(forceVersion *version.Number) (*sync.BuiltTools, error) {
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
func MakeTools(c *gc.C, metadataDir, subdir string, versionStrings []string) coretools.List {
	return makeTools(c, metadataDir, subdir, versionStrings, false)
}

// MakeToolsWithCheckSum creates some fake tools (including checksums) with the given version strings.
func MakeToolsWithCheckSum(c *gc.C, metadataDir, subdir string, versionStrings []string) coretools.List {
	return makeTools(c, metadataDir, subdir, versionStrings, true)
}

func makeTools(c *gc.C, metadataDir, subdir string, versionStrings []string, withCheckSum bool) coretools.List {
	toolsDir := filepath.Join(metadataDir, storage.BaseToolsPath)
	if subdir != "" {
		toolsDir = filepath.Join(toolsDir, subdir)
	}
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
	err = tools.MergeAndWriteMetadata(stor, toolsList, false)
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
func ParseMetadataFromDir(c *gc.C, metadataDir string, expectMirrors bool) []*tools.ToolsMetadata {
	stor, err := filestorage.NewFileStorageReader(metadataDir)
	c.Assert(err, gc.IsNil)
	return ParseMetadataFromStorage(c, stor, expectMirrors)
}

// ParseMetadataFromStorage loads ToolsMetadata from the specified storage reader.
func ParseMetadataFromStorage(c *gc.C, stor storage.StorageReader, expectMirrors bool) []*tools.ToolsMetadata {
	source := storage.NewStorageSimpleStreamsDataSource("test storage reader", stor, "tools")
	params := simplestreams.ValueParams{
		DataType:      tools.ContentDownload,
		ValueTemplate: tools.ToolsMetadata{},
	}

	const requireSigned = false
	indexPath := simplestreams.UnsignedIndex
	indexRef, err := simplestreams.GetIndexWithFormat(
		source, indexPath, "index:1.0", requireSigned, simplestreams.CloudSpec{}, params)
	c.Assert(err, gc.IsNil)
	c.Assert(indexRef.Indexes, gc.HasLen, 1)

	toolsIndexMetadata := indexRef.Indexes["com.ubuntu.juju:released:tools"]
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
				seriesVersion, err := ubuntu.SeriesVersion(toolsMetadata.Release)
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
		r, err = stor.Get(path.Join("tools", simplestreams.UnsignedMirror))
		defer r.Close()
		c.Assert(err, gc.IsNil)
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

func generateMetadata(c *gc.C, versions ...version.Binary) []metadataFile {
	var metadata = make([]*tools.ToolsMetadata, len(versions))
	for i, vers := range versions {
		basePath := fmt.Sprintf("releases/tools-%s.tar.gz", vers.String())
		metadata[i] = &tools.ToolsMetadata{
			Release: vers.Series,
			Version: vers.Number.String(),
			Arch:    vers.Arch,
			Path:    basePath,
		}
	}
	index, products, err := tools.MarshalToolsMetadataJSON(metadata, time.Now())
	c.Assert(err, gc.IsNil)
	objects := []metadataFile{
		{simplestreams.UnsignedIndex, index},
		{tools.ProductMetadataPath, products},
	}
	return objects
}

// UploadToStorage uploads tools and metadata for the specified versions to storage.
func UploadToStorage(c *gc.C, stor storage.Storage, versions ...version.Binary) map[version.Binary]string {
	uploaded := map[version.Binary]string{}
	if len(versions) == 0 {
		return uploaded
	}
	var err error
	for _, vers := range versions {
		filename := fmt.Sprintf("tools/releases/tools-%s.tar.gz", vers.String())
		// Put a file in images since the dummy storage provider requires a
		// file to exist before the URL can be found. This is to ensure it behaves
		// the same way as MAAS.
		err = stor.Put(filename, strings.NewReader("dummy"), 5)
		c.Assert(err, gc.IsNil)
		uploaded[vers], err = stor.URL(filename)
		c.Assert(err, gc.IsNil)
	}
	objects := generateMetadata(c, versions...)
	for _, object := range objects {
		toolspath := path.Join("tools", object.path)
		err = stor.Put(toolspath, bytes.NewReader(object.data), int64(len(object.data)))
		c.Assert(err, gc.IsNil)
	}
	return uploaded
}

// UploadToDirectory uploads tools and metadata for the specified versions to dir.
func UploadToDirectory(c *gc.C, dir string, versions ...version.Binary) map[version.Binary]string {
	uploaded := map[version.Binary]string{}
	if len(versions) == 0 {
		return uploaded
	}
	for _, vers := range versions {
		basePath := fmt.Sprintf("releases/tools-%s.tar.gz", vers.String())
		uploaded[vers] = fmt.Sprintf("file://%s/%s", dir, basePath)
	}
	objects := generateMetadata(c, versions...)
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
