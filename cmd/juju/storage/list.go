// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"fmt"
	"io"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
)

// NewListCommand returns a command for listing storage instances.
func NewListCommand() cmd.Command {
	cmd := &listCommand{}
	cmd.newAPIFunc = func() (StorageListAPI, error) {
		return cmd.NewStorageAPI()
	}
	return modelcmd.Wrap(cmd)
}

const listCommandDoc = `
List information about storage.
`

// listCommand returns storage instances.
type listCommand struct {
	StorageCommandBase
	out        cmd.Output
	ids        []string
	filesystem bool
	volume     bool
	newAPIFunc func() (StorageListAPI, error)
}

// Info implements Command.Info.
func (c *listCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "storage",
		Args:    "<filesystem|volume> ...",
		Purpose: "Lists storage details.",
		Doc:     listCommandDoc,
		Aliases: []string{"list-storage"},
	}
}

// SetFlags implements Command.SetFlags.
func (c *listCommand) SetFlags(f *gnuflag.FlagSet) {
	c.StorageCommandBase.SetFlags(f)
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": formatListTabular,
	})
	// TODO(axw) deprecate these flags, and introduce separate commands
	// for listing just filesystems or volumes.
	f.BoolVar(&c.filesystem, "filesystem", false, "List filesystem storage")
	f.BoolVar(&c.volume, "volume", false, "List volume storage")
}

// Init implements Command.Init.
func (c *listCommand) Init(args []string) (err error) {
	if c.filesystem && c.volume {
		return errors.New("--filesystem and --volume can not be used together")
	}
	if len(args) > 0 && !c.filesystem && !c.volume {
		return errors.New("specifying IDs only supported with --filesystem and --volume flags")
	}
	c.ids = args
	return nil
}

// Run implements Command.Run.
func (c *listCommand) Run(ctx *cmd.Context) (err error) {
	api, err := c.newAPIFunc()
	if err != nil {
		return err
	}
	defer api.Close()

	params := GetCombinedStorageInfoParams{
		Context: ctx, APIClient: api, Ids: c.ids,
	}
	switch {
	case c.filesystem:
		params.WantFilesystems = true
	case c.volume:
		params.WantVolumes = true
	default:
		params.WantStorage = true
		params.WantVolumes = true
		params.WantFilesystems = true
	}

	combined, err := GetCombinedStorageInfo(params)
	if err != nil {
		return err
	}
	if combined.empty() {
		if c.out.Name() == "tabular" {
			ctx.Infof("No storage to display.")
		}
		return nil
	}
	return c.out.Write(ctx, combined)
}

// GetCombinedStorageInfoParams holds parameters for the GetCombinedStorageInfo call.
type GetCombinedStorageInfoParams struct {
	Context                                   *cmd.Context
	APIClient                                 StorageListAPI
	Ids                                       []string
	WantStorage, WantVolumes, WantFilesystems bool
}

// GetCombinedStorageInfo returns a list of StorageInstances, Filesystems and Volumes for juju cmdline display purposes
func GetCombinedStorageInfo(p GetCombinedStorageInfoParams) (*CombinedStorage, error) {
	combined := &CombinedStorage{}
	if p.WantFilesystems {
		filesystems, err := generateListFilesystemsOutput(p.Context, p.APIClient, p.Ids)
		if err != nil {
			return nil, errors.Trace(err)
		}
		combined.Filesystems = filesystems
	}
	if p.WantVolumes {
		volumes, err := generateListVolumeOutput(p.Context, p.APIClient, p.Ids)
		if err != nil {
			return nil, errors.Trace(err)
		}
		combined.Volumes = volumes
	}
	if p.WantStorage {
		storageInstances, err := generateListStorageOutput(p.Context, p.APIClient)
		if err != nil {
			return nil, errors.Trace(err)
		}
		combined.StorageInstances = storageInstances
	}
	return combined, nil
}

// StorageListAPI defines the API methods that the storage commands use.
type StorageListAPI interface {
	Close() error
	ListStorageDetails() ([]params.StorageDetails, error)
	ListFilesystems(machines []string) ([]params.FilesystemDetailsListResult, error)
	ListVolumes(machines []string) ([]params.VolumeDetailsListResult, error)
}

// generateListStorageOutput returns a map of storage details
func generateListStorageOutput(ctx *cmd.Context, api StorageListAPI) (map[string]StorageInfo, error) {
	results, err := api.ListStorageDetails()
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	return formatStorageDetails(results)
}

// CombinedStorage holds a list of StorageInstances, Filesystems and Volumes for juju cmdline display purposes.
type CombinedStorage struct {
	StorageInstances map[string]StorageInfo    `yaml:"storage,omitempty" json:"storage,omitempty"`
	Filesystems      map[string]FilesystemInfo `yaml:"filesystems,omitempty" json:"filesystems,omitempty"`
	Volumes          map[string]VolumeInfo     `yaml:"volumes,omitempty" json:"volumes,omitempty"`
}

func (c *CombinedStorage) empty() bool {
	return len(c.StorageInstances) == 0 && len(c.Filesystems) == 0 && len(c.Volumes) == 0
}

func formatListTabular(writer io.Writer, value interface{}) error {
	combined := value.(CombinedStorage)
	var newline bool
	if len(combined.StorageInstances) > 0 {
		// If we're listing storage in tabular format, we combine all
		// of the information into a list of "storage".
		if err := formatStorageListTabular(
			writer,
			combined.StorageInstances,
			combined.Filesystems,
			combined.Volumes,
		); err != nil {
			return err
		}
		return nil
	}
	if len(combined.Filesystems) > 0 {
		if newline {
			fmt.Fprintln(writer)
		}
		if err := formatFilesystemListTabular(writer, combined.Filesystems); err != nil {
			return err
		}
		newline = true
	}
	if len(combined.Volumes) > 0 {
		if newline {
			fmt.Fprintln(writer)
		}
		if err := formatVolumeListTabular(writer, combined.Volumes); err != nil {
			return err
		}
	}
	return nil
}
