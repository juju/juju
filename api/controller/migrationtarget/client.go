// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationtarget

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/juju/charm/v8"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/version/v2"
	"gopkg.in/httprequest.v1"

	"github.com/juju/juju/api/base"
	coremigration "github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/resources"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/tools"
	jujuversion "github.com/juju/juju/version"
)

// NewClient returns a new Client based on an existing API connection.
func NewClient(caller base.APICaller) *Client {
	return &Client{
		caller:            base.NewFacadeCaller(caller, "MigrationTarget"),
		httpClientFactory: caller.HTTPClient,
	}
}

// Client is the client-side API for the MigrationTarget facade. It is
// used by the migrationmaster worker when talking to the target
// controller during a migration.
type Client struct {
	caller            base.FacadeCaller
	httpClientFactory func() (*httprequest.Client, error)
}

// BestFacadeVersion returns the best supported facade version
// on the target controller.
func (c *Client) BestFacadeVersion() int {
	return c.caller.BestAPIVersion()
}

func (c *Client) Prechecks(model coremigration.ModelInfo) error {
	args := params.MigrationModelInfo{
		UUID:                   model.UUID,
		Name:                   model.Name,
		OwnerTag:               model.Owner.String(),
		AgentVersion:           model.AgentVersion,
		ControllerAgentVersion: model.ControllerAgentVersion,
	}
	return errors.Trace(c.caller.FacadeCall("Prechecks", args, nil))
}

// Import takes a serialized model and imports it into the target
// controller.
func (c *Client) Import(bytes []byte) error {
	serialized := params.SerializedModel{Bytes: bytes}
	return errors.Trace(c.caller.FacadeCall("Import", serialized, nil))
}

// Abort removes all data relating to a previously imported model.
func (c *Client) Abort(modelUUID string) error {
	args := params.ModelArgs{ModelTag: names.NewModelTag(modelUUID).String()}
	return errors.Trace(c.caller.FacadeCall("Abort", args, nil))
}

// Activate marks a migrated model as being ready to use.
func (c *Client) Activate(modelUUID string, sourceInfo coremigration.SourceControllerInfo, relatedModels []string) error {
	if c.caller.BestAPIVersion() < 2 {
		args := params.ModelArgs{ModelTag: names.NewModelTag(modelUUID).String()}
		return errors.Trace(c.caller.FacadeCall("Activate", args, nil))
	}
	args := params.ActivateModelArgs{
		ModelTag: names.NewModelTag(modelUUID).String(),
	}
	if len(relatedModels) > 0 {
		args.ControllerTag = sourceInfo.ControllerTag.String()
		args.ControllerAlias = sourceInfo.ControllerAlias
		args.SourceAPIAddrs = sourceInfo.Addrs
		args.SourceCACert = sourceInfo.CACert
		args.CrossModelUUIDs = relatedModels
	}
	return errors.Trace(c.caller.FacadeCall("Activate", args, nil))
}

// UploadCharm sends the content to the API server using an HTTP post in order
// to add the charm binary to the model specified.
func (c *Client) UploadCharm(modelUUID string, curl *charm.URL, content io.ReadSeeker) (*charm.URL, error) {
	args := url.Values{}
	args.Add("name", curl.Name)
	args.Add("schema", curl.Schema)
	args.Add("arch", curl.Architecture)
	args.Add("user", curl.User)
	args.Add("series", curl.Series)
	args.Add("revision", strconv.Itoa(curl.Revision))

	apiURI := url.URL{
		Path:     "/migrate/charms",
		RawQuery: args.Encode(),
	}

	contentType := "application/zip"
	var resp params.CharmsResponse
	if err := c.httpPost(modelUUID, content, apiURI.String(), contentType, &resp); err != nil {
		return nil, errors.Trace(err)
	}

	respCurl, err := charm.ParseURL(resp.CharmURL)
	if err != nil {
		return nil, errors.Annotatef(err, "bad charm URL in response")
	}
	return respCurl, nil
}

// UploadTools uploads tools at the specified location to the API server over HTTPS
// for the specified model.
func (c *Client) UploadTools(modelUUID string, r io.ReadSeeker, vers version.Binary) (tools.List, error) {
	endpoint := fmt.Sprintf("/migrate/tools?binaryVersion=%s", vers)
	contentType := "application/x-tar-gz"
	var resp params.ToolsResult
	if err := c.httpPost(modelUUID, r, endpoint, contentType, &resp); err != nil {
		return nil, errors.Trace(err)
	}
	return resp.ToolsList, nil
}

// UploadResource uploads a resource to the migration endpoint.
func (c *Client) UploadResource(modelUUID string, res resources.Resource, r io.ReadSeeker) error {
	args := makeResourceArgs(res)
	args.Add("application", res.ApplicationID)
	err := c.resourcePost(modelUUID, args, r)
	return errors.Trace(err)
}

// SetPlaceholderResource sets the metadata for a placeholder resource.
func (c *Client) SetPlaceholderResource(modelUUID string, res resources.Resource) error {
	args := makeResourceArgs(res)
	args.Add("application", res.ApplicationID)
	err := c.resourcePost(modelUUID, args, nil)
	return errors.Trace(err)
}

// SetUnitResource sets the metadata for a particular unit resource.
func (c *Client) SetUnitResource(modelUUID, unit string, res resources.Resource) error {
	args := makeResourceArgs(res)
	args.Add("unit", unit)
	err := c.resourcePost(modelUUID, args, nil)
	return errors.Trace(err)
}

func (c *Client) resourcePost(modelUUID string, args url.Values, r io.ReadSeeker) error {
	uri := "/migrate/resources?" + args.Encode()
	contentType := "application/octet-stream"
	err := c.httpPost(modelUUID, r, uri, contentType, nil)
	return errors.Trace(err)
}

func makeResourceArgs(res resources.Resource) url.Values {
	args := url.Values{}
	args.Add("name", res.Name)
	args.Add("type", res.Type.String())
	args.Add("path", res.Path)
	args.Add("description", res.Description)
	args.Add("origin", res.Origin.String())
	args.Add("revision", fmt.Sprintf("%d", res.Revision))
	args.Add("size", fmt.Sprintf("%d", res.Size))
	args.Add("fingerprint", res.Fingerprint.Hex())
	if res.Username != "" {
		args.Add("user", res.Username)
	}
	if !res.IsPlaceholder() {
		args.Add("timestamp", fmt.Sprint(res.Timestamp.UnixNano()))
	}
	return args
}

func (c *Client) httpPost(modelUUID string, content io.ReadSeeker, endpoint, contentType string, response interface{}) error {
	req, err := http.NewRequest("POST", endpoint, content)
	if err != nil {
		return errors.Annotate(err, "cannot create upload request")
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set(params.MigrationModelHTTPHeader, modelUUID)

	// The returned httpClient sets the base url to /model/<uuid> if it can.
	httpClient, err := c.httpClientFactory()
	if err != nil {
		return errors.Trace(err)
	}

	return errors.Trace(httpClient.Do(c.caller.RawAPICaller().Context(), req, response))
}

// OpenLogTransferStream connects to the migration logtransfer
// endpoint on the target controller and returns a stream that JSON
// logs records can be fed into. The objects written should be params.LogRecords.
func (c *Client) OpenLogTransferStream(modelUUID string) (base.Stream, error) {
	attrs := url.Values{}
	// TODO(wallyworld) - remove in juju 4
	attrs.Set("jujuclientversion", jujuversion.Current.String())
	headers := http.Header{}
	headers.Set(params.MigrationModelHTTPHeader, modelUUID)
	caller := c.caller.RawAPICaller()
	stream, err := caller.ConnectControllerStream("/migrate/logtransfer", attrs, headers)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return stream, nil
}

// LatestLogTime asks the target controller for the time of the latest
// log record it has seen. This can be used to make the log transfer
// restartable.
func (c *Client) LatestLogTime(modelUUID string) (time.Time, error) {
	var result time.Time
	args := params.ModelArgs{ModelTag: names.NewModelTag(modelUUID).String()}
	err := c.caller.FacadeCall("LatestLogTime", args, &result)
	if err != nil {
		return time.Time{}, errors.Trace(err)
	}
	return result, nil
}

// AdoptResources asks the cloud provider to update the controller
// tags for a model's resources. This prevents the resources from
// being destroyed if the source controller is destroyed after the
// model is migrated away.
func (c *Client) AdoptResources(modelUUID string) error {
	args := params.AdoptResourcesArgs{
		ModelTag:                names.NewModelTag(modelUUID).String(),
		SourceControllerVersion: jujuversion.Current,
	}
	return errors.Trace(c.caller.FacadeCall("AdoptResources", args, nil))
}

// CACert returns the CA certificate associated with
// the connection.
func (c *Client) CACert() (string, error) {
	var result params.BytesResult
	err := c.caller.FacadeCall("CACert", nil, &result)
	if err != nil {
		return "", errors.Trace(err)
	}
	return string(result.Result), nil
}

// CheckMachines compares the machines in state with the ones reported
// by the provider and reports any discrepancies.
func (c *Client) CheckMachines(modelUUID string) ([]error, error) {
	var result params.ErrorResults
	args := params.ModelArgs{ModelTag: names.NewModelTag(modelUUID).String()}
	err := c.caller.FacadeCall("CheckMachines", args, &result)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var results []error
	for _, res := range result.Results {
		results = append(results, errors.New(res.Error.Message))
	}
	return results, nil
}
