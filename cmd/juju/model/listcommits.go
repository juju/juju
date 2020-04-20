// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/gosuri/uitable"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api/modelgeneration"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/juju/osenv"
)

const (
	listCommitsSummary = "Lists commits history"
	listCommitsDoc     = `
Commits shows the timeline of changes to the model that occurred through branching.
It does not take into account other changes to the model that did not occur through a managed branch.
Lists consists of:
- the commit number 
- user who committed the branch 
- when the branch was committed 
- the branch name

Examples:
    juju commits
    juju commits --utc

See also:
	commits
	show-commit
    add-branch
    track
    branch
    abort
    diff
`
)

// CommitsCommand supplies the "commit" CLI command used to commit changes made
// under a branch, to the model.
type CommitsCommand struct {
	modelcmd.ModelCommandBase

	api CommitsCommandAPI
	out cmd.Output

	isoTime bool
}

// CommitsCommandAPI defines an API interface to be used during testing.
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination ./mocks/commits_mock.go github.com/juju/juju/cmd/juju/model CommitsCommandAPI
type CommitsCommandAPI interface {
	Close() error

	// ListCommitsBranch commits the branch with the input name to the model,
	// effectively completing it and applying
	// all branch changes across the model.
	// The new generation ID of the model is returned.
	ListCommits() (model.GenerationCommits, error)
}

// NewCommitCommand wraps CommitsCommand with sane model settings.
func NewCommitsCommand() cmd.Command {
	return modelcmd.Wrap(&CommitsCommand{})
}

// Info implements part of the cmd.Command interface.
func (c *CommitsCommand) Info() *cmd.Info {
	info := &cmd.Info{
		Name:    "commits",
		Purpose: listCommitsSummary,
		Doc:     listCommitsDoc,
		Aliases: []string{"list-commits"},
	}
	return jujucmd.Info(info)
}

// SetFlags implements part of the cmd.Command interface.
func (c *CommitsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.BoolVar(&c.isoTime, "utc", false, "Display time as UTC in RFC3339 format")
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": c.printTabular,
	})
}

// Init implements part of the cmd.Command interface.
func (c *CommitsCommand) Init(args []string) error {
	lArgs := len(args)
	if lArgs > 0 {
		return errors.Errorf("expected no arguments, but got %v", lArgs)
	}

	// If use of ISO time not specified on command line, check env var.
	if !c.isoTime {
		envVarValue := os.Getenv(osenv.JujuStatusIsoTimeEnvKey)
		if envVarValue != "" {
			var err error
			if c.isoTime, err = strconv.ParseBool(envVarValue); err != nil {
				return errors.Annotatef(err, "invalid %s env var, expected true|false", osenv.JujuStatusIsoTimeEnvKey)
			}
		}
	}
	return nil
}

// getAPI returns the API. This allows passing in a test CommitCommandAPI
// implementation.
func (c *CommitsCommand) getAPI() (CommitsCommandAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	api, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Annotate(err, "opening API connection")
	}
	client := modelgeneration.NewClient(api)
	return client, nil
}

// Run implements the meaty part of the cmd.Command interface.
func (c *CommitsCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	commits, err := client.ListCommits()
	if err != nil {
		return errors.Trace(err)
	}
	if len(commits) == 0 && c.out.Name() == "tabular" {
		ctx.Infof("No commits to list")
		return nil
	}
	tabular := c.constructYamlJson(commits)
	return errors.Trace(c.out.Write(ctx, tabular))
}

// printTabular prints the list of actions in tabular format
func (c *CommitsCommand) printTabular(writer io.Writer, value interface{}) error {
	list, ok := value.(formattedCommitList)
	if !ok {
		return errors.New("unexpected value")
	}

	table := uitable.New()
	table.MaxColWidth = 50
	table.Wrap = true

	table.AddRow("Commit", "Committed at", "Committed by", "Branch name")
	for _, c := range list.Commits {
		table.AddRow(c.CommitId, c.CommittedAt, c.CommittedBy, c.BranchName)
	}
	_, _ = fmt.Fprint(writer, table)
	return nil
}

func (c *CommitsCommand) constructYamlJson(generations model.GenerationCommits) formattedCommitList {

	sort.Slice(generations, func(i, j int) bool {
		return generations[i].GenerationId > generations[j].GenerationId
	})

	var result formattedCommitList
	for _, gen := range generations {
		var formattedDate string
		if c.out.Name() == "tabular" {
			formattedDate = common.UserFriendlyDuration(gen.Completed, time.Now())
		} else {
			formattedDate = common.FormatTime(&gen.Completed, c.isoTime)
		}
		fmc := formattedCommit{
			CommitId:    gen.GenerationId,
			BranchName:  gen.BranchName,
			CommittedAt: formattedDate,
			CommittedBy: gen.CompletedBy,
		}
		result.Commits = append(result.Commits, fmc)
	}
	return result
}

type formattedCommit struct {
	CommitId    int    `json:"id" yaml:"id"`
	BranchName  string `json:"branch-name" yaml:"branch-name"`
	CommittedAt string `json:"committed-at" yaml:"committed-at"`
	CommittedBy string `json:"committed-by" yaml:"committed-by"`
}

type formattedCommitList struct {
	Commits []formattedCommit `json:"commits" yaml:"commits"`
}
