// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package listagreements

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/gosuri/uitable"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/terms-client/api"
	"github.com/juju/terms-client/api/wireformat"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	"github.com/juju/juju/cmd/modelcmd"
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
Charms may require a user to accept its terms in order for it to be deployed.
In other words, some applications may only be installed if a user agrees to 
accept some terms defined by the charm. 

This command lists the terms that the user has agreed to.

See also:
    agree

`

// NewListAgreementsCommand returns a new command that can be
// used to list agreements a user has made.
func NewListAgreementsCommand() modelcmd.ControllerCommand {
	return modelcmd.WrapController(&listAgreementsCommand{})
}

type term struct {
	name     string
	revision int
}

var _ cmd.Command = (*listAgreementsCommand)(nil)

// listAgreementsCommand creates a user agreement to the specified
// Terms and Conditions document.
type listAgreementsCommand struct {
	modelcmd.ControllerCommandBase
	out cmd.Output
}

// SetFlags implements Command.SetFlags.
func (c *listAgreementsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"tabular": formatTabular,
		"json":    formatJSON,
		"yaml":    cmd.FormatYaml,
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
	if len(agreements) == 0 {
		ctx.Infof("No agreements to display.")
		return nil
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

func formatTabular(writer io.Writer, value interface{}) error {
	agreements, ok := value.([]wireformat.AgreementResponse)
	if !ok {
		return errors.Errorf("expected value of type %T, got %T", agreements, value)
	}
	table := uitable.New()
	table.MaxColWidth = 50
	table.Wrap = true
	for _, col := range []int{1, 2, 3, 4} {
		table.RightAlign(col)
	}
	table.AddRow("Term", "Agreed on")
	for _, agreement := range agreements {
		if agreement.Owner != "" {
			table.AddRow(fmt.Sprintf("%s/%s/%d", agreement.Owner, agreement.Term, agreement.Revision), agreement.CreatedOn)
		} else {
			table.AddRow(fmt.Sprintf("%s/%d", agreement.Term, agreement.Revision), agreement.CreatedOn)
		}
	}

	_, err := fmt.Fprint(writer, table)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}
