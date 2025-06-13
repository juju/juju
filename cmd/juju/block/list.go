// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package block

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/controller/controller"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/output"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/rpc/params"
)

// NewListCommand returns the command that lists the disabled
// commands for the model.
func NewListCommand() cmd.Command {
	return modelcmd.Wrap(&listCommand{
		apiFunc: func(ctx context.Context, c newAPIRoot) (blockListAPI, error) {
			return getBlockAPI(ctx, c)
		},
		controllerAPIFunc: func(ctx context.Context, c newControllerAPIRoot) (controllerListAPI, error) {
			return getControllerAPI(ctx, c)
		},
	})
}

const listCommandDoc = `
List disabled commands for the model.
` + commandSets

// listCommand list blocks.
type listCommand struct {
	modelcmd.ModelCommandBase
	apiFunc           func(context.Context, newAPIRoot) (blockListAPI, error)
	controllerAPIFunc func(context.Context, newControllerAPIRoot) (controllerListAPI, error)
	all               bool
	out               cmd.Output
}

// Init implements Command.Init.
func (c *listCommand) Init(args []string) (err error) {
	return cmd.CheckEmpty(args)
}

// Info implements Command.Info.
func (c *listCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "disabled-commands",
		Purpose: "List disabled commands.",
		Doc:     listCommandDoc,
		Aliases: []string{"list-disabled-commands"},
		SeeAlso: []string{
			"disable-command",
			"enable-command",
		},
	})
}

// SetFlags implements Command.SetFlags.
func (c *listCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.BoolVar(&c.all, "all", false, "Lists for all models (administrative users only)")
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": c.formatter,
	})
}

// Run implements Command.Run.
func (c *listCommand) Run(ctx *cmd.Context) (err error) {
	if c.all {
		return c.listForController(ctx)
	}
	return c.listForModel(ctx)
}

const noBlocks = "No commands are currently disabled."

func (c *listCommand) listForModel(ctx *cmd.Context) (err error) {
	api, err := c.apiFunc(ctx, c)
	if err != nil {
		return errors.Trace(err)
	}
	defer api.Close()

	result, err := api.List(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	if len(result) == 0 && c.out.Name() == "tabular" {
		ctx.Infof(noBlocks)
		return nil
	}
	return c.out.Write(ctx, formatBlockInfo(result))
}

func (c *listCommand) listForController(ctx *cmd.Context) (err error) {
	api, err := c.controllerAPIFunc(ctx, c)
	if err != nil {
		return errors.Trace(err)
	}
	defer api.Close()

	result, err := api.ListBlockedModels(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	if len(result) == 0 && c.out.Name() == "tabular" {
		ctx.Infof(noBlocks)
		return nil
	}
	info, err := FormatModelBlockInfo(result)
	if err != nil {
		return errors.Trace(err)
	}
	return c.out.Write(ctx, info)
}

func (c *listCommand) formatter(writer io.Writer, value interface{}) error {
	if c.all {
		return FormatTabularBlockedModels(writer, value)
	}
	return formatBlocks(writer, value)
}

// blockListAPI defines the client API methods that block list command uses.
type blockListAPI interface {
	Close() error
	List(ctx context.Context) ([]params.Block, error)
}

// controllerListAPI defines the methods on the controller API endpoint
// that the blocks command calls.
type controllerListAPI interface {
	Close() error
	ListBlockedModels(context.Context) ([]params.ModelBlockInfo, error)
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
		fmt.Fprintf(writer, "No commands are currently disabled.")
		return nil
	}

	tw := output.TabWriter(writer)
	w := output.Wrapper{TabWriter: tw}
	w.Println("Disabled commands", "Message")
	for _, info := range blocks {
		w.Println(info.Commands, info.Message)
	}
	tw.Flush()

	return nil
}

type newControllerAPIRoot interface {
	NewControllerAPIRoot(ctx context.Context) (api.Connection, error)
}

// getControllerAPI returns a block api for block manipulation.
func getControllerAPI(ctx context.Context, c newControllerAPIRoot) (*controller.Client, error) {
	root, err := c.NewControllerAPIRoot(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return controller.NewClient(root), nil
}

type modelBlockInfo struct {
	Name        string   `yaml:"name" json:"name"`
	UUID        string   `yaml:"model-uuid" json:"model-uuid"`
	CommandSets []string `yaml:"disabled-commands,omitempty" json:"disabled-commands,omitempty"`
}

func FormatModelBlockInfo(all []params.ModelBlockInfo) ([]modelBlockInfo, error) {
	output := make([]modelBlockInfo, len(all))
	for i, one := range all {
		output[i] = modelBlockInfo{
			Name:        jujuclient.QualifyModelName(one.Qualifier, one.Name),
			UUID:        one.UUID,
			CommandSets: blocksToStr(one.Blocks),
		}
	}
	return output, nil
}

// FormatTabularBlockedModels writes out tabular format for blocked models.
// This method is exported as it is also used by destroy-model.
func FormatTabularBlockedModels(writer io.Writer, value interface{}) error {
	models, ok := value.([]modelBlockInfo)
	if !ok {
		return errors.Errorf("expected value of type %T, got %T", models, value)
	}

	tw := output.TabWriter(writer)
	w := output.Wrapper{TabWriter: tw}
	w.Println("Name", "Model UUID", "Disabled commands")
	for _, model := range models {
		w.Println(model.Name, model.UUID, strings.Join(model.CommandSets, ", "))
	}
	tw.Flush()
	return nil
}

func blocksToStr(blocks []string) []string {
	result := make([]string, len(blocks))
	for i, val := range blocks {
		result[i] = operationFromType(val)
	}
	sort.Strings(result)
	return result
}
