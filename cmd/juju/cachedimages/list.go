// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for infos.

package cachedimages

import (
	"fmt"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
)

const listCommandDoc = `
List cached os images in the Juju model.

Images can be filtered on:
  Kind         eg "lxd"
  Series       eg "xenial"
  Architecture eg "amd64"
The filter attributes are optional.

Examples:
  # List all cached images.
  juju cached-images

  # List cached images for xenial.
  juju cached-images --series xenial

  # List all cached lxd images for xenial amd64.
  juju cached-images --kind lxd --series xenial --arch amd64
`

// NewListCommand returns a command for listing chached images.
func NewListCommand() cmd.Command {
	return modelcmd.Wrap(&listCommand{})
}

// listCommand shows the images in the Juju server.
type listCommand struct {
	CachedImagesCommandBase
	out                cmd.Output
	Kind, Series, Arch string
}

// Info implements Command.Info.
func (c *listCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "cached-images",
		Purpose: "Shows cached os images.",
		Doc:     listCommandDoc,
		Aliases: []string{"list-cached-images"},
	}
}

// SetFlags implements Command.SetFlags.
func (c *listCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CachedImagesCommandBase.SetFlags(f)
	f.StringVar(&c.Kind, "kind", "", "The image kind to list eg lxd")
	f.StringVar(&c.Series, "series", "", "The series of the image to list eg xenial")
	f.StringVar(&c.Arch, "arch", "", "The architecture of the image to list eg amd64")
	c.out.AddFlags(f, "yaml", output.DefaultFormatters)
}

// Init implements Command.Init.
func (c *listCommand) Init(args []string) error {
	return cmd.CheckEmpty(args)
}

// ListImagesAPI defines the imagemanager API methods that the list command uses.
type ListImagesAPI interface {
	ListImages(kind, series, arch string) ([]params.ImageMetadata, error)
	Close() error
}

var getListImagesAPI = func(p *CachedImagesCommandBase) (ListImagesAPI, error) {
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

func (c *listCommand) imageMetadataToImageInfo(images []params.ImageMetadata) []ImageInfo {
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
func (c *listCommand) Run(ctx *cmd.Context) (err error) {
	client, err := getListImagesAPI(&c.CachedImagesCommandBase)
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
