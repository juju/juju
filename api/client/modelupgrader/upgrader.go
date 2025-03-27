// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelupgrader

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/api/base"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/tools"
	"github.com/juju/juju/rpc/params"
)

// Option is a function that can be used to configure a Client.
type Option = base.Option

// WithTracer returns an Option that configures the Client to use the
// supplied tracer.
var WithTracer = base.WithTracer

// Client provides methods that the Juju client command uses to upgrade models.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller

	st base.APICallCloser
}

// NewClient creates a new `Client` based on an existing authenticated API
// connection.
func NewClient(st base.APICallCloser, options ...Option) *Client {
	frontend, backend := base.NewClientFacade(st, "ModelUpgrader", options...)
	return &Client{
		ClientFacade: frontend,
		facade:       backend,
		st:           st,
	}
}

// AbortModelUpgrade aborts and archives the model upgrade
// synchronisation record, if any.
func (c *Client) AbortModelUpgrade(ctx context.Context, modelUUID string) error {
	args := params.ModelParam{
		ModelTag: names.NewModelTag(modelUUID).String(),
	}
	return c.facade.FacadeCall(ctx, "AbortModelUpgrade", args, nil)
}

// UpgradeModel upgrades the model to the provided agent version.
// The provided target version could be version.Zero, in which case
// the best version is selected by the controller and returned as
// ChosenVersion in the result.
func (c *Client) UpgradeModel(
	ctx context.Context,
	modelUUID string, targetVersion semversion.Number, stream string, ignoreAgentVersions, druRun bool,
) (semversion.Number, error) {
	args := params.UpgradeModelParams{
		ModelTag:            names.NewModelTag(modelUUID).String(),
		TargetVersion:       targetVersion,
		AgentStream:         stream,
		IgnoreAgentVersions: ignoreAgentVersions,
		DryRun:              druRun,
	}
	var result params.UpgradeModelResult
	err := c.facade.FacadeCall(ctx, "UpgradeModel", args, &result)
	if err != nil {
		return result.ChosenVersion, errors.Trace(err)
	}
	if result.Error != nil {
		err = apiservererrors.RestoreError(result.Error)
	}
	return result.ChosenVersion, errors.Trace(err)
}

// UploadTools uploads tools at the specified location to the API server over HTTPS.
func (c *Client) UploadTools(ctx context.Context, r io.Reader, vers semversion.Binary) (tools.List, error) {
	req, err := http.NewRequest("POST", fmt.Sprintf("/tools?binaryVersion=%s", vers), r)
	if err != nil {
		return nil, errors.Annotate(err, "cannot create upload request")
	}
	req.Header.Set("Content-Type", "application/x-tar-gz")

	var resp params.ToolsResult
	// The returned httpClient sets the base url to /model/<uuid> if it can.
	httpClient, err := c.st.HTTPClient()
	if err != nil {
		return nil, errors.Trace(err)
	}

	if err := httpClient.Do(ctx, req, &resp); err != nil {
		return nil, errors.Trace(err)
	}
	return resp.ToolsList, nil
}
