// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"time"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/localstorage"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/environs/tools"
	coretools "launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/version"
)

const toolsIndexMetadataPath = "tools/streams/v1/index.json"
const toolsProductMetadataPath = "tools/streams/v1/com.ubuntu.juju:released:tools.json"

// ToolsMetadataCommand is used to write out a boilerplate environments.yaml file.
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
	f.StringVar(&c.metadataDir, "d", "", "local directory to locate tools and store metadata")
	// TODO(axw) allow user to specify version
}

func (c *ToolsMetadataCommand) Run(context *cmd.Context) error {
	env, err := environs.NewFromName(c.EnvName)
	if err != nil {
		return err
	}

	if c.metadataDir != "" {
		c.metadataDir = utils.NormalizePath(c.metadataDir)
		listener, err := localstorage.Serve("127.0.0.1:0", c.metadataDir)
		if err != nil {
			return err
		}
		defer listener.Close()
		storageAddr := listener.Addr().String()
		env = localdirEnv{env, localstorage.Client(storageAddr)}
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
		// We strip the leading /, to be consistent with image metadata.
		urlPath := u.Path[1:]

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

	var cloud simplestreams.CloudMetadata
	updated := time.Now().Format(time.RFC1123Z)
	cloud.Updated = updated
	cloud.Format = "products:1.0"
	cloud.Products = make(map[string]simplestreams.MetadataCatalog)
	productIds := make([]string, len(metadata))
	itemsversion := time.Now().Format("20060201") // YYYYMMDD
	for i, t := range metadata {
		id := fmt.Sprintf("com.ubuntu.juju:%s:%s", t.Version, t.Arch)
		productIds[i] = id
		itemid := fmt.Sprintf("%s-%s-%s", t.Version, t.Release, t.Arch)
		if catalog, ok := cloud.Products[id]; ok {
			catalog.Items[itemsversion].Items[itemid] = t
		} else {
			catalog = simplestreams.MetadataCatalog{
				Arch:    t.Arch,
				Version: t.Version,
				Items: map[string]*simplestreams.ItemCollection{
					itemsversion: &simplestreams.ItemCollection{
						Items: map[string]interface{}{itemid: t},
					},
				},
			}
			cloud.Products[id] = catalog
		}
	}

	var indices simplestreams.Indices
	indices.Updated = updated
	indices.Format = "index:1.0"
	indices.Indexes = map[string]*simplestreams.IndexMetadata{
		"com.ubuntu.juju:released:tools": &simplestreams.IndexMetadata{
			Updated:          updated,
			Format:           "products:1.0",
			DataType:         "content-download",
			ProductsFilePath: toolsProductMetadataPath,
			ProductIds:       productIds,
		},
	}

	storage := env.Storage()
	objects := []struct {
		path   string
		object interface{}
	}{
		{toolsIndexMetadataPath, &indices},
		{toolsProductMetadataPath, &cloud},
	}
	for _, object := range objects {
		var path string
		if c.metadataDir != "" {
			path = filepath.Join(c.metadataDir, object.path)
		} else {
			objectUrl, err := storage.URL(object.path)
			if err != nil {
				return err
			}
			path = objectUrl
		}
		fmt.Fprintf(context.Stdout, "Writing %s\n", path)
		buf, err := marshalIndent(object.object)
		if err != nil {
			return err
		}
		if err = storage.Put(object.path, buf, int64(buf.Len())); err != nil {
			return err
		}
	}
	return nil
}

func marshalIndent(v interface{}) (*bytes.Buffer, error) {
	out, err := json.MarshalIndent(v, "", "    ")
	if err != nil {
		return nil, err
	}
	return bytes.NewBuffer(out), nil
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

// localdirEnv wraps an Environ, returning a localstorage Storage
// implementation, and ensuring no PublicStorage is available.
type localdirEnv struct {
	environs.Environ
	storage environs.Storage
}

func (e localdirEnv) Storage() environs.Storage {
	return e.storage
}

func (e localdirEnv) PublicStorage() environs.StorageReader {
	// If there's no matching tools in Storage(), FindTools
	// will fall back to environs.EmptyStorage.
	return environs.EmptyStorage
}
