// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The sla package contains the implementation of the juju sla
// command.
package sla

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/gosuri/uitable"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	"github.com/juju/romulus/api/sla"
	slawire "github.com/juju/romulus/wireformat/sla"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/modelconfig"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

var logger = loggo.GetLogger("romulus.cmd.sla")

// authorizationClient defines the interface of an api client that
// the command uses to create an sla authorization macaroon.
type authorizationClient interface {
	// Authorize returns the sla authorization macaroon for the specified model,
	Authorize(modelUUID, supportLevel, wallet string) (*slawire.SLAResponse, error)
}

type slaClient interface {
	SetSLALevel(level, owner string, creds []byte) error
	SLALevel() (string, error)
}

type slaLevel struct {
	Model string `json:"model" yaml:"model"`
	SLA   string `json:"sla" yaml:"sla"`
}

var newSLAClient = func(conn api.Connection) slaClient {
	return modelconfig.NewClient(conn)
}

var newAuthorizationClient = func(options ...sla.ClientOption) (authorizationClient, error) {
	return sla.NewClient(options...)
}

var modelId = func(conn api.Connection) string {
	// Our connection is model based so ignore the returned bool.
	tag, _ := conn.ModelTag()
	return tag.Id()
}

var newJujuClientStore = jujuclient.NewFileClientStore

// NewSLACommand returns a new command that is used to set SLA credentials for a
// deployed application.
func NewSLACommand() cmd.Command {
	slaCommand := &slaCommand{
		newSLAClient:           newSLAClient,
		newAuthorizationClient: newAuthorizationClient,
	}
	slaCommand.newAPIRoot = slaCommand.NewAPIRoot
	return modelcmd.Wrap(slaCommand)
}

// slaCommand is a command-line tool for setting
// Model.SLACredential for development & demonstration purposes.
type slaCommand struct {
	modelcmd.ModelCommandBase
	out cmd.Output

	newAPIRoot             func() (api.Connection, error)
	newSLAClient           func(api.Connection) slaClient
	newAuthorizationClient func(options ...sla.ClientOption) (authorizationClient, error)

	Level  string
	Budget string
}

// SetFlags sets additional flags for the support command.
func (c *slaCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"tabular": formatTabular,
		"json":    cmd.FormatJson,
		"yaml":    cmd.FormatYaml,
	})
	f.StringVar(&c.Budget, "budget", "", "the maximum spend for the model")
}

// Info implements cmd.Command.
func (c *slaCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "sla",
		Args:    "<level>",
		Purpose: "Set the SLA level for a model.",
		Doc: `
Set the support level for the model, effective immediately.
Examples:
    juju sla essential              # set the support level to essential
    juju sla standard --budget 1000 # set the support level to essential with a maximum budget of $1000
    juju sla                        # display the current support level for the model.
`,
	}
}

// Init implements cmd.Command.
func (c *slaCommand) Init(args []string) error {
	if len(args) < 1 {
		return nil
	}
	c.Level = args[0]
	return c.ModelCommandBase.Init(args[1:])
}

func (c *slaCommand) requestSupportCredentials(modelUUID string) (string, string, []byte, error) {
	hc, err := c.BakeryClient()
	if err != nil {
		return "", "", nil, errors.Trace(err)
	}
	authClient, err := c.newAuthorizationClient(sla.HTTPClient(hc))
	if err != nil {
		return "", "", nil, errors.Trace(err)
	}
	slaResp, err := authClient.Authorize(modelUUID, c.Level, c.Budget)
	if err != nil {
		err = common.MaybeTermsAgreementError(err)
		if termErr, ok := errors.Cause(err).(*common.TermsRequiredError); ok {
			return "", "", nil, errors.Trace(termErr.UserErr())
		}
		return "", "", nil, errors.Trace(err)
	}
	ms := macaroon.Slice{slaResp.Credentials}
	mbuf, err := json.Marshal(ms)
	if err != nil {
		return "", "", nil, errors.Trace(err)
	}
	return slaResp.Owner, slaResp.Message, mbuf, nil
}

func (c *slaCommand) displayCurrentLevel(client slaClient, modelID string, ctx *cmd.Context) error {
	modelNameMap := modelNameMap()
	modelName := modelID
	if name, ok := modelNameMap[modelID]; ok {
		modelName = name
	}
	level, err := client.SLALevel()
	if err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(c.out.Write(ctx, &slaLevel{
		Model: modelName,
		SLA:   level,
	}))
}

// Run implements cmd.Command.
func (c *slaCommand) Run(ctx *cmd.Context) error {
	root, err := c.newAPIRoot()
	if err != nil {
		return errors.Trace(err)
	}
	client := c.newSLAClient(root)
	modelId := modelId(root)

	if c.Level == "" {
		return c.displayCurrentLevel(client, modelId, ctx)
	}
	owner, message, credentials, err := c.requestSupportCredentials(modelId)
	if err != nil {
		return errors.Trace(err)
	}
	err = client.SetSLALevel(c.Level, owner, credentials)
	if err != nil {
		return errors.Trace(err)
	}
	if message != "" {
		fmt.Fprintln(ctx.Stdout, message)
	}
	return nil
}

func formatTabular(writer io.Writer, value interface{}) error {
	l, ok := value.(*slaLevel)
	if !ok {
		return errors.Errorf("expected value of type %T, got %T", l, value)
	}
	table := uitable.New()
	table.MaxColWidth = 50
	table.Wrap = true
	for _, col := range []int{2, 3, 5} {
		table.RightAlign(col)
	}
	table.AddRow("Model", "SLA")
	table.AddRow(l.Model, l.SLA)
	fmt.Fprint(writer, table)
	return nil
}

func modelNameMap() map[string]string {
	store := newJujuClientStore()
	uuidToName := map[string]string{}
	controllers, err := store.AllControllers()
	if err != nil {
		logger.Warningf("failed to read juju client controller names")
		return map[string]string{}
	}
	for cname := range controllers {
		models, err := store.AllModels(cname)
		if err != nil {
			logger.Warningf("failed to read juju client model names")
			return map[string]string{}
		}
		for mname, mdetails := range models {
			uuidToName[mdetails.ModelUUID] = cname + ":" + mname
		}
	}
	return uuidToName
}
