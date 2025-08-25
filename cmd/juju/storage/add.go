// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"fmt"
	"sort"
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/storage"
)

// NewAddCommand returns a command used to add unit storage.
func NewAddCommand() cmd.Command {
	cmd := &addCommand{}
	cmd.newAPIFunc = func() (StorageAddAPI, error) {
		return cmd.NewStorageAPI()
	}
	return modelcmd.Wrap(cmd)
}

const (
	addCommandDoc = `
Add storage to a pre-existing unit within a model. Storage is allocated from
a storage pool, using parameters provided within a "storage directive". (Use
` + "`juju deploy --storage=<storage-directive>` " + `to provision storage during the
deployment process.)

	juju add-storage <unit> <storage-directive>

` + "`<unit>` " + `is the ID of a unit that is already in the model.

` + "`<storage-directive>` " + `describes to the charm how to refer to the storage,
and where to provision it from. ` + "`<storage-directive>` " + `takes the following form:

    <storage-name>[=<storage-constraint>]

` + "`<storage-name>` " + `is defined in the charm's ` + "`metadata.yaml` " + `file.

` + "`<storage-constraint>` " + `is a description of how Juju should provision storage
instances for the unit. They are made up of up to three parts: ` + "`<storage-pool>`" + `,
` + "`<count>`" + `, and ` + "`<size>`" + `. They can be provided in any order, but we recommend the
following:

    <storage-pool>,<count>,<size>

Each parameter is optional, so long as at least one is present. So the following
storage constraints are also valid:

    <storage-pool>,<size>
    <count>,<size>
    <size>

` + "`<storage-pool>` " + `is the storage pool to provision storage instances from. Must
be a name from ` + "`juju storage-pools`" + `.  The default pool is available via
executing ` + "`juju model-config storage-default-block-source`" + `.

` + "`<count>` " + `is the number of storage instances to provision from ` + "`<storage-pool>` " + `of
` + "`<size>`" + `. Must be a positive integer. The default count is "1". May be restricted
by the charm, which can specify a maximum number of storage instances per unit.

` + "`<size>` " + `is the number of bytes to provision per storage instance. Must be a
positive number, followed by a size suffix.  Valid suffixes include M, G, T,
and P.  Defaults to "1024M", or the which can specify a minimum size required
by the charm.
`

	addCommandExamples = `
Add a 100MiB tmpfs storage instance for "pgdata" storage to unit postgresql/0:

    juju add-storage postgresql/0 pgdata=tmpfs,100M

Add 10 1TiB storage instances to "osd-devices" storage to unit ceph-osd/0 from the model's default storage pool:

    juju add-storage ceph-osd/0 osd-devices=1T,10

Add a storage instance from the (AWS-specific) ebs-ssd storage pool for "brick" storage to unit gluster/0:

    juju add-storage gluster/0 brick=ebs-ssd


Further reading:

https://juju.is/docs/storage
`

	addCommandAgs = `<unit> <storage-directive>`
)

// addCommand adds unit storage instances dynamically.
type addCommand struct {
	StorageCommandBase
	modelcmd.IAASOnlyCommand
	unitTag names.UnitTag

	// storageCons is a map of storage constraints, keyed on the storage name
	// defined in charm storage metadata.
	storageCons map[string]storage.Constraints
	newAPIFunc  func() (StorageAddAPI, error)
}

// Init implements Command.Init.
func (c *addCommand) Init(args []string) (err error) {
	if len(args) < 2 {
		return errors.New("add-storage requires a unit and a storage directive")
	}

	u := args[0]
	if !names.IsValidUnit(u) {
		return errors.NotValidf("unit name %q", u)
	}
	c.unitTag = names.NewUnitTag(u)

	c.storageCons, err = storage.ParseConstraintsMap(args[1:], false)
	return
}

// Info implements Command.Info.
func (c *addCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "add-storage",
		Purpose:  "Adds storage to a unit after it has been deployed.",
		Doc:      addCommandDoc,
		Args:     addCommandAgs,
		Examples: addCommandExamples,
		SeeAlso: []string{
			"import-filesystem",
			"storage",
			"storage-pools",
		},
	})
}

// Run implements Command.Run.
func (c *addCommand) Run(ctx *cmd.Context) (err error) {
	api, err := c.newAPIFunc()
	if err != nil {
		return err
	}
	defer api.Close()

	storages := c.createStorageAddParams()
	results, err := api.AddToUnit(storages)
	if err != nil {
		if params.IsCodeUnauthorized(err) {
			common.PermissionsMessage(ctx.Stderr, "add storage")
		}
		return err
	}

	var failures []string
	// If there was a unit-related error, then all storages will get the same error.
	// We want to collapse these - no need to repeat the same things ad nauseam.
	collapsedFailures := set.NewStrings()
	for i, one := range results {
		us := storages[i]
		if one.Error != nil {
			const fail = "failed to add storage %q to %s: %v"
			failures = append(failures, fmt.Sprintf(fail, us.StorageName, c.unitTag.Id(), one.Error))
			collapsedFailures.Add(one.Error.Error())
			continue
		}
		if one.Result == nil {
			// Old controllers don't inform us of tag names.
			ctx.Infof("added storage %q to %s", us.StorageName, c.unitTag.Id())
			continue
		}
		for _, tagString := range one.Result.StorageTags {
			tag, err := names.ParseStorageTag(tagString)
			if err != nil {
				return errors.Trace(err)
			}
			ctx.Infof("added storage %s to %s", tag.Id(), c.unitTag.Id())
		}
	}

	if len(failures) == len(storages) {
		// If we managed to collapse, then display these instead of the whole list.
		if len(collapsedFailures) < len(failures) {
			for _, one := range collapsedFailures.SortedValues() {
				fmt.Fprintln(ctx.Stderr, one)
			}
			return cmd.ErrSilent
		}
	}
	if len(failures) > 0 {
		fmt.Fprintln(ctx.Stderr, strings.Join(failures, "\n"))
		return cmd.ErrSilent
	}
	return nil
}

// StorageAddAPI defines the API methods that the storage commands use.
type StorageAddAPI interface {
	Close() error
	AddToUnit(storages []params.StorageAddParams) ([]params.AddStorageResult, error)
}

func (c *addCommand) createStorageAddParams() []params.StorageAddParams {
	all := make([]params.StorageAddParams, 0, len(c.storageCons))
	for one, sc := range c.storageCons {
		cons := sc
		all = append(all, params.StorageAddParams{
			UnitTag:     c.unitTag.String(),
			StorageName: one,
			Constraints: params.StorageConstraints{
				Pool:  cons.Pool,
				Size:  &cons.Size,
				Count: &cons.Count,
			},
		})
	}

	// For consistency and because we are coming from a map,
	// ensure that collection is sorted by storage name for deterministic results.
	sort.Sort(storageParams(all))
	return all
}

type storageParams []params.StorageAddParams

func (v storageParams) Len() int {
	return len(v)
}

func (v storageParams) Swap(i, j int) {
	v[i], v[j] = v[j], v[i]
}

func (v storageParams) Less(i, j int) bool {
	return v[i].StorageName < v[j].StorageName
}
