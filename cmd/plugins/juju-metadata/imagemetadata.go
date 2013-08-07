// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"strings"

	"launchpad.net/gnuflag"
	"launchpad.net/goose/identity"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/environs/imagemetadata"
)

// ImageMetadataCommand is used to write out a boilerplate environments.yaml file.
type ImageMetadataCommand struct {
	cmd.CommandBase
	Name     string
	Series   string
	Arch     string
	ImageId  string
	Region   string
	Endpoint string
}

func (c *ImageMetadataCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "generate-image",
		Purpose: "generate simplestreams image metadata",
	}
}

func (c *ImageMetadataCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.Series, "s", "precise", "the charm series")
	f.StringVar(&c.Arch, "a", "amd64", "the image achitecture")
	f.StringVar(&c.Name, "n", "", "the cloud name, as a prefix for the generated file names")
	f.StringVar(&c.ImageId, "i", "", "the image id")
	f.StringVar(&c.Region, "r", "", "the region")
	f.StringVar(&c.Endpoint, "u", "", "the cloud endpoint (for Openstack, this is the Identity Service endpoint)")
}

func (c *ImageMetadataCommand) Init(args []string) error {
	cred := identity.CredentialsFromEnv()
	if c.Region == "" {
		c.Region = cred.Region
	}
	if c.Endpoint == "" {
		c.Endpoint = cred.URL
	}
	if c.ImageId == "" {
		return fmt.Errorf("image id must be specified")
	}
	if c.Region == "" {
		return fmt.Errorf("image region must be specified")
	}
	if c.Endpoint == "" {
		return fmt.Errorf("cloud endpoint URL must be specified")
	}

	return cmd.CheckEmpty(args)
}

func (c *ImageMetadataCommand) Run(context *cmd.Context) error {
	out := context.Stdout

	im := imagemetadata.ImageMetadata{
		Id:   c.ImageId,
		Arch: c.Arch,
	}
	cloudSpec := simplestreams.CloudSpec{
		Region:   c.Region,
		Endpoint: c.Endpoint,
	}
	files, err := imagemetadata.Boilerplate(c.Name, c.Series, &im, &cloudSpec)
	if err != nil {
		return fmt.Errorf("boilerplate image metadata files could not be created: %v", err)
	}
	fmt.Fprintf(
		out,
		"Boilerplate image metadata files %q have been written to %s.\n", strings.Join(files, ", "), config.JujuHome())
	fmt.Fprintf(out, `Copy the files to the path "streams/v1" in your cloud's public bucket.`)
	fmt.Fprintln(out, "")
	return nil
}
