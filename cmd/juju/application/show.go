// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/application"
	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/constraints"
)

const showApplicationDoc = `
The command takes deployed application names or aliases as an argument.

The command does an exact search. It does not support wildcards.

Examples:
    $ juju show-application mysql
    $ juju show-application mysql wordpress

    $ juju show-application myapplication
        where "myapplication" is the application name alias, see "juju help deploy" for more information

`

// NewShowApplicationCommand returns a command that displays applications info.
func NewShowApplicationCommand() cmd.Command {
	s := &showApplicationCommand{}
	s.newAPIFunc = func() (ApplicationsInfoAPI, error) {
		return s.newApplicationAPI()
	}
	return modelcmd.Wrap(s)
}

// showApplicationCommand displays application information.
type showApplicationCommand struct {
	modelcmd.ModelCommandBase

	out        cmd.Output
	apps       []string
	newAPIFunc func() (ApplicationsInfoAPI, error)
}

// Info implements Command.Info.
func (c *showApplicationCommand) Info() *cmd.Info {
	showCmd := &cmd.Info{
		Name:    "show-application",
		Args:    "<application name or alias>",
		Purpose: "Displays information about an application.",
		Doc:     showApplicationDoc,
	}
	return jujucmd.Info(showCmd)
}

// Init implements Command.Init.
func (c *showApplicationCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.Errorf("an application name must be supplied")
	}
	c.apps = args
	var invalid []string
	for _, one := range c.apps {
		if !names.IsValidApplication(one) {
			invalid = append(invalid, one)
		}
	}
	if len(invalid) == 0 {
		return nil
	}
	plural := "s"
	if len(invalid) == 1 {
		plural = ""
	}
	return errors.NotValidf(`application name%v %v`, plural, strings.Join(invalid, `, `))
}

// SetFlags implements Command.SetFlags.
func (c *showApplicationCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", cmd.DefaultFormatters.Formatters())
}

// ApplicationsInfoAPI defines the API methods that show-application command uses.
type ApplicationsInfoAPI interface {
	Close() error
	BestAPIVersion() int
	ApplicationsInfo([]names.ApplicationTag) ([]params.ApplicationInfoResult, error)
}

func (c *showApplicationCommand) newApplicationAPI() (ApplicationsInfoAPI, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return application.NewClient(root), nil
}

func (c *showApplicationCommand) Run(ctx *cmd.Context) error {
	client, err := c.newAPIFunc()
	if err != nil {
		return err
	}
	defer client.Close()

	if v := client.BestAPIVersion(); v < 9 {
		// old client does not support showing applications.
		return errors.NotSupportedf("show applications on API server version %v", v)
	}

	tags, err := c.getApplicationTags()
	if err != nil {
		return err
	}

	results, err := client.ApplicationsInfo(tags)
	if err != nil {
		return errors.Trace(err)
	}

	var errs params.ErrorResults
	var valid []params.ApplicationResult
	for _, result := range results {
		if result.Error != nil {
			errs.Results = append(errs.Results, params.ErrorResult{result.Error})
			continue
		}
		valid = append(valid, *result.Result)
	}
	if len(errs.Results) > 0 {
		return errs.Combine()
	}

	output, err := formatApplicationInfos(valid)
	if err != nil {
		return err
	}
	return c.out.Write(ctx, output)
}

func (c *showApplicationCommand) getApplicationTags() ([]names.ApplicationTag, error) {
	tags := make([]names.ApplicationTag, len(c.apps))
	for i, one := range c.apps {
		if !names.IsValidApplication(one) {
			return nil, errors.Errorf("invalid application name %v", one)
		}
		tags[i] = names.NewApplicationTag(one)
	}
	return tags, nil
}

// formatApplicationInfos takes a set of params.ApplicationInfo and
// creates a mapping from storage ID application name to application info.
func formatApplicationInfos(all []params.ApplicationResult) (map[string]ApplicationInfo, error) {
	if len(all) == 0 {
		return nil, nil
	}
	output := make(map[string]ApplicationInfo)
	for _, one := range all {
		tag, info, err := createApplicationInfo(one)
		if err != nil {
			return nil, errors.Trace(err)
		}
		output[tag.Name] = info
	}
	return output, nil
}

// ApplicationInfo defines the serialization behaviour of the application information.
type ApplicationInfo struct {
	Charm            string            `yaml:"charm,omitempty" json:"charm,omitempty"`
	Series           string            `yaml:"series,omitempty" json:"series,omitempty"`
	Channel          string            `yaml:"channel,omitempty" json:"channel,omitempty"`
	Constraints      constraints.Value `yaml:"constraints,omitempty" json:"constraints,omitempty"`
	Principal        bool              `yaml:"principal" json:"principal"`
	Exposed          bool              `yaml:"exposed" json:"exposed"`
	Remote           bool              `yaml:"remote" json:"remote"`
	EndpointBindings map[string]string `yaml:"endpoint-bindings,omitempty" json:"endpoint-bindings,omitempty"`
}

func createApplicationInfo(details params.ApplicationResult) (names.ApplicationTag, ApplicationInfo, error) {
	tag, err := names.ParseApplicationTag(details.Tag)
	if err != nil {
		return names.ApplicationTag{}, ApplicationInfo{}, errors.Trace(err)
	}

	info := ApplicationInfo{
		Charm:            details.Charm,
		Series:           details.Series,
		Channel:          details.Channel,
		Constraints:      details.Constraints,
		Principal:        details.Principal,
		Exposed:          details.Exposed,
		Remote:           details.Remote,
		EndpointBindings: details.EndpointBindings,
	}
	return tag, info, nil
}
