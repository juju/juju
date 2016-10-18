// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
	"encoding/json"

	"github.com/juju/errors"
	"github.com/juju/version"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/watcher"
)

// NewWatcherFunc exists to let us unit test Facade without patching.
type NewWatcherFunc func(base.APICaller, params.NotifyWatchResult) watcher.NotifyWatcher

// NewClient returns a new Client based on an existing API connection.
func NewClient(caller base.APICaller, newWatcher NewWatcherFunc) *Client {
	return &Client{
		caller:     base.NewFacadeCaller(caller, "MigrationMaster"),
		newWatcher: newWatcher,
	}
}

// Client describes the client side API for the MigrationMaster facade
// (used by the migrationmaster worker).
type Client struct {
	caller     base.FacadeCaller
	newWatcher NewWatcherFunc
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
		ExternalControl:  status.Spec.ExternalControl,
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
		UUID:         info.UUID,
		Name:         info.Name,
		Owner:        owner,
		AgentVersion: info.AgentVersion,
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
	var serialized params.SerializedModel
	err := c.caller.FacadeCall("Export", nil, &serialized)
	if err != nil {
		return migration.SerializedModel{}, err
	}

	// Convert tools info to output map.
	tools := make(map[version.Binary]string)
	for _, toolsInfo := range serialized.Tools {
		v, err := version.ParseBinary(toolsInfo.Version)
		if err != nil {
			return migration.SerializedModel{}, errors.Annotate(err, "error parsing tools version")
		}
		tools[v] = toolsInfo.URI
	}

	return migration.SerializedModel{
		Bytes:  serialized.Bytes,
		Charms: serialized.Charms,
		Tools:  tools,
	}, nil
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

	out.SomeUnknownMachines, out.SomeUnknownUnits, err = groupTagIds(in.UnknownSample)
	if err != nil {
		return out, errors.Annotate(err, "processing unknown agents")
	}

	out.FailedMachines, out.FailedUnits, err = groupTagIds(in.Failed)
	if err != nil {
		return out, errors.Annotate(err, "processing failed agents")
	}

	return out, nil
}

func groupTagIds(tagStrs []string) ([]string, []string, error) {
	var machines []string
	var units []string

	for i := 0; i < len(tagStrs); i++ {
		tag, err := names.ParseTag(tagStrs[i])
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
		switch t := tag.(type) {
		case names.MachineTag:
			machines = append(machines, t.Id())
		case names.UnitTag:
			units = append(units, t.Id())
		default:
			return nil, nil, errors.Errorf("unsupported tag: %q", tag)
		}
	}
	return machines, units, nil
}
