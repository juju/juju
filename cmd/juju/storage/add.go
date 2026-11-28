// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/storage"
	"github.com/juju/juju/rpc/params"
)

// NewAddCommand returns a command used to add unit storage.
func NewAddCommand() cmd.Command {
	cmd := &addCommand{}
	cmd.newAPIFunc = func(ctx context.Context) (StorageAddAPI, error) {
		return cmd.NewStorageAPI(ctx)
	}
	return modelcmd.Wrap(cmd)
}

const (
	addCommandDoc = `
Request additional instances of a storage name declared by the unit's charm
revision. The command reports the new storage IDs, not a completed attachment.
Inspect each ID with show-storage before using the storage.

Pass one or more directives as separate arguments. A bare storage name requests
one instance using the unit's current source and size. To override the request,
use name=pool,count,size without spaces. Components may be omitted or reordered;
use a positive integer for count and a size suffix such as G for GiB.
Existing and requested instances together must fit the charm's count range.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/storage/
`

	addCommandExamples = `
Request an additional 32 GiB EBS device if the Ceph OSD revision on ceph-osd/0
declares osd-devices and has room for one more instance:

    juju add-storage ceph-osd/0 osd-devices=ebs,32G,1

Request one additional OSD device for ceph-osd/0 using its current source and size:

    juju add-storage ceph-osd/0 osd-devices
`

	addCommandAgs = `<unit> <storage-directive> [<storage-directive> ...]`
)

// addCommand adds unit storage instances dynamically.
type addCommand struct {
	StorageCommandBase
	modelcmd.IAASOnlyCommand
	unitTag names.UnitTag

	// storageDirectives is a map of storage directives, keyed on the storage name
	// defined in charm storage metadata.
	storageDirectives map[string]storage.Directive
	newAPIFunc        func(ctx context.Context) (StorageAddAPI, error)
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

	c.storageDirectives, err = storage.ParseDirectivesMap(args[1:], false)
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
	api, err := c.newAPIFunc(ctx)
	if err != nil {
		return err
	}
	defer api.Close()

	storages := c.createStorageAddParams()
	results, err := api.AddToUnit(ctx, storages)
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
	AddToUnit(ctx context.Context, storages []params.StorageAddParams) ([]params.AddStorageResult, error)
}

func (c *addCommand) createStorageAddParams() []params.StorageAddParams {
	all := make([]params.StorageAddParams, 0, len(c.storageDirectives))
	for one, directive := range c.storageDirectives {
		d := directive
		all = append(all, params.StorageAddParams{
			UnitTag:     c.unitTag.String(),
			StorageName: one,
			Directives: params.StorageDirectives{
				Pool:    d.Pool,
				SizeMiB: &d.Size,
				Count:   &d.Count,
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
