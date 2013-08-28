// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"net/http"
	"net/url"
	"time"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/environs/tools"
	coretools "launchpad.net/juju-core/tools"
)

// ToolsMetadataCommand is used to write out a boilerplate environments.yaml file.
type ToolsMetadataCommand struct {
	cmd.EnvCommandBase
	fetch bool
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
}

func (c *ToolsMetadataCommand) Run(context *cmd.Context) error {
	env, err := environs.NewFromName(c.EnvName)
	if err != nil {
		return err
	}

	fmt.Println("Finding tools...")
	toolsList, err := tools.FindTools(env, 1, coretools.Filter{})
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

		var size float64
		var sha256hex string
		if c.fetch {
			fmt.Println("Fetching tools to generate hash:", t.URL)
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
			ProductsFilePath: "streams/v1/com.ubuntu.juju:released:tools.json",
			ProductIds:       productIds,
		},
	}

	out := context.Stdout
	data, err := json.MarshalIndent(&indices, "", "    ")
	if err != nil {
		return err
	}
	if _, err = out.Write(data); err != nil {
		return err
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out)
	data, err = json.MarshalIndent(&cloud, "", "    ")
	if err != nil {
		return err
	}
	if _, err = out.Write(data); err != nil {
		return err
	}
	fmt.Fprintln(out)
	return nil
}

// fetchToolsHash fetches the file at the specified URL,
// and calculates its size in bytes and computes a SHA256
// hash of its contents.
func fetchToolsHash(url string) (size float64, sha256hash hash.Hash, err error) {
	resp, err := http.Get(url)
	if err != nil {
		return 0, nil, err
	}
	sha256hash = sha256.New()
	sizeint, err := io.Copy(sha256hash, resp.Body)
	resp.Body.Close()
	return float64(sizeint), sha256hash, err
}
