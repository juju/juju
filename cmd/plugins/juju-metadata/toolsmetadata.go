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
	"net/url"
	"os"
	"path/filepath"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/environs/tools"
	coretools "launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/version"
)

// pathPrefix is the prefix for metadata paths.
const pathPrefix = "tools/"

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
	// TODO(axw) allow user to specify version
}

func (c *ToolsMetadataCommand) Run(context *cmd.Context) error {
	env, err := environs.NewFromName(c.EnvName)
	if err != nil {
		return err
	}

	fmt.Fprintln(context.Stdout, "Finding tools...")
	toolsList, err := tools.FindTools(env, version.Current.Major, coretools.Filter{})
	if err != nil {
		return err
	}

	metadata := make([]*tools.ToolsMetadata, len(toolsList))
	for i, t := range toolsList {
		u, err := url.Parse(t.URL)
		if err != nil {
			return err
		}
		urlPath := u.Path[1:]
		// FIXME(axw) path should be relative to base URL. We don't know whether
		// it's from the public or private storage at this point.

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

		metadata[i] = &tools.ToolsMetadata{
			Release:  t.Version.Series,
			Version:  t.Version.Number.String(),
			Arch:     t.Version.Arch,
			Path:     urlPath,
			FileType: "tar.gz",
			Size:     size,
			SHA256:   sha256hex,
		}
	}

	if c.metadataDir == "" {
		c.metadataDir = config.JujuHome()
	}
	c.metadataDir = utils.NormalizePath(c.metadataDir)

	index, products, err := tools.MarshalToolsMetadataJSON(metadata)
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
		path := filepath.Join(c.metadataDir, pathPrefix, object.path)
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
