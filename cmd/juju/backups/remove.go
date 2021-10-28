// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"fmt"

	"github.com/juju/cmd/v3"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/params"
)

const removeDoc = `
remove-backup removes a backup from remote storage.
`

// NewRemoveCommand returns a command used to remove a
// backup from remote storage.
func NewRemoveCommand() cmd.Command {
	return modelcmd.Wrap(&removeCommand{})
}

type removeCommand struct {
	CommandBase
	// ID refers to the backup to be removed.
	ID         string
	KeepLatest bool
}

// Info implements Command.Info.
func (c *removeCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "remove-backup",
		Args:    "[--keep-latest|<ID>]",
		Purpose: "Remove the specified backup from remote storage.",
		Doc:     removeDoc,
	})
}

// SetFlags implements Command.SetFlags.
func (c *removeCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	f.BoolVar(&c.KeepLatest, "keep-latest", false,
		"Remove all backups on remote storage except for the latest.")
}

// Init implements Command.Init.
func (c *removeCommand) Init(args []string) error {
	switch {
	case len(args) == 0 && !c.KeepLatest:
		return errors.New("missing ID or --keep-latest option")
	case len(args) != 0:
		id, args := args[0], args[1:]
		if err := cmd.CheckEmpty(args); err != nil {
			return errors.Trace(err)
		}
		c.ID = id
	case c.KeepLatest:
	default:
		return errors.New("unknown error parsing arguments")
	}
	return nil
}

// Run implements Command.Run.
func (c *removeCommand) Run(ctx *cmd.Context) error {
	if err := c.validateIaasController(c.Info().Name); err != nil {
		return errors.Trace(err)
	}

	client, apiVersion, err := c.NewGetAPI()
	if err != nil {
		return errors.Trace(err)
	}
	defer client.Close()

	if apiVersion < 2 && c.KeepLatest {
		return errors.New("--keep-latest is not supported by this controller")
	}

	ids := []string{}
	var keep string
	if c.KeepLatest {
		list, err := client.List()
		if err != nil {
			return errors.Trace(err)
		}

		ids, keep, err = parseList(list.List)
		switch {
		case err != nil:
			return errors.Trace(err)
		case len(ids) > 0:
			break
		case keep != "":
			ctx.Warningf("no backups to remove, %v most current", keep)
			return nil
		default:
			ctx.Warningf("no backups to remove")
			return nil
		}
	} else {
		ids = append(ids, c.ID)
	}
	//ctx.Infof("%s; %+v", keep, ids)

	results, err := client.Remove(ids...)
	if err != nil {
		return errors.Trace(err)
	}

	for i, err := range results {
		if err.Error != nil {
			// Some errors do not provide enough info, let's try to fix that here.
			err.Error.Message = fmt.Sprintf("failed to remove %v: %s", ids[i], err.Error.Message)
			continue
		}
		ctx.Infof("successfully removed: %v\n", ids[i])
	}
	if c.KeepLatest {
		ctx.Infof("kept: %v", keep)
	}
	return errors.Trace(params.ErrorResults{results}.Combine())
}

// parseList returns a list of IDs to be removed and the one ID to be kept.
// Keep the latest ID based on Started.
func parseList(list []params.BackupsMetadataResult) ([]string, string, error) {
	if len(list) == 0 {
		return nil, "", nil
	}
	latest := list[0]
	retList := set.NewStrings()
	// Start looking for a new latest with the 2nd item in the slice.
	for _, entry := range list[1:] {
		if entry.Started.After(latest.Started) {
			// Found a new latest, add the old one to the set
			retList.Add(latest.ID)
			latest = entry
			continue
		}
		// Not the latest, add to the set
		retList.Add(entry.ID)
	}
	return retList.SortedValues(), latest.ID, nil
}
