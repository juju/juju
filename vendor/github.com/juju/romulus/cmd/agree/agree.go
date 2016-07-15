// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agree

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/juju/cmd/modelcmd"
	"launchpad.net/gnuflag"

	"github.com/juju/romulus/api/terms"
)

var (
	clientNew = terms.NewClient
)

const agreeDoc = `
Agree to the terms required by a charm.

When deploying a charm that requires agreement to terms, use 'juju agree' to
view the terms and agree to them. Then the charm may be deployed.

Once you have agreed to terms, you will not be prompted to view them again.

Examples:
    # Displays terms for somePlan revision 1 and prompts for agreement.
    juju agree somePlan/1

    # Displays the terms for revision 1 of somePlan, revision 2 of otherPlan,
    # and prompts for agreement.
    juju agree somePlan/1 otherPlan/2

    # Agrees to the terms without prompting.
    juju agree somePlan/1 otherPlan/2 --yes
`

// NewAgreeCommand returns a new command that can be
// used to create user agreements.
func NewAgreeCommand() cmd.Command {
	return &agreeCommand{}
}

type term struct {
	name     string
	revision int
}

// agreeCommand creates a user agreement to the specified terms.
type agreeCommand struct {
	modelcmd.JujuCommandBase
	out cmd.Output

	terms           []term
	termIds         []string
	SkipTermContent bool
}

// SetFlags implements Command.SetFlags.
func (c *agreeCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.SkipTermContent, "yes", false, "Agree to terms non interactively")
	c.out.AddFlags(f, "json", cmd.DefaultFormatters)
}

// Info implements Command.Info.
func (c *agreeCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "agree",
		Args:    "<term>",
		Purpose: "Agree to terms.",
		Doc:     agreeDoc,
	}
}

// Init read and verifies the arguments.
func (c *agreeCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("missing arguments")
	}

	for _, t := range args {
		name, rev, err := parseTermRevision(t)
		if err != nil {
			return errors.Annotate(err, "invalid term format")
		}
		if rev == 0 {
			return errors.Errorf("must specify a valid term revision %q", t)
		}
		c.terms = append(c.terms, term{name, rev})
		c.termIds = append(c.termIds, t)
	}
	if len(c.terms) == 0 {
		return errors.New("must specify a valid term revision")
	}
	return nil
}

// Run implements Command.Run.
func (c *agreeCommand) Run(ctx *cmd.Context) error {
	client, err := c.BakeryClient()
	if err != nil {
		return errors.Trace(err)
	}

	termsClient, err := clientNew(terms.HTTPClient(client))
	if err != nil {
		return err
	}

	if c.SkipTermContent {
		err := saveAgreements(ctx, termsClient, c.terms)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	}

	needAgreement := []terms.GetTermsResponse{}
	terms, err := termsClient.GetUnsignedTerms(&terms.CheckAgreementsRequest{
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
		agreedTerms[i] = term{name: t.Name, revision: t.Revision}
	}

	answer = strings.TrimSpace(answer)
	if userAgrees(answer) {
		err = saveAgreements(ctx, termsClient, agreedTerms)
		if err != nil {
			return errors.Trace(err)
		}
	} else {
		fmt.Fprintf(ctx.Stdout, "You didn't agree to the presented terms.\n")
		return nil
	}

	return nil
}

func saveAgreements(ctx *cmd.Context, termsClient terms.Client, ts []term) error {
	agreements := make([]terms.SaveAgreement, len(ts))
	for i, t := range ts {
		agreements[i] = terms.SaveAgreement{
			TermName:     t.name,
			TermRevision: t.revision,
		}
	}
	response, err := termsClient.SaveAgreement(&terms.SaveAgreements{Agreements: agreements})
	if err != nil {
		return errors.Annotate(err, "failed to save user agreement")
	}
	for _, agreement := range response.Agreements {
		_, err = fmt.Fprintf(ctx.Stdout, "Agreed to revision %v of %v for Juju users\n", agreement.Revision, agreement.Term)
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

var userAnswer = func() (string, error) {
	return bufio.NewReader(os.Stdin).ReadString('\n')
}

func parseTermRevision(s string) (string, int, error) {
	fail := func(err error) (string, int, error) {
		return "", -1, err
	}
	tokens := strings.Split(s, "/")
	if len(tokens) == 1 {
		return tokens[0], 0, nil
	} else if len(tokens) > 2 {
		return fail(errors.New("unknown term revision format"))
	}

	termName := tokens[0]
	termRevisionString := tokens[1]
	termRevision, err := strconv.Atoi(termRevisionString)
	if err != nil {
		return fail(errors.Trace(err))
	}
	return termName, termRevision, nil
}

func printTerms(ctx *cmd.Context, terms []terms.GetTermsResponse) error {
	output := ""
	for _, t := range terms {
		output += fmt.Sprintf(`
=== %v/%v: %v ===
%v
========
`, t.Name, t.Revision, t.CreatedOn, t.Content)
	}
	buffer := bytes.NewReader([]byte(output))
	less := exec.Command("less")
	less.Args = []string{"less", "-P", "Press 'q' to quit after you've read the terms."}
	less.Stdout = ctx.Stdout
	less.Stdin = buffer
	err := less.Run()
	if err != nil {
		fmt.Fprintf(ctx.Stdout, output)
		return errors.Annotate(err, "failed to print plan")
	}
	return nil
}

func userAgrees(input string) bool {
	if input == "y" || input == "Y" || input == "" {
		return true
	}
	return false
}
