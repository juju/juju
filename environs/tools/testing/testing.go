// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/filestorage"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/environs/storage"
	"launchpad.net/juju-core/environs/tools"
	jc "launchpad.net/juju-core/testing/checkers"
	coretools "launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/utils/set"
	"launchpad.net/juju-core/version"
)

// MakeTools creates some fake tools with the given version strings.
func MakeTools(c *gc.C, metadataDir, subdir string, versionStrings []string) {
	makeTools(c, metadataDir, subdir, versionStrings, false)
}

// MakeTools creates some fake tools (including checksums) with the given version strings.
func MakeToolsWithCheckSum(c *gc.C, metadataDir, subdir string, versionStrings []string) {
	makeTools(c, metadataDir, subdir, versionStrings, true)
}

func makeTools(c *gc.C, metadataDir, subdir string, versionStrings []string, withCheckSum bool) {
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
	stor, err := filestorage.NewFileStorageWriter(metadataDir, filestorage.UseDefaultTmpDir)
	c.Assert(err, gc.IsNil)
	err = tools.MergeAndWriteMetadata(stor, toolsList, false)
	c.Assert(err, gc.IsNil)
}

// SHA256sum creates the sha256 checksum for the specified file.
func SHA256sum(c *gc.C, path string) (int64, string) {
	if strings.HasPrefix(path, "file://") {
		path = path[len("file://"):]
	}
	f, err := os.Open(path)
	c.Assert(err, gc.IsNil)
	defer f.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, f)
	c.Assert(err, gc.IsNil)
	return size, fmt.Sprintf("%x", hash.Sum(nil))
}

// ParseMetadata loads ToolsMetadata from the specified directory.
func ParseMetadata(c *gc.C, metadataDir string, expectMirrors bool) []*tools.ToolsMetadata {
	params := simplestreams.ValueParams{
		DataType:      tools.ContentDownload,
		ValueTemplate: tools.ToolsMetadata{},
	}

	source := simplestreams.NewURLDataSource("file://"+metadataDir+"/tools", simplestreams.VerifySSLHostnames)

	const requireSigned = false
	indexPath := simplestreams.UnsignedIndex
	indexRef, err := simplestreams.GetIndexWithFormat(
		source, indexPath, "index:1.0", requireSigned, simplestreams.CloudSpec{}, params)
	c.Assert(err, gc.IsNil)
	c.Assert(indexRef.Indexes, gc.HasLen, 1)

	toolsIndexMetadata := indexRef.Indexes["com.ubuntu.juju:released:tools"]
	c.Assert(toolsIndexMetadata, gc.NotNil)

	data, err := ioutil.ReadFile(filepath.Join(metadataDir, "tools", toolsIndexMetadata.ProductsFilePath))
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
				seriesVersion, err := simplestreams.SeriesVersion(toolsMetadata.Release)
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
		data, err := ioutil.ReadFile(filepath.Join(metadataDir, "tools", simplestreams.UnsignedMirror))
		c.Assert(string(data), jc.Contains, `"mirrors":`)
		c.Assert(err, gc.IsNil)
	}
	return toolsMetadata
}
