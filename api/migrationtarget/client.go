// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationtarget

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/httprequest"
	"github.com/juju/version"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	coremigration "github.com/juju/juju/core/migration"
	"github.com/juju/juju/tools"
)

type httpClient interface {
	HTTPClient() (*httprequest.Client, error)
}

// NewClient returns a new Client based on an existing API connection.
func NewClient(caller base.APICaller) *Client {
	return &Client{
		caller:            base.NewFacadeCaller(caller, "MigrationTarget"),
		httpClientFactory: caller,
	}
}

// Client is the client-side API for the MigrationTarget facade. It is
// used by the migrationmaster worker when talking to the target
// controller during a migration.
type Client struct {
	caller            base.FacadeCaller
	httpClientFactory httpClient
}

func (c *Client) Prechecks(model coremigration.ModelInfo) error {
	args := params.MigrationModelInfo{
		UUID:         model.UUID,
		Name:         model.Name,
		OwnerTag:     model.Owner.String(),
		AgentVersion: model.AgentVersion,
	}
	return c.caller.FacadeCall("Prechecks", args, nil)
}

// Import takes a serialized model and imports it into the target
// controller.
func (c *Client) Import(bytes []byte) error {
	serialized := params.SerializedModel{Bytes: bytes}
	return c.caller.FacadeCall("Import", serialized, nil)
}

// Abort removes all data relating to a previously imported model.
func (c *Client) Abort(modelUUID string) error {
	args := params.ModelArgs{ModelTag: names.NewModelTag(modelUUID).String()}
	return c.caller.FacadeCall("Abort", args, nil)
}

// Activate marks a migrated model as being ready to use.
func (c *Client) Activate(modelUUID string) error {
	args := params.ModelArgs{ModelTag: names.NewModelTag(modelUUID).String()}
	return c.caller.FacadeCall("Activate", args, nil)
}

// UploadCharm sends the content to the API server using an HTTP post in order
// to add the charm binary to the model specified.
func (c *Client) UploadCharm(modelUUID string, curl *charm.URL, content io.ReadSeeker) (*charm.URL, error) {
	args := url.Values{}
	args.Add("series", curl.Series)
	args.Add("schema", curl.Schema)
	args.Add("revision", strconv.Itoa(curl.Revision))
	apiURI := url.URL{Path: "/migrate/charms", RawQuery: args.Encode()}

	contentType := "application/zip"
	var resp params.CharmsResponse
	if err := c.httpPost(modelUUID, content, apiURI.String(), contentType, &resp); err != nil {
		return nil, errors.Trace(err)
	}

	curl, err := charm.ParseURL(resp.CharmURL)
	if err != nil {
		return nil, errors.Annotatef(err, "bad charm URL in response")
	}
	return curl, nil
}

// UploadTools uploads tools at the specified location to the API server over HTTPS
// for the specified model.
func (c *Client) UploadTools(modelUUID string, r io.ReadSeeker, vers version.Binary, additionalSeries ...string) (tools.List, error) {
	endpoint := fmt.Sprintf("/migrate/tools?binaryVersion=%s&series=%s", vers, strings.Join(additionalSeries, ","))
	contentType := "application/x-tar-gz"
	var resp params.ToolsResult
	if err := c.httpPost(modelUUID, r, endpoint, contentType, &resp); err != nil {
		return nil, errors.Trace(err)
	}
	return resp.ToolsList, nil
}

func (c *Client) httpPost(modelUUID string, content io.ReadSeeker, endpoint, contentType string, response interface{}) error {
	req, err := http.NewRequest("POST", endpoint, nil)
	if err != nil {
		return errors.Annotate(err, "cannot create upload request")
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set(params.MigrationModelHTTPHeader, modelUUID)

	// The returned httpClient sets the base url to /model/<uuid> if it can.
	httpClient, err := c.httpClientFactory.HTTPClient()
	if err != nil {
		return errors.Trace(err)
	}

	if err := httpClient.Do(req, content, response); err != nil {
		return errors.Trace(err)
	}
	return nil
}
