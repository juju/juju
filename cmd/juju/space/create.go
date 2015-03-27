// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space

import (
	"encoding/binary"
	"fmt"
	"net"
	"regexp"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
)

// CreateCommand calls the API to create a new network space.
type CreateCommand struct {
	SpaceCommandBase
	Name  string
	CIDRs []string
}

const createEnvHelpDoc = `
Creates a new network space with a given name, optionally including one or more
subnets specified with their CIDR values.

A network space name can consist of
`

func (c *CreateCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "create",
		Args:    "<name> [<CIDR1> <CIDR2> ...]",
		Purpose: "create network space",
		Doc:     strings.TrimSpace(createEnvHelpDoc),
	}
}

// Init checks the arguments for sanity and sets up the command to run
func (c *CreateCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("No space named in command")
	}

	if NameIsValid(args[0]) {
		c.Name = args[0]
	} else {
		return errors.New(fmt.Sprintf("Space name %q is invalid", args[0]))
	}

	networks := make(map[uint64]bool)
	for _, arg := range args[1:] {
		if _, ipNet, err := net.ParseCIDR(arg); err == nil {
			// We have a valid CIDR, now check that it is unique
			subnet, bytesRead := binary.Uvarint(ipNet.IP)
			if bytesRead == 0 {
				return errors.New("Error converting subnet to uint64.")
			}
			if _, ok := networks[subnet]; ok == true {
				// subnet already exists in this space.
				return errors.New(fmt.Sprintf("Duplicate subnet in space %v", arg))
			} else {
				networks[subnet] = true
			}
			c.CIDRs = append(c.CIDRs, arg)
		} else {
			return errors.New(fmt.Sprintf("%q is not a valid CIDR", arg))
		}
	}

	return nil
}

// Run implements Command.Run.
func (c *CreateCommand) Run(ctx *cmd.Context) (err error) {
	api, err := c.NewSpaceAPIClient()
	if err != nil {
		return err
	}
	defer api.Close()
	return nil
}

// NameIsValid checks that the name given for a space contains only valid characters
func NameIsValid(name string) bool {
	r := regexp.MustCompile("^[-a-z0-9]+$")
	return r.MatchString(name)
}
