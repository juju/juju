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

// Command is the top-level command wrapping all storage functionality.
type Command struct {
	cmd.SuperCommand
}

// NewSuperCommand creates the storage supercommand and
// registers the subcommands that it supports.
func NewSuperCommand() cmd.Command {
	storagecmd := Command{
		SuperCommand: *cmd.NewSuperCommand(
			cmd.SuperCommandParams{
				Name:        "storage",
				Doc:         storageCmdDoc,
				UsagePrefix: "juju",
				Purpose:     storageCmdPurpose,
			})}
	storagecmd.Register(envcmd.Wrap(&ShowCommand{}))
	storagecmd.Register(envcmd.Wrap(&ListCommand{}))
	return &storagecmd
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
	UnitId      string `yaml:"unit_id,omitempty" json:"unit_id,omitempty"`
	Attached    string `yaml:"attached_status,omitempty" json:"attached_status,omitempty"`
	Location    string `yaml:"location,omitempty" json:"location,omitempty"`
	Provisioned string `yaml:"provisioned_status,omitempty" json:"provisioned_status,omitempty"`
}

// formatStorageInfo takes a set of StorageInstances and creates a
// mapping from storage instance ID to information structures.
func formatStorageInfo(storages []params.StorageDetails) (map[string]map[string]StorageInfo, error) {
	if len(storages) == 0 {
		return nil, nil
	}
	output := make(map[string]map[string]StorageInfo)
	isAttached := func(inspect params.StorageAttachedStatus) bool {
		return inspect == params.StorageAttachedStatusAttached
	}
	for _, one := range storages {
		storageTag, err := names.ParseStorageTag(one.StorageTag)
		if err != nil {
			return nil, errors.Annotate(err, "invalid storage tag")
		}
		ownerTag, err := names.ParseTag(one.OwnerTag)
		if err != nil {
			return nil, errors.Annotate(err, "invalid owner tag")
		}
		storageName, err := names.StorageName(storageTag.Id())
		if err != nil {
			panic(err) // impossible
		}
		si := StorageInfo{
			StorageName: storageName,
			Kind:        one.Kind.String(),
			Attached:    one.Attached.String(),
			Location:    one.Location,
			Provisioned: one.Provisioned.String(),
		}
		if isAttached(one.Attached) {
			unitTag, err := names.ParseTag(one.UnitTag)
			if err != nil {
				return nil, errors.Annotate(err, "invalid unit tag")
			}
			si.UnitId = unitTag.Id()
		}
		owner := ownerTag.Id()
		ownerColl, ok := output[owner]
		if !ok {
			ownerColl = map[string]StorageInfo{}
			output[owner] = ownerColl
		}
		ownerColl[storageTag.Id()] = si
	}
	return output, nil
}
