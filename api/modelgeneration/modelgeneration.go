// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelgeneration

import (
	"time"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/model"
)

// Client provides methods that the Juju client command uses to interact
// with models stored in the Juju Server.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient creates a new `Client` based on an existing authenticated API
// connection.
func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "ModelGeneration")
	return &Client{ClientFacade: frontend, facade: backend}
}

// AddBranch adds a new branch to the model.
func (c *Client) AddBranch(branchName string) error {
	var result params.ErrorResult
	err := c.facade.FacadeCall("AddBranch", argForBranch(branchName), &result)
	if err != nil {
		return errors.Trace(err)
	}
	if result.Error != nil {
		return errors.Trace(result.Error)
	}
	return nil
}

// Abort aborts an existing branch to the model.
func (c *Client) AbortBranch(branchName string) error {
	var result params.ErrorResult
	err := c.facade.FacadeCall("AbortBranch", argForBranch(branchName), &result)
	if err != nil {
		return errors.Trace(err)
	}
	if result.Error != nil {
		return errors.Trace(result.Error)
	}
	return nil
}

// CommitBranch commits the branch with the input name to the model,
// effectively completing it and applying all branch changes across the model.
// The new generation ID of the model is returned.
func (c *Client) CommitBranch(branchName string) (int, error) {
	var result params.IntResult
	err := c.facade.FacadeCall("CommitBranch", argForBranch(branchName), &result)
	if err != nil {
		return 0, errors.Trace(err)
	}
	if result.Error != nil {
		return 0, errors.Trace(result.Error)
	}
	return result.Result, nil
}

// TrackBranch sets the input units and/or applications
// to track changes made under the input branch name.
func (c *Client) TrackBranch(branchName string, entities []string) error {
	var result params.ErrorResults
	arg := params.BranchTrackArg{
		BranchName: branchName,
	}
	if len(entities) == 0 {
		return errors.New("no units or applications specified")
	}
	for _, entity := range entities {
		switch {
		case names.IsValidApplication(entity):
			arg.Entities = append(arg.Entities,
				params.Entity{Tag: names.NewApplicationTag(entity).String()})
		case names.IsValidUnit(entity):
			arg.Entities = append(arg.Entities,
				params.Entity{Tag: names.NewUnitTag(entity).String()})
		default:
			return errors.Errorf("%q is not an application or a unit", entity)
		}
	}
	err := c.facade.FacadeCall("TrackBranch", arg, &result)
	if err != nil {
		return errors.Trace(err)
	}

	if err := result.Combine(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// HasActiveBranch returns true if the model has an
// "in-flight" branch with the input name.
func (c *Client) HasActiveBranch(branchName string) (bool, error) {
	var result params.BoolResult
	err := c.facade.FacadeCall("HasActiveBranch", argForBranch(branchName), &result)
	if err != nil {
		return false, errors.Trace(err)
	}
	if result.Error != nil {
		return false, errors.Trace(result.Error)
	}
	return result.Result, nil
}

// BranchInfo returns information about "in-flight" branches.
// If a non-empty string is supplied for branch name,
// then only information for that branch is returned.
// Supplying true for detailed returns extra unit detail for the branch.
func (c *Client) BranchInfo(
	branchName string, detailed bool, formatTime func(time.Time) string,
) (model.GenerationSummaries, error) {
	arg := params.BranchInfoArgs{Detailed: detailed}
	if branchName != "" {
		arg.BranchNames = []string{branchName}
	}

	var result params.GenerationResults
	err := c.facade.FacadeCall("BranchInfo", arg, &result)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if result.Error != nil {
		return nil, errors.Trace(result.Error)
	}
	return generationInfoFromResult(result, detailed, formatTime), nil
}

func argForBranch(branchName string) params.BranchArg {
	return params.BranchArg{
		BranchName: branchName,
	}
}

func generationInfoFromResult(
	results params.GenerationResults, detailed bool, formatTime func(time.Time) string,
) model.GenerationSummaries {
	summaries := make(model.GenerationSummaries)
	for _, res := range results.Generations {
		appDeltas := make([]model.GenerationApplication, len(res.Applications))
		for i, a := range res.Applications {
			bApp := model.GenerationApplication{
				ApplicationName: a.ApplicationName,
				UnitProgress:    a.UnitProgress,
				ConfigChanges:   a.ConfigChanges,
			}
			if detailed {
				bApp.UnitDetail = &model.GenerationUnits{
					UnitsTracking: a.UnitsTracking,
					UnitsPending:  a.UnitsPending,
				}
			}
			appDeltas[i] = bApp
		}
		summaries[res.BranchName] = model.Generation{
			Created:      formatTime(time.Unix(res.Created, 0)),
			CreatedBy:    res.CreatedBy,
			Applications: appDeltas,
		}
	}
	return summaries
}
