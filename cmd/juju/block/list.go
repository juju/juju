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

func newListCommand() cmd.Command {
	return modelcmd.Wrap(&listCommand{})
}

const listCommandDoc = `
List blocks for Juju model.
This command shows if each block type is enabled. 
For enabled blocks, block message is shown if it was specified.
`

// listCommand list blocks.
type listCommand struct {
	modelcmd.ModelCommandBase
	out cmd.Output
}

// Init implements Command.Init.
func (c *listCommand) Init(args []string) (err error) {
	return nil
}

// Info implements Command.Info.
func (c *listCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "list",
		Purpose: "List Juju blocks.",
		Doc:     listCommandDoc,
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
	Operation string  `yaml:"block" json:"block"`
	Enabled   bool    `yaml:"enabled" json:"enabled"`
	Message   *string `yaml:"message,omitempty" json:"message,omitempty"`
}

// formatBlockInfo takes a set of Block and creates a
// mapping to information structures.
func formatBlockInfo(all []params.Block) []BlockInfo {
	output := make([]BlockInfo, len(blockArgs))

	info := make(map[string]BlockInfo, len(all))
	// not all block types may be returned from client
	for _, one := range all {
		op := OperationFromType(one.Type)
		bi := BlockInfo{
			Operation: op,
			// If client returned it, it means that it is enabled
			Enabled: true,
			Message: &one.Message,
		}
		info[op] = bi
	}

	for i, aType := range blockArgs {
		if val, ok := info[aType]; ok {
			output[i] = val
			continue
		}
		output[i] = BlockInfo{Operation: aType}
	}

	return output
}

// formatBlocks writes block list representation.
func formatBlocks(writer io.Writer, value interface{}) error {
	blocks, ok := value.([]BlockInfo)
	if !ok {
		return errors.Errorf("expected value of type %T, got %T", blocks, value)
	}
	// To format things as desired.
	tw := output.TabWriter(writer)

	for _, ablock := range blocks {
		fmt.Fprintln(tw)
		switched := "off"
		if ablock.Enabled {
			switched = "on"
		}
		fmt.Fprintf(tw, "%v\t", ablock.Operation)
		if ablock.Message != nil {
			fmt.Fprintf(tw, "=%v, %v", switched, *ablock.Message)
			continue
		}
		fmt.Fprintf(tw, "=%v", switched)
	}

	tw.Flush()

	return nil
}
