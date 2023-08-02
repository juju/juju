// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/rpc/params"
)

func newListImagesCommand() cmd.Command {
	return modelcmd.Wrap(&listImagesCommand{})
}

const listCommandDoc = `
List information about image metadata stored in Juju model.
This list can be filtered using various filters as described below.

More than one filter can be specified. Result will contain metadata that matches 
all filters in combination.

If no filters are supplied, all stored image metadata will be listed.

Filtering a list of images for a set of bases can be done via --bases. A base can 
be  specified using the OS name and the version of the OS, separated by @. For 
example, --bases ubuntu@22.04.
`

// listImagesCommand returns stored image metadata.
type listImagesCommand struct {
	cloudImageMetadataCommandBase

	out cmd.Output

	Stream          string
	Region          string
	Series          []string
	Bases           []string
	Arches          []string
	VirtType        string
	RootStorageType string
}

// Init implements Command.Init.
func (c *listImagesCommand) Init(args []string) (err error) {
	if len(c.Bases) > 0 && len(c.Series) > 0 {
		return errors.New("--series and --bases cannot be specified together")
	}

	if len(c.Arches) > 0 {
		result := []string{}
		for _, one := range c.Arches {
			result = append(result, strings.Split(one, ",")...)
		}
		c.Arches = result
	}
	return nil
}

// Info implements Command.Info.
func (c *listImagesCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "images",
		Purpose: "lists cloud image metadata used when choosing an image to start",
		Doc:     listCommandDoc,
		Aliases: []string{"list-images"},
	})
}

// SetFlags implements Command.SetFlags.
func (c *listImagesCommand) SetFlags(f *gnuflag.FlagSet) {
	c.cloudImageMetadataCommandBase.SetFlags(f)

	f.StringVar(&c.Stream, "stream", "", "image metadata stream")
	f.StringVar(&c.Region, "region", "", "image metadata cloud region")

	f.Var(cmd.NewAppendStringsValue(&c.Series), "series", "only show cloud image metadata for these series. DEPRECATED use --bases")
	f.Var(cmd.NewAppendStringsValue(&c.Bases), "bases", "only show cloud image metadata for these bases")
	f.Var(cmd.NewAppendStringsValue(&c.Arches), "arch", "only show cloud image metadata for these architectures")

	f.StringVar(&c.VirtType, "virt-type", "", "image metadata virtualisation type")
	f.StringVar(&c.RootStorageType, "storage-type", "", "image metadata root storage type")

	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": formatMetadataListTabular,
	})
}

// Run implements Command.Run.
func (c *listImagesCommand) Run(ctx *cmd.Context) (err error) {
	var bases []corebase.Base
	// Note: we validated that both series and bases cannot be specified in
	// Init(), so it's safe to assume that only one of them is set here.
	if len(c.Series) > 0 {
		ctx.Warningf("series flag is deprecated, use --bases instead")
		for _, s := range c.Series {
			for _, one := range strings.Split(s, ",") {
				b, err := corebase.GetBaseFromSeries(one)
				if err != nil {
					return errors.Annotatef(err, "attempting to convert %q to a base", c.Series)
				}
				bases = append(bases, b)
			}
		}
		c.Series = nil
	}
	if len(c.Bases) > 0 {
		for _, b := range c.Bases {
			for _, one := range strings.Split(b, ",") {
				b, err := corebase.ParseBaseFromString(one)
				if err != nil {
					return errors.Trace(err)
				}
				bases = append(bases, b)
			}
		}
	}

	api, err := getImageMetadataListAPI(c)
	if err != nil {
		return err
	}
	defer api.Close()

	found, err := c.List(api, bases)
	if err != nil {
		return err
	}
	if len(found) == 0 {
		return nil
	}

	info, errs := convertDetailsToInfo(found)
	if len(errs) > 0 {
		// display individual error
		fmt.Fprintf(ctx.Stderr, strings.Join(errs, "\n"))
	}

	var output interface{}
	switch c.out.Name() {
	case "yaml", "json":
		output = groupMetadata(info)
	default:
		{
			sort.Sort(metadataInfos(info))
			output = info
		}
	}
	return c.out.Write(ctx, output)
}

func (c *listImagesCommand) List(api MetadataListAPI, bases []corebase.Base) ([]params.CloudImageMetadata, error) {
	return api.List(c.Stream, c.Region, bases, c.Arches, c.VirtType, c.RootStorageType)
}

var getImageMetadataListAPI = (*listImagesCommand).getImageMetadataListAPI

// MetadataListAPI defines the API methods that list image metadata command uses.
type MetadataListAPI interface {
	Close() error
	List(stream, region string, series []corebase.Base, arches []string, virtType, rootStorageType string) ([]params.CloudImageMetadata, error)
}

func (c *listImagesCommand) getImageMetadataListAPI() (MetadataListAPI, error) {
	return c.NewImageMetadataAPI()
}

// convertDetailsToInfo converts cloud image metadata received from api to
// structure native to CLI.
// We also return a list of errors for versions we could not convert to series for user friendly read.
func convertDetailsToInfo(details []params.CloudImageMetadata) ([]MetadataInfo, []string) {
	if len(details) == 0 {
		return nil, nil
	}

	info := make([]MetadataInfo, len(details))
	errs := []string{}
	for i, one := range details {
		info[i] = MetadataInfo{
			Source:          one.Source,
			Version:         one.Version,
			Arch:            one.Arch,
			Region:          one.Region,
			ImageId:         one.ImageId,
			Stream:          one.Stream,
			VirtType:        one.VirtType,
			RootStorageType: one.RootStorageType,
		}
	}
	return info, errs
}

// metadataInfos is a convenience type enabling to sort
// a collection of MetadataInfo
type metadataInfos []MetadataInfo

// Implements sort.Interface
func (m metadataInfos) Len() int {
	return len(m)
}

// Implements sort.Interface and sort image metadata
// by source, series, arch and region.
// All properties are sorted in alphabetical order
// except for series which is reversed -
// latest series are at the beginning of the collection.
func (m metadataInfos) Less(i, j int) bool {
	if m[i].Source != m[j].Source {
		// Alphabetical order here is incidentally does what we want:
		// we want "custom" metadata to precede
		// "public" metadata.
		// This may need to b revisited if more metadata sources will be discovered.
		return m[i].Source < m[j].Source
	}
	if m[i].Version != m[j].Version {
		// reverse order
		return m[i].Version > m[j].Version
	}
	if m[i].Arch != m[j].Arch {
		// alphabetical order
		return m[i].Arch < m[j].Arch
	}
	// alphabetical order
	return m[i].Region < m[j].Region
}

// Implements sort.Interface
func (m metadataInfos) Swap(i, j int) {
	m[i], m[j] = m[j], m[i]
}

type minMetadataInfo struct {
	ImageId         string `yaml:"image-id" json:"image-id"`
	Stream          string `yaml:"stream" json:"stream"`
	VirtType        string `yaml:"virt-type,omitempty" json:"virt-type,omitempty"`
	RootStorageType string `yaml:"storage-type,omitempty" json:"storage-type,omitempty"`
}

// groupMetadata constructs map representation of metadata
// grouping individual items by source, version, arch and region
// to be served to Yaml and JSON output for readability.
func groupMetadata(metadata []MetadataInfo) map[string]map[string]map[string]map[string][]minMetadataInfo {
	result := map[string]map[string]map[string]map[string][]minMetadataInfo{}

	for _, m := range metadata {
		sourceMap, ok := result[m.Source]
		if !ok {
			sourceMap = map[string]map[string]map[string][]minMetadataInfo{}
			result[m.Source] = sourceMap
		}

		versionMap, ok := sourceMap[m.Version]
		if !ok {
			versionMap = map[string]map[string][]minMetadataInfo{}
			sourceMap[m.Version] = versionMap
		}

		archMap, ok := versionMap[m.Arch]
		if !ok {
			archMap = map[string][]minMetadataInfo{}
			versionMap[m.Arch] = archMap
		}

		archMap[m.Region] = append(archMap[m.Region], minMetadataInfo{m.ImageId, m.Stream, m.VirtType, m.RootStorageType})
	}

	return result
}
