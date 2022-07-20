// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelupgrader

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/version/v2"

	// "github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/tools"
)

// Client provides methods that the Juju client command uses to upgrade models.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller

	st base.APICallCloser
}

// NewClient creates a new `Client` based on an existing authenticated API
// connection.
func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "ModelUpgrader")
	return &Client{
		ClientFacade: frontend,
		facade:       backend,
		st:           st,
	}
}

// AbortModelUpgrade aborts and archives the model upgrade
// synchronisation record, if any.
func (c *Client) AbortModelUpgrade(modelUUID string) error {
	args := params.ModelParam{
		ModelTag: names.NewModelTag(modelUUID).String(),
	}
	return c.facade.FacadeCall("AbortModelUpgrade", args, nil)
}

// UpgradeModel upgrades the model to the provided agent version.
func (c *Client) UpgradeModel(
	modelUUID string, clientVersion, targetVersion version.Number, officialClient bool,
	stream string, ignoreAgentVersions, druRun bool,
) (version.Number, bool, error) {
	args := params.UpgradeModelParams{
		ModelTag:            names.NewModelTag(modelUUID).String(),
		TargetVersion:       targetVersion,
		ClientVersion:       clientVersion,
		AgentStream:         stream,
		OfficialClient:      officialClient,
		IgnoreAgentVersions: ignoreAgentVersions,
		DryRun:              druRun,
	}
	var result params.UpgradeModelResult
	err := c.facade.FacadeCall("UpgradeModel", args, &result)
	if err != nil {
		return result.ChosenVersion, false, errors.Trace(err)
	}
	if result.Error != nil {
		err = apiservererrors.RestoreError(result.Error)
	}
	return result.ChosenVersion, result.CanImplicitUpload, errors.Trace(err)
}

// UploadTools uploads tools at the specified location to the API server over HTTPS.
func (c *Client) UploadTools(r io.ReadSeeker, vers version.Binary, additionalSeries ...string) (tools.List, error) {
	endpoint := fmt.Sprintf("/tools?binaryVersion=%s&series=%s", vers, strings.Join(additionalSeries, ","))
	contentType := "application/x-tar-gz"
	var resp params.ToolsResult
	if err := c.httpPost(r, endpoint, contentType, &resp); err != nil {
		return nil, errors.Trace(err)
	}
	return resp.ToolsList, nil
}

func (c *Client) httpPost(content io.ReadSeeker, endpoint, contentType string, response interface{}) error {
	req, err := http.NewRequest("POST", endpoint, content)
	if err != nil {
		return errors.Annotate(err, "cannot create upload request")
	}
	req.Header.Set("Content-Type", contentType)

	// apiConn, ok := c.st.(api.Connection)
	// if !ok {
	// 	return errors.New("unable to obtain api.Connection")
	// }
	// // The returned httpClient sets the base url to /model/<uuid> if it can.
	// httpClient, err := apiConn.HTTPClient()
	// if err != nil {
	// 	return errors.Trace(err)
	// }

	// The returned httpClient sets the base url to /model/<uuid> if it can.
	httpClient, err := c.st.HTTPClient()
	if err != nil {
		return errors.Trace(err)
	}

	if err := httpClient.Do(c.facade.RawAPICaller().Context(), req, response); err != nil {
		return errors.Trace(err)
	}
	return nil
}
