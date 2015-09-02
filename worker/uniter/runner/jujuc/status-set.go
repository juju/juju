// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"
	"launchpad.net/goyaml"

	"github.com/juju/juju/apiserver/params"
)

// StatusSetCommand implements the status-set command.
type StatusSetCommand struct {
	cmd.CommandBase
	ctx     Context
	status  string
	message string
	data    map[string]interface{}
	service bool
}

// NewStatusSetCommand makes a jujuc status-set command.
func NewStatusSetCommand(ctx Context) cmd.Command {
	return &StatusSetCommand{ctx: ctx}
}

func (c *StatusSetCommand) Info() *cmd.Info {
	doc := `
Sets the workload status of the charm. Message is optional.
The "last updated" attribute of the status is set, even if the
status and message are the same as what's already set.
`
	return &cmd.Info{
		Name:    "status-set",
		Args:    "<maintenance | blocked | waiting | active> [message] [data]",
		Purpose: "set status information",
		Doc:     doc,
	}
}

var validStatus = []params.Status{
	params.StatusMaintenance,
	params.StatusBlocked,
	params.StatusWaiting,
	params.StatusActive,
}

func (c *StatusSetCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.service, "service", false, "set this status for the service to which the unit belongs if the unit is the leader")
}

func cleanup(m map[interface{}]interface{}) (map[string]interface{}, error) {
	ret := make(map[string]interface{}, len(m))
	for k, v := range m {
		key, ok := k.(string)
		if !ok {
			return nil, errors.New("keys must be strings")
		}
		switch vt := v.(type) {
		case string, int64, int32, int:
			ret[key] = vt
		case map[interface{}]interface{}:
			cleanMap, err := cleanup(vt)
			if err != nil {
				return nil, errors.Annotate(err, "cannot process this data")
			}
			ret[key] = cleanMap
		default:
			return nil, errors.New("values can only be strings, ints or other maps")
		}
	}
	return ret, nil

}

func (c *StatusSetCommand) parseData(data string) error {
	var raw map[string]interface{}
	if err := goyaml.Unmarshal([]byte(data), &raw); err != nil {
		return errors.Annotate(err, "cannot parse the status data")
	}
	c.data = make(map[string]interface{}, len(raw))
	for k, v := range raw {
		if vt, ok := v.(map[interface{}]interface{}); ok {
			cleanMap, err := cleanup(vt)
			if err != nil {
				return errors.Annotate(err, "cannot process data")
			}
			c.data[k] = cleanMap
			continue
		}
		c.data[k] = v
	}
	return nil
}

func (c *StatusSetCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.Errorf("invalid args, require <status> [message] [data]")
	}
	valid := false
	for _, s := range validStatus {
		if string(s) == args[0] {
			valid = true
			break
		}
	}
	if !valid {
		return errors.Errorf("invalid status %q, expected one of %v", args[0], validStatus)
	}
	c.status = args[0]

	if len(args) > 1 {
		c.message = args[1]
	}
	if len(args) > 2 {
		if err := c.parseData(args[2]); err != nil {
			return errors.Annotate(err, "cannot parse data to set status")
		}
		return cmd.CheckEmpty(args[3:])
	}
	return nil
}

func (c *StatusSetCommand) Run(ctx *cmd.Context) error {
	statusInfo := StatusInfo{
		Status: c.status,
		Info:   c.message,
		Data:   c.data,
	}
	if c.service {
		return c.ctx.SetServiceStatus(statusInfo)
	}
	return c.ctx.SetUnitStatus(statusInfo)

}
