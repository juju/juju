// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/juju/description/v9"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"gopkg.in/httprequest.v1"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/resource"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/watcher"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/rpc/params"
)

// Option is a function that can be used to configure a Client.
type Option = base.Option

// WithTracer returns an Option that configures the Client to use the
// supplied tracer.
var WithTracer = base.WithTracer

// NewWatcherFunc exists to let us unit test Facade without patching.
type NewWatcherFunc func(base.APICaller, params.NotifyWatchResult) watcher.NotifyWatcher

// NewClient returns a new Client based on an existing API connection.
func NewClient(caller base.APICaller, newWatcher NewWatcherFunc, options ...Option) *Client {
	return &Client{
		caller:            base.NewFacadeCaller(caller, "MigrationMaster", options...),
		newWatcher:        newWatcher,
		httpClientFactory: caller.HTTPClient,
	}
}

// Client describes the client side API for the MigrationMaster facade
// (used by the migrationmaster worker).
type Client struct {
	caller            base.FacadeCaller
	newWatcher        NewWatcherFunc
	httpClientFactory func() (*httprequest.Client, error)
}

// Watch returns a watcher which reports when a migration is active
// for the model associated with the API connection.
func (c *Client) Watch(ctx context.Context) (watcher.NotifyWatcher, error) {
	var result params.NotifyWatchResult
	err := c.caller.FacadeCall(ctx, "Watch", nil, &result)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if result.Error != nil {
		return nil, result.Error
	}
	return c.newWatcher(c.caller.RawAPICaller(), result), nil
}

// MigrationStatus returns the details and progress of the latest
// model migration.
func (c *Client) MigrationStatus(ctx context.Context) (migration.MigrationStatus, error) {
	var empty migration.MigrationStatus
	var status params.MasterMigrationStatus
	err := c.caller.FacadeCall(ctx, "MigrationStatus", nil, &status)
	if err != nil {
		return empty, errors.Trace(err)
	}

	modelTag, err := names.ParseModelTag(status.Spec.ModelTag)
	if err != nil {
		return empty, errors.Annotatef(err, "parsing model tag")
	}

	phase, ok := migration.ParsePhase(status.Phase)
	if !ok {
		return empty, errors.New("unable to parse phase")
	}

	target := status.Spec.TargetInfo
	controllerTag, err := names.ParseControllerTag(target.ControllerTag)
	if err != nil {
		return empty, errors.Annotatef(err, "parsing controller tag")
	}

	authTag, err := names.ParseUserTag(target.AuthTag)
	if err != nil {
		return empty, errors.Annotatef(err, "unable to parse auth tag")
	}

	var macs []macaroon.Slice
	if target.Macaroons != "" {
		if err := json.Unmarshal([]byte(target.Macaroons), &macs); err != nil {
			return empty, errors.Annotatef(err, "unmarshalling macaroon")
		}
	}

	return migration.MigrationStatus{
		MigrationId:      status.MigrationId,
		ModelUUID:        modelTag.Id(),
		Phase:            phase,
		PhaseChangedTime: status.PhaseChangedTime,
		TargetInfo: migration.TargetInfo{
			ControllerTag: controllerTag,
			Addrs:         target.Addrs,
			CACert:        target.CACert,
			AuthTag:       authTag,
			Password:      target.Password,
			Macaroons:     macs,
		},
	}, nil
}

// SetPhase updates the phase of the currently active model migration.
func (c *Client) SetPhase(ctx context.Context, phase migration.Phase) error {
	args := params.SetMigrationPhaseArgs{
		Phase: phase.String(),
	}
	return c.caller.FacadeCall(ctx, "SetPhase", args, nil)
}

// SetStatusMessage sets a human readable message regarding the
// progress of a migration.
func (c *Client) SetStatusMessage(ctx context.Context, message string) error {
	args := params.SetMigrationStatusMessageArgs{
		Message: message,
	}
	return c.caller.FacadeCall(ctx, "SetStatusMessage", args, nil)
}

// ModelInfo return basic information about the model to migrated.
func (c *Client) ModelInfo(ctx context.Context) (migration.ModelInfo, error) {
	if c.caller.BestAPIVersion() < 5 {
		return c.modelInfoCompat(ctx)
	}
	var info params.MigrationModelInfo
	err := c.caller.FacadeCall(ctx, "ModelInfo", nil, &info)
	if err != nil {
		return migration.ModelInfo{}, errors.Trace(err)
	}

	// The model description is marshalled into YAML (description package does
	// not support JSON) to prevent potential issues with
	// marshalling/unmarshalling on the target API controller.
	var modelDescription description.Model
	if bytes := info.ModelDescription; len(bytes) > 0 {
		var err error
		modelDescription, err = description.Deserialize(info.ModelDescription)
		if err != nil {
			return migration.ModelInfo{}, errors.Annotate(err, "failed to marshal model description")
		}
	}

	return migration.ModelInfo{
		UUID:                   info.UUID,
		Name:                   info.Name,
		Qualifier:              info.Qualifier,
		AgentVersion:           info.AgentVersion,
		ControllerAgentVersion: info.ControllerAgentVersion,
		ModelDescription:       modelDescription,
	}, nil
}

// SourceControllerInfo returns connection information about the source controller
// and uuids of any other hosted models involved in cross model relations.
func (c *Client) SourceControllerInfo(ctx context.Context) (migration.SourceControllerInfo, []string, error) {
	var info params.MigrationSourceInfo
	err := c.caller.FacadeCall(ctx, "SourceControllerInfo", nil, &info)
	if err != nil {
		return migration.SourceControllerInfo{}, nil, errors.Trace(err)
	}
	sourceTag, err := names.ParseControllerTag(info.ControllerTag)
	if err != nil {
		return migration.SourceControllerInfo{}, nil, errors.Trace(err)
	}
	return migration.SourceControllerInfo{
		ControllerTag:   sourceTag,
		ControllerAlias: info.ControllerAlias,
		Addrs:           info.Addrs,
		CACert:          info.CACert,
	}, info.LocalRelatedModels, nil
}

// Prechecks verifies that the source controller and model are healthy
// and able to participate in a migration.
func (c *Client) Prechecks(ctx context.Context) error {
	return c.caller.FacadeCall(ctx, "Prechecks", params.PrechecksArgs{}, nil)
}

// Export returns a serialized representation of the model associated
// with the API connection. The charms used by the model are also
// returned.
func (c *Client) Export(ctx context.Context) (migration.SerializedModel, error) {
	var empty migration.SerializedModel
	var serialized params.SerializedModel
	err := c.caller.FacadeCall(ctx, "Export", nil, &serialized)
	if err != nil {
		return empty, errors.Trace(err)
	}

	// Convert tools info to output map.
	tools := make(map[string]semversion.Binary, len(serialized.Tools))
	for _, toolsInfo := range serialized.Tools {
		v, err := semversion.ParseBinary(toolsInfo.Version)
		if err != nil {
			return migration.SerializedModel{}, errors.Annotate(err, "error parsing agent binary version")
		}
		tools[toolsInfo.SHA256] = v
	}

	resources, err := convertResources(serialized.Resources)
	if err != nil {
		return empty, errors.Trace(err)
	}

	return migration.SerializedModel{
		Bytes:     serialized.Bytes,
		Charms:    serialized.Charms,
		Tools:     tools,
		Resources: resources,
	}, nil
}

// ProcessRelations runs a series of processes to ensure that the relations
// of a given model are correct after a migrated model.
func (c *Client) ProcessRelations(ctx context.Context, controllerAlias string) error {
	param := params.ProcessRelations{
		ControllerAlias: controllerAlias,
	}
	var result params.ErrorResult
	err := c.caller.FacadeCall(ctx, "ProcessRelations", param, &result)
	if err != nil {
		return errors.Trace(err)
	}
	if result.Error != nil {
		return errors.Trace(result.Error)
	}
	return nil
}

// OpenResource downloads the named resource for an application.
func (c *Client) OpenResource(ctx context.Context, application, name string) (io.ReadCloser, error) {
	httpClient, err := c.httpClientFactory()
	if err != nil {
		return nil, errors.Annotate(err, "unable to create HTTP client")
	}

	uri := fmt.Sprintf("/applications/%s/resources/%s", application, name)
	var resp *http.Response
	if err := httpClient.Get(
		ctx,
		uri, &resp); err != nil {
		return nil, errors.Annotate(err, "unable to retrieve resource")
	}
	return resp.Body, nil
}

// Reap removes the documents for the model associated with the API
// connection.
func (c *Client) Reap(ctx context.Context) error {
	return c.caller.FacadeCall(ctx, "Reap", nil, nil)
}

// WatchMinionReports returns a watcher which reports when a migration
// minion has made a report for the current migration phase.
func (c *Client) WatchMinionReports(ctx context.Context) (watcher.NotifyWatcher, error) {
	var result params.NotifyWatchResult
	err := c.caller.FacadeCall(ctx, "WatchMinionReports", nil, &result)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if result.Error != nil {
		return nil, result.Error
	}
	return c.newWatcher(c.caller.RawAPICaller(), result), nil
}

// MinionReports returns details of the reports made by migration
// minions to the controller for the current migration phase.
func (c *Client) MinionReports(ctx context.Context) (migration.MinionReports, error) {
	var in params.MinionReports
	var out migration.MinionReports

	err := c.caller.FacadeCall(ctx, "MinionReports", nil, &in)
	if err != nil {
		return out, errors.Trace(err)
	}

	out.MigrationId = in.MigrationId

	phase, ok := migration.ParsePhase(in.Phase)
	if !ok {
		return out, errors.Errorf("invalid phase: %q", in.Phase)
	}
	out.Phase = phase

	out.SuccessCount = in.SuccessCount
	out.UnknownCount = in.UnknownCount

	out.SomeUnknownMachines, out.SomeUnknownUnits, out.SomeUnknownApplications, err = groupTagIds(in.UnknownSample)
	if err != nil {
		return out, errors.Annotate(err, "processing unknown agents")
	}

	out.FailedMachines, out.FailedUnits, out.FailedApplications, err = groupTagIds(in.Failed)
	if err != nil {
		return out, errors.Annotate(err, "processing failed agents")
	}

	return out, nil
}

// MinionReportTimeout returns the maximum duration that the migration master
// worker should wait for minions to report on a migration phase.
func (c *Client) MinionReportTimeout(ctx context.Context) (time.Duration, error) {
	var timeout time.Duration

	var res params.StringResult
	err := c.caller.FacadeCall(ctx, "MinionReportTimeout", nil, &res)
	if err != nil {
		return timeout, errors.Trace(err)
	}

	if res.Error != nil {
		return timeout, res.Error
	}

	timeout, err = time.ParseDuration(res.Result)
	return timeout, errors.Trace(err)
}

// StreamModelLog takes a starting time and returns a channel that
// will yield the logs on or after that time - these are the logs that
// need to be transferred to the target after the migration is
// successful.
func (c *Client) StreamModelLog(ctx context.Context, start time.Time) (<-chan common.LogMessage, error) {
	return common.StreamDebugLog(ctx, c.caller.RawAPICaller(), common.DebugLogParams{
		Replay:    true,
		NoTail:    true,
		StartTime: start,
	})
}

func groupTagIds(tagStrs []string) ([]string, []string, []string, error) {
	var machines []string
	var units []string
	var applications []string

	for i := 0; i < len(tagStrs); i++ {
		tag, err := names.ParseTag(tagStrs[i])
		if err != nil {
			return nil, nil, nil, errors.Trace(err)
		}
		switch t := tag.(type) {
		case names.MachineTag:
			machines = append(machines, t.Id())
		case names.UnitTag:
			units = append(units, t.Id())
		case names.ApplicationTag:
			applications = append(applications, t.Id())
		default:
			return nil, nil, nil, errors.Errorf("unsupported tag: %q", tag)
		}
	}
	return machines, units, applications, nil
}

func convertResources(in []params.SerializedModelResource) ([]resource.Resource, error) {
	if len(in) == 0 {
		return nil, nil
	}
	out := make([]resource.Resource, 0, len(in))
	for _, resource := range in {
		outResource, err := convertResource(resource)
		if err != nil {
			return nil, errors.Trace(err)
		}
		out = append(out, outResource)
	}
	return out, nil
}

func convertResource(res params.SerializedModelResource) (resource.Resource, error) {
	var empty resource.Resource
	type_, err := charmresource.ParseType(res.Type)
	if err != nil {
		return empty, errors.Trace(err)
	}
	origin, err := charmresource.ParseOrigin(res.Origin)
	if err != nil {
		return empty, errors.Trace(err)
	}
	var fp charmresource.Fingerprint
	if res.FingerprintHex != "" {
		if fp, err = charmresource.ParseFingerprint(res.FingerprintHex); err != nil {
			return empty, errors.Annotate(err, "invalid fingerprint")
		}
	}
	return resource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name: res.Name,
				Type: type_,
			},
			Origin:      origin,
			Revision:    res.Revision,
			Size:        res.Size,
			Fingerprint: fp,
		},
		ApplicationName: res.Application,
		RetrievedBy:     res.Username,
		Timestamp:       res.Timestamp,
	}, nil
}
