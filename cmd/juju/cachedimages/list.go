// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for infos.

package cachedimages

import (
	"fmt"
	"time"

	"github.com/juju/cmd"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/apiserver/params"
)

const ListCommandDoc = `
List cached os images in the Juju environment.

Images can be filtered on:
  Kind         eg "lxc"
  Series       eg "trusty"
  Architecture eg "amd64"
The filter attributes are optional.

Examples:

  # List all cached images.
  juju cache-images list

  # List cached images for trusty.
  juju cache-images list --series trusty

  # List all cached lxc images for trusty amd64.
  juju cache-images list --kind lxc --series trusty --arch amd64
`

// ListCommand shows the images in the Juju server.
type ListCommand struct {
	CachedImagesCommandBase
	out                cmd.Output
	Kind, Series, Arch string
}

// Info implements Command.Info.
func (c *ListCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "list",
		Purpose: "shows cached os images",
		Doc:     ListCommandDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *ListCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CachedImagesCommandBase.SetFlags(f)
	f.StringVar(&c.Kind, "kind", "", "the image kind to list eg lxc")
	f.StringVar(&c.Series, "series", "", "the series of the image to list eg trusty")
	f.StringVar(&c.Arch, "arch", "", "the architecture of the image to list eg amd64")
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
}

// Init implements Command.Init.
func (c *ListCommand) Init(args []string) (err error) {
	return cmd.CheckEmpty(args)
}

// ListImagesAPI defines the imagemanager API methods that the list command uses.
type ListImagesAPI interface {
	ListImages(kind, series, arch string) ([]params.ImageMetadata, error)
	Close() error
}

var getListImagesAPI = func(p *ListCommand) (ListImagesAPI, error) {
	return p.NewImagesManagerClient()
}

// ImageInfo defines the serialization behaviour of image metadata.
type ImageInfo struct {
	Kind      string `yaml:"kind" json:"kind"`
	Series    string `yaml:"series" json:"series"`
	Arch      string `yaml:"arch" json:"arch"`
	SourceURL string `yaml:"source-url" json:"source-url"`
	Created   string `yaml:"created" json:"created"`
}

func (c *ListCommand) imageMetadataToImageInfo(images []params.ImageMetadata) []ImageInfo {
	var output []ImageInfo
	for _, metadata := range images {
		imageInfo := ImageInfo{
			Kind:      metadata.Kind,
			Series:    metadata.Series,
			Arch:      metadata.Arch,
			Created:   metadata.Created.Format(time.RFC1123),
			SourceURL: metadata.URL,
		}
		output = append(output, imageInfo)
	}
	return output
}

// Run implements Command.Run.
func (c *ListCommand) Run(ctx *cmd.Context) (err error) {
	client, err := getListImagesAPI(c)
	if err != nil {
		return err
	}
	defer client.Close()

	results, err := client.ListImages(c.Kind, c.Series, c.Arch)
	if err != nil {
		return err
	}
	imageInfo := c.imageMetadataToImageInfo(results)
	if len(imageInfo) == 0 {
		fmt.Fprintf(ctx.Stdout, "no matching images found\n")
		return nil
	}
	fmt.Fprintf(ctx.Stdout, "Cached images:\n")
	return c.out.Write(ctx, imageInfo)
}
