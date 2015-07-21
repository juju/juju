// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/api/storage"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
)

var logger = loggo.GetLogger("juju.cmd.juju.storage")

const storageCmdDoc = `
"juju storage" is used to manage storage instances in
 the Juju environment.
`

const storageCmdPurpose = "manage storage instances"

// NewSuperCommand creates the storage supercommand and
// registers the subcommands that it supports.
func NewSuperCommand() cmd.Command {
	storagecmd := cmd.NewSuperCommand(
		cmd.SuperCommandParams{
			Name:        "storage",
			Doc:         storageCmdDoc,
			UsagePrefix: "juju",
			Purpose:     storageCmdPurpose,
		})
	storagecmd.Register(envcmd.Wrap(&ShowCommand{}))
	storagecmd.Register(envcmd.Wrap(&ListCommand{}))
	storagecmd.Register(envcmd.Wrap(&AddCommand{}))
	storagecmd.Register(NewPoolSuperCommand())
	storagecmd.Register(NewVolumeSuperCommand())
	return storagecmd
}

// StorageCommandBase is a helper base structure that has a method to get the
// storage managing client.
type StorageCommandBase struct {
	envcmd.EnvCommandBase
}

// NewStorageAPI returns a storage api for the root api endpoint
// that the environment command returns.
func (c *StorageCommandBase) NewStorageAPI() (*storage.Client, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, err
	}
	return storage.NewClient(root), nil
}

// StorageInfo defines the serialization behaviour of the storage information.
type StorageInfo struct {
	StorageName string `yaml:"storage" json:"storage"`
	Kind        string `yaml:"kind" json:"kind"`
	Status      string `yaml:"status,omitempty" json:"status,omitempty"`
	Persistent  bool   `yaml:"persistent" json:"persistent"`
	Location    string `yaml:"location,omitempty" json:"location,omitempty"`
}

// formatStorageDetails takes a set of StorageDetail and creates a
// mapping keyed on unit and storage id.
func formatStorageDetails(storages []params.StorageDetails) (map[string]map[string]StorageInfo, error) {
	if len(storages) == 0 {
		return nil, nil
	}
	output := make(map[string]map[string]StorageInfo)
	for _, one := range storages {
		storageTag, err := names.ParseStorageTag(one.StorageTag)
		if err != nil {
			return nil, errors.Annotate(err, "invalid storage tag")
		}
		unitTag, err := names.ParseTag(one.UnitTag)
		if err != nil {
			return nil, errors.Annotate(err, "invalid unit tag")
		}

		storageName, err := names.StorageName(storageTag.Id())
		if err != nil {
			panic(err) // impossible
		}
		si := StorageInfo{
			StorageName: storageName,
			Kind:        one.Kind.String(),
			Status:      one.Status,
			Location:    one.Location,
			Persistent:  one.Persistent,
		}
		unit := unitTag.Id()
		unitColl, ok := output[unit]
		if !ok {
			unitColl = map[string]StorageInfo{}
			output[unit] = unitColl
		}
		unitColl[storageTag.Id()] = si
	}
	return output, nil
}
