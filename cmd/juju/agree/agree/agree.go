// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agree

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/juju/charm/v12"
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/terms-client/v2/api"
	"github.com/juju/terms-client/v2/api/wireformat"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
)

var (
	clientNew = api.NewClient
)

const agreeDoc = `
Agree to the terms required by a charm.

When deploying a charm that requires agreement to terms, use ` + "`juju agree`" + ` to
view the terms and agree to them. Then the charm may be deployed.

Once you have agreed to terms, you will not be prompted to view them again.

`

const agreeExamples = `
Displays terms for somePlan revision 1 and prompts for agreement:

    juju agree somePlan/1

Displays the terms for revision 1 of somePlan, revision 2 of otherPlan, and prompts for agreement:

    juju agree somePlan/1 otherPlan/2

Agree to the terms without prompting:

    juju agree somePlan/1 otherPlan/2 --yes
`

// NewAgreeCommand returns a new command that can be
// used to create user agreements.
func NewAgreeCommand() modelcmd.ControllerCommand {
	return modelcmd.WrapController(&agreeCommand{})
}

type term struct {
	owner    string
	name     string
	revision int
}

// agreeCommand creates a user agreement to the specified terms.
type agreeCommand struct {
	modelcmd.ControllerCommandBase

	terms           []term
	termIds         []string
	SkipTermContent bool
}

// SetFlags implements Command.SetFlags.
func (c *agreeCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	f.BoolVar(&c.SkipTermContent, "yes", false, "Agree to terms non interactively")
}

// Info implements Command.Info.
func (c *agreeCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "agree",
		Args:     "<term>",
		Purpose:  "Agree to terms.",
		Doc:      agreeDoc,
		Examples: agreeExamples,
		SeeAlso: []string{
			"agreements",
		},
	})
}

// Init read and verifies the arguments.
func (c *agreeCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("missing arguments")
	}

	for _, t := range args {
		termId, err := charm.ParseTerm(t)
		if err != nil {
			return errors.Annotate(err, "invalid term format")
		}
		if termId.Revision == 0 {
			return errors.Errorf("must specify a valid term revision %q", t)
		}
		c.terms = append(c.terms, term{owner: termId.Owner, name: termId.Name, revision: termId.Revision})
		c.termIds = append(c.termIds, t)
	}
	if len(c.terms) == 0 {
		return errors.New("must specify a valid term revision")
	}
	return c.CommandBase.Init([]string{})
}

// Run implements Command.Run.
func (c *agreeCommand) Run(ctx *cmd.Context) error {
	client, err := c.BakeryClient()
	if err != nil {
		return errors.Trace(err)
	}

	termsClient, err := clientNew(api.HTTPClient(client))
	if err != nil {
		return err
	}

	if c.SkipTermContent {
		err := saveAgreements(c.StdContext, ctx, termsClient, c.terms)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	}

	needAgreement := []wireformat.GetTermsResponse{}
	terms, err := termsClient.GetUnsignedTerms(c.StdContext, &wireformat.CheckAgreementsRequest{
		Terms: c.termIds,
	})
	if err != nil {
		return errors.Annotate(err, "failed to retrieve terms")
	}
	needAgreement = append(needAgreement, terms...)

	if len(needAgreement) == 0 {
		fmt.Fprintf(ctx.Stdout, "Already agreed\n")
		return nil
	}

	err = printTerms(ctx, needAgreement)
	if err != nil {
		return errors.Trace(err)
	}
	fmt.Fprintf(ctx.Stdout, "Do you agree to the displayed terms? (Y/n): ")
	answer, err := userAnswer()
	if err != nil {
		return errors.Trace(err)
	}

	agreedTerms := make([]term, len(needAgreement))
	for i, t := range needAgreement {
		agreedTerms[i] = term{owner: t.Owner, name: t.Name, revision: t.Revision}
	}

	answer = strings.TrimSpace(answer)
	if userAgrees(answer) {
		err = saveAgreements(c.StdContext, ctx, termsClient, agreedTerms)
		if err != nil {
			return errors.Trace(err)
		}
	} else {
		fmt.Fprintf(ctx.Stdout, "You didn't agree to the presented terms.\n")
		return nil
	}

	return nil
}

func saveAgreements(stdContext context.Context, ctx *cmd.Context, termsClient api.Client, ts []term) error {
	agreements := make([]wireformat.SaveAgreement, len(ts))
	for i, t := range ts {
		agreements[i] = wireformat.SaveAgreement{
			TermOwner:    t.owner,
			TermName:     t.name,
			TermRevision: t.revision,
		}
	}
	response, err := termsClient.SaveAgreement(stdContext, &wireformat.SaveAgreements{Agreements: agreements})
	if err != nil {
		return errors.Annotate(err, "failed to save user agreement")
	}
	for _, agreement := range response.Agreements {
		termName := agreement.Term
		if agreement.Owner != "" {
			termName = fmt.Sprintf("%v/%v", agreement.Owner, agreement.Term)
		}
		_, err = fmt.Fprintf(ctx.Stdout, "Agreed to revision %v of %v for Juju users\n", agreement.Revision, termName)
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

var userAnswer = func() (string, error) {
	return bufio.NewReader(os.Stdin).ReadString('\n')
}

func printTerms(ctx *cmd.Context, terms []wireformat.GetTermsResponse) (returnErr error) {
	output := ""
	for _, t := range terms {
		if t.Owner != "" {
			output += fmt.Sprintf(`
=== %v/%v/%v: %v ===
%v
========
`, t.Owner, t.Name, t.Revision, t.CreatedOn, t.Content)
		} else {
			output += fmt.Sprintf(`
=== %v/%v: %v ===
%v
========
`, t.Name, t.Revision, t.CreatedOn, t.Content)
		}
	}
	defer func() {
		if returnErr != nil {
			_, err := fmt.Fprint(ctx.Stdout, output)
			returnErr = errors.Annotate(err, "failed to print plan")
		}
	}()

	buffer := bytes.NewReader([]byte(output))
	pager, err := pagerCmd()
	if err != nil {
		return err
	}
	pager.Stdout = ctx.Stdout
	pager.Stdin = buffer
	err = pager.Run()
	return errors.Annotate(err, "failed to print plan")
}

func pagerCmd() (*exec.Cmd, error) {
	os.Unsetenv("LESS")
	if pager := os.Getenv("PAGER"); pager != "" {
		if pagerPath, err := exec.LookPath(pager); err == nil {
			return exec.Command(pagerPath), nil
		}
	}
	if lessPath, err := exec.LookPath("less"); err == nil {
		return exec.Command(lessPath, "-P", "Press 'q' to quit after you've read the terms."), nil
	}
	return nil, errors.NotFoundf("pager")
}

func userAgrees(input string) bool {
	if input == "y" || input == "Y" || input == "" {
		return true
	}
	return false
}
