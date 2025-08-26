// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"fmt"
	"io"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/rpc/params"
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

const listCommandExample = `
List all storage:

    juju storage

List only filesystem storage:

    juju storage --filesystem

List only volume storage:

    juju storage --volume
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
	return jujucmd.Info(&cmd.Info{
		Name:     "storage",
		Args:     "<filesystem|volume> ...",
		Purpose:  "Lists storage details.",
		Doc:      listCommandDoc,
		Aliases:  []string{"list-storage"},
		Examples: listCommandExample,
		SeeAlso: []string{
			"show-storage",
			"add-storage",
			"remove-storage",
		},
	})
}

// SetFlags implements Command.SetFlags.
func (c *listCommand) SetFlags(f *gnuflag.FlagSet) {
	c.StorageCommandBase.SetFlags(f)
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": formatListTabularOne,
	})
	// TODO(axw) deprecate these flags, and introduce separate commands
	// for listing just filesystems or volumes.
	f.BoolVar(&c.filesystem, "filesystem", false, "List filesystem storage (deprecated)")
	f.BoolVar(&c.volume, "volume", false, "List volume storage (deprecated)")
}

// Init implements Command.Init.
func (c *listCommand) Init(args []string) (err error) {
	if c.filesystem && c.volume {
		return errors.New("--filesystem and --volume can not be used together")
	}
	if len(args) > 0 && !c.filesystem && !c.volume {
		return errors.New("specifying IDs only supported with --filesystem and --volume options")
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
	if combined.Empty() {
		if c.out.Name() == "tabular" {
			ctx.Infof("No storage to display.")
		}
		return nil
	}
	return c.out.Write(ctx, *combined)
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

// CombinedStorageFromParams is called with a response from FullStatus.
// TODO: move storage handling to a common package.
func CombinedStorageFromParams(
	storage []params.StorageDetails,
	filesystems []params.FilesystemDetails,
	volumes []params.VolumeDetails,
) (*CombinedStorage, error) {
	var err error
	cs := &CombinedStorage{}
	cs.StorageInstances, err = formatStorageDetails(storage)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cs.Filesystems, err = convertToFilesystemInfo(filesystems)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cs.Volumes, err = convertToVolumeInfo(volumes)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return cs, nil
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
		return nil, errors.Trace(err)
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

// Empty checks if CombinedStorage is empty.
func (c *CombinedStorage) Empty() bool {
	return len(c.StorageInstances) == 0 && len(c.Filesystems) == 0 && len(c.Volumes) == 0
}

// formatListTabularOne writes a tabular summary of storage instances or filesystems or volumes.
func formatListTabularOne(writer io.Writer, value interface{}) error {
	return formatListTabular(writer, value, false)
}

func formatListTabular(writer io.Writer, value interface{}, all bool) error {
	combined := value.(CombinedStorage)
	var newline bool
	if len(combined.StorageInstances) > 0 {
		// If we're listing storage in tabular format, we combine all
		// of the information into a list of "storage".
		if err := formatStorageInstancesListTabular(writer, combined); err != nil {
			return errors.Trace(err)
		}
		if !all {
			return nil
		}
		newline = true
	}
	if len(combined.Filesystems) > 0 {
		if newline {
			fmt.Fprintln(writer)
		}
		if err := formatFilesystemListTabular(writer, combined.Filesystems); err != nil {
			return err
		}
		if !all {
			return nil
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

// FormatListTabularAll writes a tabular summary of storage instances, filesystems and volumes.
func FormatListTabularAll(writer io.Writer, value interface{}) error {
	return formatListTabular(writer, value, true)
}
