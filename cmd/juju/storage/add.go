// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"fmt"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/storage"
)

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
Environment default values will be used for all ommitted constraint values.
There is no need to comma-separate ommitted constraints. 

Example:
    Add 3 ebs storage instances for "data" storage to unit u/0:     

      juju storage add u/0 data=ebs,1024,3 
    or
      juju storage add u/0 data=ebs,3
    or
      juju storage add u/0 data=ebs,,3 
    
    
    Add 1 storage instances for "data" storage to unit u/0 
    using default environment provider pool: 

      juju storage add u/0 data=1 
    or
      juju storage add u/0 data 
`
	addCommandAgs = `
<unit name> <storage directive> ...
    where storage directive is 
        <charm storage name>=<storage constraints> 
    or
        <charm storage name>
`
)

// AddCommand adds unit storage instances dynamically.
type AddCommand struct {
	StorageCommandBase
	unitTag string

	// storageCons is a map of storage constraints, keyed on the storage name
	// defined in charm storage metadata.
	storageCons map[string]storage.Constraints
}

// Init implements Command.Init.
func (c *AddCommand) Init(args []string) (err error) {
	if len(args) < 2 {
		return errors.New("storage add requires a unit and a storage directive")
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
func (c *AddCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "add",
		Purpose: "adds unit storage dynamically",
		Doc:     addCommandDoc,
		Args:    addCommandAgs,
	}
}

// Run implements Command.Run.
func (c *AddCommand) Run(ctx *cmd.Context) (err error) {
	api, err := getStorageAddAPI(c)
	if err != nil {
		return err
	}
	defer api.Close()

	storages := c.createStorageAddParams()
	results, err := api.AddToUnit(storages)
	if err != nil {
		return err
	}
	// If there are any failures, display them first.
	// Then display all added storage.
	// If there are no failures, then there is no need to display all successes.
	var added []string

	for i, one := range results {
		us := storages[i]
		if one.Error != nil {
			fmt.Fprintf(ctx.Stderr, fail+": %v\n", us.StorageName, one.Error)
			continue
		}
		added = append(added, fmt.Sprintf(success, us.StorageName))
	}
	if len(added) < len(storages) {
		fmt.Fprintf(ctx.Stderr, strings.Join(added, "\n"))
	}
	return nil
}

var (
	getStorageAddAPI = (*AddCommand).getStorageAddAPI
	storageName      = "storage %q"
	success          = "success: " + storageName
	fail             = "fail: " + storageName
)

// StorageAddAPI defines the API methods that the storage commands use.
type StorageAddAPI interface {
	Close() error
	AddToUnit(storages []params.StorageAddParams) ([]params.ErrorResult, error)
}

func (c *AddCommand) getStorageAddAPI() (StorageAddAPI, error) {
	return c.NewStorageAPI()
}

func (c *AddCommand) createStorageAddParams() []params.StorageAddParams {
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
	return all
}
