// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"crypto/sha256"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/filestorage"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/environs/sync"
	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/provider/ec2"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/version"
)

var DefaultToolsLocation = sync.DefaultToolsLocation

// ToolsMetadataCommand is used to generate simplestreams metadata for
// juju tools.
type ToolsMetadataCommand struct {
	cmd.EnvCommandBase
	fetch       bool
	metadataDir string
}

func (c *ToolsMetadataCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "generate-tools",
		Purpose: "generate simplestreams tools metadata",
	}
}

func (c *ToolsMetadataCommand) SetFlags(f *gnuflag.FlagSet) {
	c.EnvCommandBase.SetFlags(f)
	f.BoolVar(&c.fetch, "fetch", true, "fetch tools and compute content size and hash")
	f.StringVar(&c.metadataDir, "d", "", "local directory in which to store metadata")
}

func (c *ToolsMetadataCommand) Run(context *cmd.Context) error {
	if c.metadataDir == "" {
		c.metadataDir = config.JujuHome()
	}
	c.metadataDir = utils.NormalizePath(c.metadataDir)

	// Create a StorageReader that will get a tools list from the local disk.
	// Since ReadList expects tools to be immediately under "tools/", and we
	// want them to be in tools/releases, we have to wrap the storage.
	sourceStorage, err := filestorage.NewFileStorageReader(c.metadataDir)
	if err != nil {
		return err
	}
	sourceStorage = prefixedToolsStorage{sourceStorage, "tools/"}

	fmt.Fprintln(context.Stdout, "Finding tools...")
	toolsList, err := tools.ReadList(sourceStorage, version.Current.Major)
	if err == tools.ErrNoTools {
		sourceStorage = ec2.NewHTTPStorageReader(sync.DefaultToolsLocation)
		toolsList, err = tools.ReadList(sourceStorage, version.Current.Major)
	}
	if err != nil {
		return err
	}

	metadata := make([]*tools.ToolsMetadata, len(toolsList))
	for i, t := range toolsList {
		var size int64
		var sha256hex string
		if c.fetch {
			fmt.Fprintln(context.Stdout, "Fetching tools to generate hash:", t.URL)
			var sha256hash hash.Hash
			size, sha256hash, err = fetchToolsHash(t.URL)
			if err != nil {
				return err
			}
			sha256hex = fmt.Sprintf("%x", sha256hash.Sum(nil))
		}

		path := fmt.Sprintf("releases/juju-%s-%s-%s.tgz", t.Version.Number, t.Version.Series, t.Version.Arch)
		metadata[i] = &tools.ToolsMetadata{
			Release:  t.Version.Series,
			Version:  t.Version.Number.String(),
			Arch:     t.Version.Arch,
			Path:     path,
			FileType: "tar.gz",
			Size:     size,
			SHA256:   sha256hex,
		}
	}

	index, products, err := tools.MarshalToolsMetadataJSON(metadata, time.Now())
	if err != nil {
		return err
	}
	objects := []struct {
		path string
		data []byte
	}{
		{simplestreams.DefaultIndexPath + simplestreams.UnsignedSuffix, index},
		{tools.ProductMetadataPath, products},
	}
	for _, object := range objects {
		path := filepath.Join(c.metadataDir, "tools", object.path)
		fmt.Fprintf(context.Stdout, "Writing %s\n", path)
		if err = writeFile(path, object.data); err != nil {
			return err
		}
	}
	return nil
}

func writeFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil && !os.IsExist(err) {
		return err
	}
	return ioutil.WriteFile(path, data, 0644)
}

// fetchToolsHash fetches the file at the specified URL,
// and calculates its size in bytes and computes a SHA256
// hash of its contents.
func fetchToolsHash(url string) (size int64, sha256hash hash.Hash, err error) {
	resp, err := http.Get(url)
	if err != nil {
		return 0, nil, err
	}
	sha256hash = sha256.New()
	size, err = io.Copy(sha256hash, resp.Body)
	resp.Body.Close()
	return size, sha256hash, err
}

type prefixedToolsStorage struct {
	environs.StorageReader
	prefix string
}

func (s prefixedToolsStorage) Get(name string) (io.ReadCloser, error) {
	return s.StorageReader.Get(name[len(s.prefix):])
}

func (s prefixedToolsStorage) List(prefix string) ([]string, error) {
	names, err := s.StorageReader.List(prefix[len(s.prefix):])
	if err == nil {
		for i, name := range names {
			names[i] = s.prefix + name
		}
	}
	return names, err
}

func (s prefixedToolsStorage) URL(name string) (string, error) {
	return s.StorageReader.URL(name[len(s.prefix):])
}
