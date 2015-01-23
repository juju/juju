// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/apiserver/params"
)

const ShowCommandDoc = `
Show the details of a single storage.
The storage can be specified by a (unit, storage-name) pair.

* note use of positional arguments

options:
-e, --environment (= "")
   juju environment to operate in
-o, --output (= "")
   specify an output
--unit
   unit name
--storage
   storage name
--format
   output format, yaml or json. Default yaml.
`

// ShowCommand attempts to release storage instance.
type ShowCommand struct {
	StorageCommandBase
	UnitName    string
	StorageName string
	out         cmd.Output
}

// Init implements Command.Init.
func (c *ShowCommand) Init(args []string) (err error) {
	if c.UnitName == "" || c.StorageName == "" {
		return errors.New("both unit and storage names are required")
	}
	return nil
}

// Info implements Command.Info.
func (c *ShowCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "show",
		Purpose: "shows storage instance",
		Doc:     ShowCommandDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *ShowCommand) SetFlags(f *gnuflag.FlagSet) {
	c.StorageCommandBase.SetFlags(f)
	f.StringVar(&c.UnitName, "unit", "", "unit name for storage info")
	f.StringVar(&c.StorageName, "storage", "", "storage name for storage info")
	c.out.AddFlags(f, "yaml", cmd.DefaultFormatters)
}

// StorageInfo defines the serialization behaviour of the storage information.
type StorageInfo struct {
	UnitName      string   `yaml:"unit-name" json:"unit-name"`
	StorageName   string   `yaml:"storage-name" json:"storage-name"`
	AvailableSize int      `yaml:"available-size" json:"available-size"`
	TotalSize     int      `yaml:"total-size" json:"total-size"`
	Tags          []string `yaml:"tags,omitempty" json:"tags,omitempty"`
}

// Run implements Command.Run.
func (c *ShowCommand) Run(ctx *cmd.Context) (err error) {
	api, err := getStorageShowAPI(c)
	if err != nil {
		return err
	}
	defer api.Close()

	result, err := api.Show(c.UnitName, c.StorageName)
	if err != nil {
		return err
	}
	output := c.apiStoragesToInstanceSlice(result)
	if len(output) != 1 {
		return errors.Errorf("expected 1 result, got %d", len(output))
	}

	return c.out.Write(ctx, output[0])
}

var (
	getStorageShowAPI = (*ShowCommand).getStorageShowAPI
)

// StorageAPI defines the API methods that the storage commands use.
type StorageShowAPI interface {
	Close() error
	Show(unitName, storageName string) ([]params.StorageInstance, error)
}

func (c *ShowCommand) getStorageShowAPI() (StorageShowAPI, error) {
	return c.NewStorageAPI()
}

func (c *ShowCommand) apiStoragesToInstanceSlice(all []params.StorageInstance) []StorageInfo {
	var output []StorageInfo
	for _, one := range all {
		outInfo := StorageInfo{
			UnitName:      one.UnitName,
			StorageName:   one.StorageName,
			AvailableSize: one.AvailableSize,
			TotalSize:     one.TotalSize,
			Tags:          one.Tags,
		}
		output = append(output, outInfo)
	}
	return output
}
