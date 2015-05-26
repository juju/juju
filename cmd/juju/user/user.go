// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"
	"github.com/juju/utils/readpass"

	"github.com/juju/juju/cmd/envcmd"
)

var logger = loggo.GetLogger("juju.cmd.juju.user")

const userCommandDoc = `
"juju user" is used to manage the user accounts and access control in
the Juju environment.
`

const userCommandPurpose = "manage user accounts and access control"

// NewSuperCommand creates the user supercommand and registers the subcommands
// that it supports.
func NewSuperCommand() cmd.Command {
	usercmd := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:        "user",
		Doc:         userCommandDoc,
		UsagePrefix: "juju",
		Purpose:     userCommandPurpose,
	})
	usercmd.Register(envcmd.WrapSystem(&AddCommand{}))
	usercmd.Register(envcmd.WrapSystem(&ChangePasswordCommand{}))
	usercmd.Register(envcmd.WrapSystem(&InfoCommand{}))
	usercmd.Register(envcmd.WrapSystem(&DisableCommand{}))
	usercmd.Register(envcmd.WrapSystem(&EnableCommand{}))
	usercmd.Register(envcmd.WrapSystem(&ListCommand{}))
	return usercmd
}

// UserCommandBase is a helper base structure that has a method to get the
// user manager client.
type UserCommandBase struct {
	envcmd.SysCommandBase
}

var readPassword = readpass.ReadPassword

func (*UserCommandBase) generateOrReadPassword(ctx *cmd.Context, generate bool) (string, error) {
	if generate {
		password, err := utils.RandomPassword()
		if err != nil {
			return "", errors.Annotate(err, "failed to generate random password")
		}
		return password, nil
	}

	fmt.Fprintln(ctx.Stdout, "password:")
	password, err := readPassword()
	if err != nil {
		return "", errors.Trace(err)
	}
	fmt.Fprintln(ctx.Stdout, "type password again:")
	verify, err := readPassword()
	if err != nil {
		return "", errors.Trace(err)
	}
	if password != verify {
		return "", errors.New("Passwords do not match")
	}
	return password, nil
}
