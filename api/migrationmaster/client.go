// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/juju/errors"
	"github.com/juju/httprequest"
	"github.com/juju/version"
	charmresource "gopkg.in/juju/charm.v6/resource"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon.v2-unstable"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/resource"
)

// NewWatcherFunc exists to let us unit test Facade without patching.
type NewWatcherFunc func(base.APICaller, params.NotifyWatchResult) watcher.NotifyWatcher

// NewClient returns a new Client based on an existing API connection.
func NewClient(caller base.APICaller, newWatcher NewWatcherFunc) *Client {
	return &Client{
		caller:            base.NewFacadeCaller(caller, "MigrationMaster"),
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
func (c *Client) Watch() (watcher.NotifyWatcher, error) {
	var result params.NotifyWatchResult
	err := c.caller.FacadeCall("Watch", nil, &result)
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
func (c *Client) MigrationStatus() (migration.MigrationStatus, error) {
	var empty migration.MigrationStatus
	var status params.MasterMigrationStatus
	err := c.caller.FacadeCall("MigrationStatus", nil, &status)
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
func (c *Client) SetPhase(phase migration.Phase) error {
	args := params.SetMigrationPhaseArgs{
		Phase: phase.String(),
	}
	return c.caller.FacadeCall("SetPhase", args, nil)
}

// SetStatusMessage sets a human readable message regarding the
// progress of a migration.
func (c *Client) SetStatusMessage(message string) error {
	args := params.SetMigrationStatusMessageArgs{
		Message: message,
	}
	return c.caller.FacadeCall("SetStatusMessage", args, nil)
}

// ModelInfo return basic information about the model to migrated.
func (c *Client) ModelInfo() (migration.ModelInfo, error) {
	var info params.MigrationModelInfo
	err := c.caller.FacadeCall("ModelInfo", nil, &info)
	if err != nil {
		return migration.ModelInfo{}, errors.Trace(err)
	}
	owner, err := names.ParseUserTag(info.OwnerTag)
	if err != nil {
		return migration.ModelInfo{}, errors.Trace(err)
	}
	return migration.ModelInfo{
		UUID:                   info.UUID,
		Name:                   info.Name,
		Owner:                  owner,
		AgentVersion:           info.AgentVersion,
		ControllerAgentVersion: info.ControllerAgentVersion,
	}, nil
}

// Prechecks verifies that the source controller and model are healthy
// and able to participate in a migration.
func (c *Client) Prechecks() error {
	return c.caller.FacadeCall("Prechecks", nil, nil)
}

// Export returns a serialized representation of the model associated
// with the API connection. The charms used by the model are also
// returned.
func (c *Client) Export() (migration.SerializedModel, error) {
	var empty migration.SerializedModel
	var serialized params.SerializedModel
	err := c.caller.FacadeCall("Export", nil, &serialized)
	if err != nil {
		return empty, errors.Trace(err)
	}

	// Convert tools info to output map.
	tools := make(map[version.Binary]string)
	for _, toolsInfo := range serialized.Tools {
		v, err := version.ParseBinary(toolsInfo.Version)
		if err != nil {
			return migration.SerializedModel{}, errors.Annotate(err, "error parsing agent binary version")
		}
		tools[v] = toolsInfo.URI
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

// OpenResource downloads the named resource for an application.
func (c *Client) OpenResource(application, name string) (io.ReadCloser, error) {
	httpClient, err := c.httpClientFactory()
	if err != nil {
		return nil, errors.Annotate(err, "unable to create HTTP client")
	}

	uri := fmt.Sprintf("/applications/%s/resources/%s", application, name)
	var resp *http.Response
	if err := httpClient.Get(uri, &resp); err != nil {
		return nil, errors.Annotate(err, "unable to retrieve resource")
	}
	return resp.Body, nil
}

// Reap removes the documents for the model associated with the API
// connection.
func (c *Client) Reap() error {
	return c.caller.FacadeCall("Reap", nil, nil)
}

// WatchMinionReports returns a watcher which reports when a migration
// minion has made a report for the current migration phase.
func (c *Client) WatchMinionReports() (watcher.NotifyWatcher, error) {
	var result params.NotifyWatchResult
	err := c.caller.FacadeCall("WatchMinionReports", nil, &result)
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
func (c *Client) MinionReports() (migration.MinionReports, error) {
	var in params.MinionReports
	var out migration.MinionReports

	err := c.caller.FacadeCall("MinionReports", nil, &in)
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

// StreamModelLog takes a starting time and returns a channel that
// will yield the logs on or after that time - these are the logs that
// need to be transferred to the target after the migration is
// successful.
func (c *Client) StreamModelLog(start time.Time) (<-chan common.LogMessage, error) {
	return common.StreamDebugLog(c.caller.RawAPICaller(), common.DebugLogParams{
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

func convertResources(in []params.SerializedModelResource) ([]migration.SerializedModelResource, error) {
	if len(in) == 0 {
		return nil, nil
	}
	out := make([]migration.SerializedModelResource, 0, len(in))
	for _, resource := range in {
		outResource, err := convertAppResource(resource)
		if err != nil {
			return nil, errors.Trace(err)
		}
		out = append(out, outResource)
	}
	return out, nil
}

func convertAppResource(in params.SerializedModelResource) (migration.SerializedModelResource, error) {
	var empty migration.SerializedModelResource
	appRev, err := convertResourceRevision(in.Application, in.Name, in.ApplicationRevision)
	if err != nil {
		return empty, errors.Annotate(err, "application revision")
	}
	csRev, err := convertResourceRevision(in.Application, in.Name, in.CharmStoreRevision)
	if err != nil {
		return empty, errors.Annotate(err, "charmstore revision")
	}
	unitRevs := make(map[string]resource.Resource)
	for unitName, inUnitRev := range in.UnitRevisions {
		unitRev, err := convertResourceRevision(in.Application, in.Name, inUnitRev)
		if err != nil {
			return empty, errors.Annotate(err, "unit revision")
		}
		unitRevs[unitName] = unitRev
	}
	return migration.SerializedModelResource{
		ApplicationRevision: appRev,
		CharmStoreRevision:  csRev,
		UnitRevisions:       unitRevs,
	}, nil
}

func convertResourceRevision(app, name string, rev params.SerializedModelResourceRevision) (resource.Resource, error) {
	var empty resource.Resource
	type_, err := charmresource.ParseType(rev.Type)
	if err != nil {
		return empty, errors.Trace(err)
	}
	origin, err := charmresource.ParseOrigin(rev.Origin)
	if err != nil {
		return empty, errors.Trace(err)
	}
	var fp charmresource.Fingerprint
	if rev.FingerprintHex != "" {
		if fp, err = charmresource.ParseFingerprint(rev.FingerprintHex); err != nil {
			return empty, errors.Annotate(err, "invalid fingerprint")
		}
	}
	return resource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:        name,
				Type:        type_,
				Path:        rev.Path,
				Description: rev.Description,
			},
			Origin:      origin,
			Revision:    rev.Revision,
			Size:        rev.Size,
			Fingerprint: fp,
		},
		ApplicationID: app,
		Username:      rev.Username,
		Timestamp:     rev.Timestamp,
	}, nil
}
