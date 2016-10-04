// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"fmt"
	"sort"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
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
Add storage instances to a unit dynamically using provided storage directives.
Specify a unit and a storage specification in the same format 
as passed to juju deploy --storage=”...”.

A storage directive consists of a storage name as per charm specification
and storage constraints, e.g. pool, count, size.

The acceptable format for storage constraints is a comma separated
sequence of: POOL, COUNT, and SIZE, where

    POOL identifies the storage pool. POOL can be a string
    starting with a letter, followed by zero or more digits
    or letters optionally separated by hyphens.

    COUNT is a positive integer indicating how many instances
    of the storage to create. If unspecified, and SIZE is
    specified, COUNT defaults to 1.

    SIZE describes the minimum size of the storage instances to
    create. SIZE is a floating point number and multiplier from
    the set (M, G, T, P, E, Z, Y), which are all treated as
    powers of 1024.

Storage constraints can be optionally ommitted.
Model default values will be used for all ommitted constraint values.
There is no need to comma-separate ommitted constraints. 

Examples:
    # Add 3 ebs storage instances for "data" storage to unit u/0:

      juju add-storage u/0 data=ebs,1024,3 
    or
      juju add-storage u/0 data=ebs,3
    or
      juju add-storage u/0 data=ebs,,3 
    
    
    # Add 1 storage instances for "data" storage to unit u/0
    # using default model provider pool:

      juju add-storage u/0 data=1 
    or
      juju add-storage u/0 data 
`
	addCommandAgs = `
<unit name> <storage directive> ...
    where storage directive is 
        <charm storage name>=<storage constraints> 
    or
        <charm storage name>
`
)

// addCommand adds unit storage instances dynamically.
type addCommand struct {
	StorageCommandBase
	unitTag string

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
	c.unitTag = names.NewUnitTag(u).String()

	c.storageCons, err = storage.ParseConstraintsMap(args[1:], false)
	return
}

// Info implements Command.Info.
func (c *addCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "add-storage",
		Purpose: "Adds unit storage dynamically.",
		Doc:     addCommandDoc,
		Args:    addCommandAgs,
	}
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
			return common.PermissionsError(err, "add storage")
		}
		return err
	}

	var added []string
	var failures []string
	// If there was a unit-related error, then all storages will get the same error.
	// We want to collapse these - no need to repeat the same things ad nauseam.
	collapsedFailures := set.NewStrings()
	for i, one := range results {
		us := storages[i]
		if one.Error != nil {
			failures = append(failures, fmt.Sprintf(fail, us.StorageName, one.Error))
			collapsedFailures.Add(one.Error.Error())
			continue
		}
		added = append(added, fmt.Sprintf(success, us.StorageName))
	}

	if len(added) > 0 {
		fmt.Fprintln(ctx.Stdout, strings.Join(added, newline))
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
		fmt.Fprintln(ctx.Stderr, strings.Join(failures, newline))
		return cmd.ErrSilent
	}
	return nil
}

var (
	newline = "\n"
	success = "added %q"
	fail    = "failed to add %q: %v"
)

// StorageAddAPI defines the API methods that the storage commands use.
type StorageAddAPI interface {
	Close() error
	AddToUnit(storages []params.StorageAddParams) ([]params.ErrorResult, error)
}

func (c *addCommand) createStorageAddParams() []params.StorageAddParams {
	all := make([]params.StorageAddParams, 0, len(c.storageCons))
	for one, cons := range c.storageCons {
		all = append(all,
			params.StorageAddParams{
				UnitTag:     c.unitTag,
				StorageName: one,
				Constraints: params.StorageConstraints{
					cons.Pool,
					&cons.Size,
					&cons.Count,
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
