// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"strconv"
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api/client/application"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
)

var helpSummary = `
Removes an existing relation between two applications.`[1:]

var helpDetails = `
An existing relation between the two specified applications will be removed.
This should not result in either of the applications entering an error state,
but may result in either or both of the applications being unable to continue
normal operation. In the case that there is more than one relation between
two applications it is necessary to specify which is to be removed (see
examples). Relations will automatically be removed when using the`[1:] + "`juju\nremove-application`" + ` command.

The relation is specified using the relation endpoint names, e.g., ` + "`mysql wordpress`" + `, or
` + "`mediawiki:db mariadb:db`" + `.

It is also possible to specify the relation ID, if known. This is useful to
terminate a relation originating from a different model, where only the ID is known.

Sometimes, the removal of the relation may fail as Juju encounters errors
and failures that need to be dealt with before a relation can be removed.
However, at times, there is a need to remove a relation ignoring
all operational errors. In these rare cases, use the ` + "`--force`" + ` option but note
that ` + "`--force`" + ` will remove a relation without giving it the opportunity to be removed cleanly.

`

const helpExamples = `
    juju remove-relation mysql wordpress
    juju remove-relation 4
    juju remove-relation 4 --force

In the case of multiple relations, the endpoint name should be specified
at least once. For example, the commands below will all have the same effect:

    juju remove-relation mediawiki:db mariadb:db
    juju remove-relation mediawiki mariadb:db
    juju remove-relation mediawiki:db mariadb
`

// NewRemoveRelationCommand returns a command to remove a relation between 2 applications.
func NewRemoveRelationCommand() cmd.Command {
	command := &removeRelationCommand{}
	command.newAPIFunc = func() (ApplicationDestroyRelationAPI, error) {
		root, err := command.NewAPIRoot()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return application.NewClient(root), nil

	}
	return modelcmd.Wrap(command)
}

// removeRelationCommand causes an existing application relation to be shut down.
type removeRelationCommand struct {
	modelcmd.ModelCommandBase
	RelationId int
	Endpoints  []string
	newAPIFunc func() (ApplicationDestroyRelationAPI, error)
	Force      bool
	NoWait     bool
	fs         *gnuflag.FlagSet
}

func (c *removeRelationCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "remove-relation",
		Args:     "<application1>[:<relation name1>] <application2>[:<relation name2>] | <relation-id>",
		Purpose:  helpSummary,
		Doc:      helpDetails,
		Examples: helpExamples,
		SeeAlso: []string{
			"integrate",
			"remove-application",
		},
	})
}

func (c *removeRelationCommand) Init(args []string) (err error) {
	if len(args) == 1 {
		if c.RelationId, err = strconv.Atoi(args[0]); err != nil || c.RelationId < 0 {
			return errors.NotValidf("relation ID %q", args[0])
		}
		return nil
	}
	if len(args) != 2 {
		return errors.Errorf("a relation must involve two applications")
	}
	c.Endpoints = args
	return nil
}

func (c *removeRelationCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.BoolVar(&c.Force, "force", false, "Force remove a relation")
	c.fs = f
}

// ApplicationDestroyRelationAPI defines the API methods that application remove relation command uses.
type ApplicationDestroyRelationAPI interface {
	Close() error
	DestroyRelation(force *bool, maxWait *time.Duration, endpoints ...string) error
	DestroyRelationId(relationId int, force *bool, maxWait *time.Duration) error
}

func (c *removeRelationCommand) Run(_ *cmd.Context) error {
	noWaitSet := false
	forceSet := false
	c.fs.Visit(func(flag *gnuflag.Flag) {
		if flag.Name == "no-wait" {
			noWaitSet = true
		} else if flag.Name == "force" {
			forceSet = true
		}
	})
	if !forceSet && noWaitSet {
		return errors.NotValidf("--no-wait without --force")
	}
	var maxWait *time.Duration
	var force *bool
	if c.Force {
		force = &c.Force
		if c.NoWait {
			zeroSec := 0 * time.Second
			maxWait = &zeroSec
		}
	}

	client, err := c.newAPIFunc()
	if err != nil {
		return err
	}
	defer client.Close()
	if len(c.Endpoints) > 0 {
		err = client.DestroyRelation(force, maxWait, c.Endpoints...)
	} else {
		err = client.DestroyRelationId(c.RelationId, force, maxWait)
	}
	return block.ProcessBlockedError(err, block.BlockRemove)
}
