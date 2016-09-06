// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package listagreements

import (
	"encoding/json"
	"io"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/terms-client/api"
	"github.com/juju/terms-client/api/wireformat"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
)

var (
	newClient = func(client *httpbakery.Client) (TermsServiceClient, error) {
		return api.NewClient(api.HTTPClient(client))
	}
)

// TermsServiceClient defines methods needed for the Terms Service CLI
// commands.
type TermsServiceClient interface {
	GetUsersAgreements() ([]wireformat.AgreementResponse, error)
}

const listAgreementsDoc = `
List terms the user has agreed to.
`

// NewListAgreementsCommand returns a new command that can be
// used to list agreements a user has made.
func NewListAgreementsCommand() *listAgreementsCommand {
	return &listAgreementsCommand{}
}

type term struct {
	name     string
	revision int
}

var _ cmd.Command = (*listAgreementsCommand)(nil)

// listAgreementsCommand creates a user agreement to the specified
// Terms and Conditions document.
type listAgreementsCommand struct {
	modelcmd.JujuCommandBase
	out cmd.Output
}

// SetFlags implements Command.SetFlags.
func (c *listAgreementsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.JujuCommandBase.SetFlags(f)
	c.out.AddFlags(f, "json", map[string]cmd.Formatter{
		"json": formatJSON,
		"yaml": cmd.FormatYaml,
	})
}

// Info implements Command.Info.
func (c *listAgreementsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "agreements",
		Purpose: "List user's agreements.",
		Doc:     listAgreementsDoc,
		Aliases: []string{"list-agreements"},
	}
}

// Run implements Command.Run.
func (c *listAgreementsCommand) Run(ctx *cmd.Context) error {
	client, err := c.BakeryClient()
	if err != nil {
		return errors.Annotate(err, "failed to create an http client")
	}

	apiClient, err := newClient(client)
	if err != nil {
		return errors.Annotate(err, "failed to create a terms API client")
	}

	agreements, err := apiClient.GetUsersAgreements()
	if err != nil {
		return errors.Annotate(err, "failed to list user agreements")
	}
	if agreements == nil {
		agreements = []wireformat.AgreementResponse{}
	}
	err = c.out.Write(ctx, agreements)
	if err != nil {
		return errors.Mask(err)
	}
	return nil
}

func formatJSON(writer io.Writer, value interface{}) error {
	bytes, err := json.MarshalIndent(value, "", "    ")
	if err != nil {
		return err
	}
	bytes = append(bytes, '\n')
	_, err = writer.Write(bytes)
	return err
}
