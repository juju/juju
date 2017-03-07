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
		Args:    "<machineID> ...",
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

	var wantStorage, wantVolumes, wantFilesystems bool
	switch {
	case c.filesystem:
		wantFilesystems = true
	case c.volume:
		wantVolumes = true
	default:
		wantStorage = true
		wantVolumes = true
		wantFilesystems = true
	}

	var combined combinedStorage
	if wantFilesystems {
		filesystems, err := generateListFilesystemsOutput(ctx, api, c.ids)
		if err != nil {
			return err
		}
		combined.Filesystems = filesystems
	}
	if wantVolumes {
		volumes, err := generateListVolumeOutput(ctx, api, c.ids)
		if err != nil {
			return err
		}
		combined.Volumes = volumes
	}
	if wantStorage {
		storageInstances, err := generateListStorageOutput(ctx, api)
		if err != nil {
			return err
		}
		combined.StorageInstances = storageInstances
	}
	if combined.empty() {
		if c.out.Name() == "tabular" {
			ctx.Infof("No storage to display.")
		}
		return nil
	}
	return c.out.Write(ctx, combined)
}

// StorageAPI defines the API methods that the storage commands use.
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

type combinedStorage struct {
	StorageInstances map[string]StorageInfo    `yaml:"storage,omitempty" json:"storage,omitempty"`
	Filesystems      map[string]FilesystemInfo `yaml:"filesystems,omitempty" json:"filesystems,omitempty"`
	Volumes          map[string]VolumeInfo     `yaml:"volumes,omitempty" json:"volumes,omitempty"`
}

func (c *combinedStorage) empty() bool {
	return len(c.StorageInstances) == 0 && len(c.Filesystems) == 0 && len(c.Volumes) == 0
}

func formatListTabular(writer io.Writer, value interface{}) error {
	combined := value.(combinedStorage)
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
