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

// AddGeneration adds a model generation to the config.
func (c *Client) AddGeneration(modelUUID, branchName string) error {
	var result params.ErrorResult
	err := c.facade.FacadeCall("AddGeneration", argForBranch(modelUUID, branchName), &result)
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
func (c *Client) CommitBranch(modelUUID, branchName string) (int, error) {
	var result params.IntResult
	err := c.facade.FacadeCall("CommitBranch", argForBranch(modelUUID, branchName), &result)
	if err != nil {
		return 0, errors.Trace(err)
	}
	if result.Error != nil {
		return 0, errors.Trace(result.Error)
	}
	return result.Result, nil
}

// TrackBranch sets the input units and/or applications to track changes made
// under the input branch name.
func (c *Client) TrackBranch(modelUUID, branchName string, entities []string) error {
	var result params.ErrorResults
	arg := params.BranchTrackArg{
		Model:      argForModel(modelUUID),
		BranchName: branchName,
	}
	if len(entities) == 0 {
		return errors.Trace(errors.New("No units or applications to advance"))
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
			return errors.Trace(errors.New("Must be application or unit"))
		}
	}
	err := c.facade.FacadeCall("AdvanceGeneration", arg, &result)
	if err != nil {
		return errors.Trace(err)
	}

	// If there were errors based on the advancing units, return those.
	// Otherwise check the results of auto-completion.
	if err := result.Combine(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// HasActiveBranch returns true if the model has an "in-flight" branch with
// the input name.
func (c *Client) HasActiveBranch(modelUUID, branchName string) (bool, error) {
	var result params.BoolResult
	err := c.facade.FacadeCall("HasActiveBranch", argForBranch(modelUUID, branchName), &result)
	if err != nil {
		return false, errors.Trace(err)
	}
	if result.Error != nil {
		return false, errors.Trace(result.Error)
	}
	return result.Result, nil
}

// GenerationInfo returns a list of application with changes in the "next"
// generation, with units moved to the generation, and any generational
// configuration changes.
func (c *Client) GenerationInfo(
	modelUUID, branchName string, formatTime func(time.Time) string,
) (model.GenerationSummaries, error) {
	var result params.GenerationResult
	err := c.facade.FacadeCall("BranchInfo", argForBranch(modelUUID, branchName), &result)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if result.Error != nil {
		return nil, errors.Trace(result.Error)
	}
	return generationInfoFromResult(result.Generation, formatTime), nil
}

func argForBranch(modelUUID, branchName string) params.BranchArg {
	return params.BranchArg{
		Model:      argForModel(modelUUID),
		BranchName: branchName,
	}
}

func argForModel(modelUUID string) params.Entity {
	return params.Entity{Tag: names.NewModelTag(modelUUID).String()}
}

func generationInfoFromResult(res params.Generation, formatTime func(time.Time) string) model.GenerationSummaries {
	appDeltas := make([]model.GenerationApplication, len(res.Applications))
	for i, a := range res.Applications {
		appDeltas[i] = model.GenerationApplication{
			ApplicationName: a.ApplicationName,
			Units:           a.Units,
			ConfigChanges:   a.ConfigChanges,
		}
	}
	gen := model.Generation{
		Created:      formatTime(time.Unix(res.Created, 0)),
		CreatedBy:    res.CreatedBy,
		Applications: appDeltas,
	}
	return map[string]model.Generation{res.BranchName: gen}
}
