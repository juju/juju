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

	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/utils/set"
	"launchpad.net/juju-core/version"
)

// MakeTools creates some fake tools with the given version strings.
func MakeTools(c *gc.C, metadataDir, subdir string, versionStrings []string) {
	toolsDir := filepath.Join(metadataDir, "tools")
	if subdir != "" {
		toolsDir = filepath.Join(toolsDir, subdir)
	}
	c.Assert(os.MkdirAll(toolsDir, 0755), gc.IsNil)
	for _, versionString := range versionStrings {
		binary := version.MustParseBinary(versionString)
		path := filepath.Join(toolsDir, fmt.Sprintf("juju-%s.tgz", binary))
		err := ioutil.WriteFile(path, []byte(binary.String()), 0644)
		c.Assert(err, gc.IsNil)
	}
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
func ParseMetadata(c *gc.C, metadataDir string) []*tools.ToolsMetadata {
	params := simplestreams.ValueParams{
		DataType:      tools.ContentDownload,
		ValueTemplate: tools.ToolsMetadata{},
	}

	source := simplestreams.NewURLDataSource("file://"+metadataDir+"/tools", simplestreams.VerifySSLHostnames)

	const requireSigned = false
	indexPath := simplestreams.DefaultIndexPath + simplestreams.UnsignedSuffix
	indexRef, err := simplestreams.GetIndexWithFormat(source, indexPath, "index:1.0", requireSigned, params)
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
	return toolsMetadata
}
