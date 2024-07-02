// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationtarget

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/juju/description/v6"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"
	"gopkg.in/httprequest.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	coremigration "github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/resources"
	"github.com/juju/juju/internal/tools"
	"github.com/juju/juju/rpc/params"
	jujuversion "github.com/juju/juju/version"
)

// Option is a function that can be used to configure a Client.
type Option = base.Option

// WithTracer returns an Option that configures the Client to use the
// supplied tracer.
var WithTracer = base.WithTracer

// NewClient returns a new Client based on an existing API connection.
func NewClient(caller base.APICaller, options ...Option) *Client {
	return &Client{
		caller:                base.NewFacadeCaller(caller, "MigrationTarget", options...),
		httpRootClientFactory: caller.RootHTTPClient,
	}
}

// Client is the client-side API for the MigrationTarget facade. It is
// used by the migrationmaster worker when talking to the target
// controller during a migration.
type Client struct {
	caller                base.FacadeCaller
	httpRootClientFactory func() (*httprequest.Client, error)
}

// BestFacadeVersion returns the best supported facade version
// on the target controller.
func (c *Client) BestFacadeVersion() int {
	return c.caller.BestAPIVersion()
}

// Prechecks checks that the target controller is able to accept the
// model being migrated.
func (c *Client) Prechecks(model coremigration.ModelInfo) error {
	// The model description is marshalled into YAML (description package does
	// not support JSON) to prevent potential issues with
	// marshalling/unmarshalling on the target API controller.
	serialised, err := description.Serialize(model.ModelDescription)
	if err != nil {
		return errors.Annotate(err, "failed to marshal model description")
	}

	// Pass all the known facade versions to the controller so that it
	// can check that the target controller supports them. Passing all of them
	// ensures that we don't have to update this code when new facades are
	// added, or if the controller wants to change the logic service side.
	supported := api.SupportedFacadeVersions()
	versions := make(map[string][]int, len(supported))
	for name, version := range supported {
		versions[name] = version
	}

	args := params.MigrationModelInfo{
		UUID:                   model.UUID,
		Name:                   model.Name,
		OwnerTag:               model.Owner.String(),
		AgentVersion:           model.AgentVersion,
		ControllerAgentVersion: model.ControllerAgentVersion,
		FacadeVersions:         versions,
		ModelDescription:       serialised,
	}
	return errors.Trace(c.caller.FacadeCall(context.TODO(), "Prechecks", args, nil))
}

// Import takes a serialized model and imports it into the target
// controller.
func (c *Client) Import(bytes []byte) error {
	serialized := params.SerializedModel{Bytes: bytes}
	return errors.Trace(c.caller.FacadeCall(context.TODO(), "Import", serialized, nil))
}

// Abort removes all data relating to a previously imported model.
func (c *Client) Abort(modelUUID string) error {
	args := params.ModelArgs{ModelTag: names.NewModelTag(modelUUID).String()}
	return errors.Trace(c.caller.FacadeCall(context.TODO(), "Abort", args, nil))
}

// Activate marks a migrated model as being ready to use.
func (c *Client) Activate(modelUUID string, sourceInfo coremigration.SourceControllerInfo, relatedModels []string) error {
	if c.caller.BestAPIVersion() < 2 {
		args := params.ModelArgs{ModelTag: names.NewModelTag(modelUUID).String()}
		return errors.Trace(c.caller.FacadeCall(context.TODO(), "Activate", args, nil))
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
	return errors.Trace(c.caller.FacadeCall(context.TODO(), "Activate", args, nil))
}

// UploadCharm sends the content to the API server using an HTTP post in order
// to add the charm binary to the model specified.
func (c *Client) UploadCharm(ctx context.Context, modelUUID string, curl string, charmRef string, content io.ReadSeeker) (string, error) {
	apiURI := url.URL{Path: fmt.Sprintf("/migrate/charms/%s", charmRef)}

	contentType := "application/zip"
	resp := &http.Response{}
	// Add Juju-Curl header to Put operation. Juju 3.4 apiserver
	// expects this header to be present, since we still need some
	// of the values from the charm url.
	headers := map[string]string{
		"Juju-Curl": curl,
	}
	if err := c.httpPut(ctx, modelUUID, content, apiURI.String(), contentType, headers, &resp); err != nil {
		return "", errors.Trace(err)
	}

	respCurl := resp.Header.Get("Juju-Curl")
	if respCurl == "" {
		return "", errors.Errorf("response returned no charm URL")
	}
	return respCurl, nil
}

// UploadTools uploads tools at the specified location to the API server over HTTPS
// for the specified model.
func (c *Client) UploadTools(ctx context.Context, modelUUID string, r io.ReadSeeker, vers version.Binary) (tools.List, error) {
	endpoint := fmt.Sprintf("/migrate/tools?binaryVersion=%s", vers)
	contentType := "application/x-tar-gz"
	var resp params.ToolsResult
	if err := c.httpPost(ctx, modelUUID, r, endpoint, contentType, nil, &resp); err != nil {
		return nil, errors.Trace(err)
	}
	return resp.ToolsList, nil
}

// UploadResource uploads a resource to the migration endpoint.
func (c *Client) UploadResource(ctx context.Context, modelUUID string, res resources.Resource, r io.ReadSeeker) error {
	args := makeResourceArgs(res)
	args.Add("application", res.ApplicationID)
	err := c.resourcePost(ctx, modelUUID, args, r)
	return errors.Trace(err)
}

// SetPlaceholderResource sets the metadata for a placeholder resource.
func (c *Client) SetPlaceholderResource(ctx context.Context, modelUUID string, res resources.Resource) error {
	args := makeResourceArgs(res)
	args.Add("application", res.ApplicationID)
	err := c.resourcePost(ctx, modelUUID, args, nil)
	return errors.Trace(err)
}

// SetUnitResource sets the metadata for a particular unit resource.
func (c *Client) SetUnitResource(ctx context.Context, modelUUID, unit string, res resources.Resource) error {
	args := makeResourceArgs(res)
	args.Add("unit", unit)
	err := c.resourcePost(ctx, modelUUID, args, nil)
	return errors.Trace(err)
}

func (c *Client) resourcePost(ctx context.Context, modelUUID string, args url.Values, r io.ReadSeeker) error {
	uri := "/migrate/resources?" + args.Encode()
	contentType := "application/octet-stream"
	err := c.httpPost(ctx, modelUUID, r, uri, contentType, nil, nil)
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

func (c *Client) httpPost(ctx context.Context, modelUUID string, content io.ReadSeeker, endpoint, contentType string, headers map[string]string, response interface{}) error {
	return c.http(ctx, "POST", modelUUID, content, endpoint, contentType, headers, response)
}

func (c *Client) httpPut(ctx context.Context, modelUUID string, content io.ReadSeeker, endpoint, contentType string, headers map[string]string, response interface{}) error {
	return c.http(ctx, "PUT", modelUUID, content, endpoint, contentType, headers, response)
}

func (c *Client) http(ctx context.Context, method, modelUUID string, content io.ReadSeeker, endpoint, contentType string, headers map[string]string, response interface{}) error {
	req, err := http.NewRequest(method, endpoint, content)
	if err != nil {
		return errors.Annotate(err, "cannot create upload request")
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set(params.MigrationModelHTTPHeader, modelUUID)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// The returned httpClient sets the base url to the controller api root
	httpClient, err := c.httpRootClientFactory()
	if err != nil {
		return errors.Trace(err)
	}

	return errors.Trace(httpClient.Do(ctx, req, response))
}

// OpenLogTransferStream connects to the migration logtransfer
// endpoint on the target controller and returns a stream that JSON
// logs records can be fed into. The objects written should be params.LogRecords.
func (c *Client) OpenLogTransferStream(ctx context.Context, modelUUID string) (base.Stream, error) {
	headers := http.Header{}
	headers.Set(params.MigrationModelHTTPHeader, modelUUID)
	caller := c.caller.RawAPICaller()
	stream, err := caller.ConnectControllerStream(ctx, "/migrate/logtransfer", url.Values{}, headers)
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
	err := c.caller.FacadeCall(context.TODO(), "LatestLogTime", args, &result)
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
	return errors.Trace(c.caller.FacadeCall(context.TODO(), "AdoptResources", args, nil))
}

// CACert returns the CA certificate associated with
// the connection.
func (c *Client) CACert() (string, error) {
	var result params.BytesResult
	err := c.caller.FacadeCall(context.TODO(), "CACert", nil, &result)
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
	err := c.caller.FacadeCall(context.TODO(), "CheckMachines", args, &result)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var results []error
	for _, res := range result.Results {
		results = append(results, errors.Errorf(res.Error.Message))
	}
	return results, nil
}
