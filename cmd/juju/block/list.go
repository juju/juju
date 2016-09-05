// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package block

import (
	"fmt"
	"io"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
)

// NewListCommand returns the command that lists the disabled
// commands for the model.
func NewListCommand() cmd.Command {
	return modelcmd.Wrap(&listCommand{})
}

const listCommandDoc = `
List disabled commands for the model.
` + commandSets + `
See Also:
    juju disable-command
    juju enable-command
`

// listCommand list blocks.
type listCommand struct {
	modelcmd.ModelCommandBase
	out cmd.Output
}

// Init implements Command.Init.
func (c *listCommand) Init(args []string) (err error) {
	return cmd.CheckEmpty(args)
}

// Info implements Command.Info.
func (c *listCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "disabled-commands",
		Purpose: "List disabled commands.",
		Doc:     listCommandDoc,
		Aliases: []string{"list-disabled-commands"},
	}
}

// SetFlags implements Command.SetFlags.
func (c *listCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	c.out.AddFlags(f, "blocks", map[string]cmd.Formatter{
		"yaml":   cmd.FormatYaml,
		"json":   cmd.FormatJson,
		"blocks": formatBlocks,
	})
}

// Run implements Command.Run.
func (c *listCommand) Run(ctx *cmd.Context) (err error) {
	api, err := getBlockListAPI(&c.ModelCommandBase)
	if err != nil {
		return err
	}
	defer api.Close()

	result, err := api.List()
	if err != nil {
		return err
	}
	return c.out.Write(ctx, formatBlockInfo(result))
}

// BlockListAPI defines the client API methods that block list command uses.
type BlockListAPI interface {
	Close() error
	List() ([]params.Block, error)
}

var getBlockListAPI = func(cmd *modelcmd.ModelCommandBase) (BlockListAPI, error) {
	return getBlockAPI(cmd)
}

// BlockInfo defines the serialization behaviour of the block information.
type BlockInfo struct {
	Commands string `yaml:"command-set" json:"command-set"`
	Message  string `yaml:"message,omitempty" json:"message,omitempty"`
}

// formatBlockInfo takes a set of Block and creates a
// mapping to information structures.
func formatBlockInfo(all []params.Block) []BlockInfo {
	output := make([]BlockInfo, len(all))
	for i, one := range all {
		set, ok := toCmdValue[one.Type]
		if !ok {
			set = "<unknown>"
		}
		output[i] = BlockInfo{
			Commands: set,
			Message:  one.Message,
		}
	}
	return output
}

// formatBlocks writes block list representation.
func formatBlocks(writer io.Writer, value interface{}) error {
	blocks, ok := value.([]BlockInfo)
	if !ok {
		return errors.Errorf("expected value of type %T, got %T", blocks, value)
	}

	if len(blocks) == 0 {
		fmt.Fprintf(writer, "No commands are currently disabled.\n")
		return nil
	}

	tw := output.TabWriter(writer)
	fmt.Fprintln(tw, "COMMANDS\tMESSAGE")
	for _, info := range blocks {
		fmt.Fprintf(tw, "%s\t%s\n", info.Commands, info.Message)
	}
	tw.Flush()

	return nil
}
