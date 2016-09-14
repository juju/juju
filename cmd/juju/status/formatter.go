// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"strings"

	"github.com/juju/utils/series"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/status"
)

type statusFormatter struct {
	status         *params.FullStatus
	controllerName string
	relations      map[int]params.RelationStatus
	isoTime        bool
}

// NewStatusFormatter takes stored model information (params.FullStatus) and populates
// the statusFormatter struct used in various status formatting methods
func NewStatusFormatter(status *params.FullStatus, isoTime bool) *statusFormatter {
	return newStatusFormatter(status, "", isoTime)
}

func newStatusFormatter(status *params.FullStatus, controllerName string, isoTime bool) *statusFormatter {
	sf := statusFormatter{
		status:         status,
		controllerName: controllerName,
		relations:      make(map[int]params.RelationStatus),
		isoTime:        isoTime,
	}
	for _, relation := range status.Relations {
		sf.relations[relation.Id] = relation
	}
	return &sf
}

func (sf *statusFormatter) format() (formattedStatus, error) {
	if sf.status == nil {
		return formattedStatus{}, nil
	}
	cloudTag, err := names.ParseCloudTag(sf.status.Model.CloudTag)
	if err != nil {
		return formattedStatus{}, err
	}
	out := formattedStatus{
		Model: modelStatus{
			Name:             sf.status.Model.Name,
			Controller:       sf.controllerName,
			Cloud:            cloudTag.Id(),
			CloudRegion:      sf.status.Model.CloudRegion,
			Version:          sf.status.Model.Version,
			AvailableVersion: sf.status.Model.AvailableVersion,
			Migration:        sf.status.Model.Migration,
		},
		Machines:     make(map[string]machineStatus),
		Applications: make(map[string]applicationStatus),
	}
	for k, m := range sf.status.Machines {
		out.Machines[k] = sf.formatMachine(m)
	}
	for sn, s := range sf.status.Applications {
		out.Applications[sn] = sf.formatApplication(sn, s)
	}
	return out, nil
}

// MachineFormat takes stored model information (params.FullStatus) and formats machine status info.
func (sf *statusFormatter) MachineFormat(machineId []string) formattedMachineStatus {
	if sf.status == nil {
		return formattedMachineStatus{}
	}
	out := formattedMachineStatus{
		Model:    sf.status.Model.Name,
		Machines: make(map[string]machineStatus),
	}
	for k, m := range sf.status.Machines {
		if len(machineId) != 0 {
			for i := 0; i < len(machineId); i++ {
				if m.Id == machineId[i] {
					out.Machines[k] = sf.formatMachine(m)
				}
			}
		} else {
			out.Machines[k] = sf.formatMachine(m)
		}
	}
	return out
}

func (sf *statusFormatter) formatMachine(machine params.MachineStatus) machineStatus {
	var out machineStatus

	out = machineStatus{
		JujuStatus:    sf.getStatusInfoContents(machine.AgentStatus),
		DNSName:       machine.DNSName,
		InstanceId:    machine.InstanceId,
		MachineStatus: sf.getStatusInfoContents(machine.InstanceStatus),
		Series:        machine.Series,
		Id:            machine.Id,
		Containers:    make(map[string]machineStatus),
		Hardware:      machine.Hardware,
	}

	for k, m := range machine.Containers {
		out.Containers[k] = sf.formatMachine(m)
	}

	for _, job := range machine.Jobs {
		if job == multiwatcher.JobManageModel {
			out.HAStatus = makeHAStatus(machine.HasVote, machine.WantsVote)
			break
		}
	}
	return out
}

func (sf *statusFormatter) formatApplication(name string, application params.ApplicationStatus) applicationStatus {
	appOS, _ := series.GetOSFromSeries(application.Series)
	var (
		charmOrigin = ""
		charmName   = ""
		charmRev    = 0
	)
	if curl, err := charm.ParseURL(application.Charm); err != nil {
		// We should never fail to parse a charm url sent back
		// but if we do, don't crash.
		logger.Errorf("failed to parse charm: %v", err)
	} else {
		switch curl.Schema {
		case "cs":
			charmOrigin = "jujucharms"
		case "local":
			charmOrigin = "local"
		default:
			charmOrigin = "unknown"
		}
		charmName = curl.Name
		charmRev = curl.Revision
	}

	out := applicationStatus{
		Err:           application.Err,
		Charm:         application.Charm,
		Series:        application.Series,
		OS:            strings.ToLower(appOS.String()),
		CharmOrigin:   charmOrigin,
		CharmName:     charmName,
		CharmRev:      charmRev,
		Exposed:       application.Exposed,
		Life:          application.Life,
		Relations:     application.Relations,
		CanUpgradeTo:  application.CanUpgradeTo,
		SubordinateTo: application.SubordinateTo,
		Units:         make(map[string]unitStatus),
		StatusInfo:    sf.getServiceStatusInfo(application),
		Version:       application.WorkloadVersion,
	}
	for k, m := range application.Units {
		out.Units[k] = sf.formatUnit(unitFormatInfo{
			unit:            m,
			unitName:        k,
			applicationName: name,
			meterStatuses:   application.MeterStatuses,
		})
	}
	return out
}

func (sf *statusFormatter) getServiceStatusInfo(service params.ApplicationStatus) statusInfoContents {
	// TODO(perrito66) add status validation.
	info := statusInfoContents{
		Err:     service.Status.Err,
		Current: status.Status(service.Status.Status),
		Message: service.Status.Info,
		Version: service.Status.Version,
	}
	if service.Status.Since != nil {
		info.Since = common.FormatTime(service.Status.Since, sf.isoTime)
	}
	return info
}

type unitFormatInfo struct {
	unit            params.UnitStatus
	unitName        string
	applicationName string
	meterStatuses   map[string]params.MeterStatus
}

func (sf *statusFormatter) formatUnit(info unitFormatInfo) unitStatus {
	// TODO(Wallyworld) - this should be server side but we still need to support older servers.
	sf.updateUnitStatusInfo(&info.unit, info.applicationName)

	out := unitStatus{
		WorkloadStatusInfo: sf.getWorkloadStatusInfo(info.unit),
		JujuStatusInfo:     sf.getAgentStatusInfo(info.unit),
		Machine:            info.unit.Machine,
		OpenedPorts:        info.unit.OpenedPorts,
		PublicAddress:      info.unit.PublicAddress,
		Charm:              info.unit.Charm,
		Subordinates:       make(map[string]unitStatus),
	}

	if ms, ok := info.meterStatuses[info.unitName]; ok {
		out.MeterStatus = &meterStatus{
			Color:   ms.Color,
			Message: ms.Message,
		}
	}

	for k, m := range info.unit.Subordinates {
		out.Subordinates[k] = sf.formatUnit(unitFormatInfo{
			unit:            m,
			unitName:        k,
			applicationName: info.applicationName,
			meterStatuses:   info.meterStatuses,
		})
	}
	return out
}

func (sf *statusFormatter) getStatusInfoContents(inst params.DetailedStatus) statusInfoContents {
	// TODO(perrito66) add status validation.
	info := statusInfoContents{
		Err:     inst.Err,
		Current: status.Status(inst.Status),
		Message: inst.Info,
		Version: inst.Version,
		Life:    inst.Life,
	}
	if inst.Since != nil {
		info.Since = common.FormatTime(inst.Since, sf.isoTime)
	}
	return info
}

func (sf *statusFormatter) getWorkloadStatusInfo(unit params.UnitStatus) statusInfoContents {
	// TODO(perrito66) add status validation.
	info := statusInfoContents{
		Err:     unit.WorkloadStatus.Err,
		Current: status.Status(unit.WorkloadStatus.Status),
		Message: unit.WorkloadStatus.Info,
		Version: unit.WorkloadStatus.Version,
	}
	if unit.WorkloadStatus.Since != nil {
		info.Since = common.FormatTime(unit.WorkloadStatus.Since, sf.isoTime)
	}
	return info
}

func (sf *statusFormatter) getAgentStatusInfo(unit params.UnitStatus) statusInfoContents {
	// TODO(perrito66) add status validation.
	info := statusInfoContents{
		Err:     unit.AgentStatus.Err,
		Current: status.Status(unit.AgentStatus.Status),
		Message: unit.AgentStatus.Info,
		Version: unit.AgentStatus.Version,
	}
	if unit.AgentStatus.Since != nil {
		info.Since = common.FormatTime(unit.AgentStatus.Since, sf.isoTime)
	}
	return info
}

func (sf *statusFormatter) updateUnitStatusInfo(unit *params.UnitStatus, applicationName string) {
	// TODO(perrito66) add status validation.
	if status.Status(unit.WorkloadStatus.Status) == status.Error {
		if relation, ok := sf.relations[getRelationIdFromData(unit)]; ok {
			// Append the details of the other endpoint on to the status info string.
			if ep, ok := findOtherEndpoint(relation.Endpoints, applicationName); ok {
				unit.WorkloadStatus.Info = unit.WorkloadStatus.Info + " for " + ep.String()
			}
		}
	}
}

func makeHAStatus(hasVote, wantsVote bool) string {
	var s string
	switch {
	case hasVote && wantsVote:
		s = "has-vote"
	case hasVote && !wantsVote:
		s = "removing-vote"
	case !hasVote && wantsVote:
		s = "adding-vote"
	case !hasVote && !wantsVote:
		s = "no-vote"
	}
	return s
}

func getRelationIdFromData(unit *params.UnitStatus) int {
	if relationId_, ok := unit.WorkloadStatus.Data["relation-id"]; ok {
		if relationId, ok := relationId_.(float64); ok {
			return int(relationId)
		} else {
			logger.Infof("relation-id found status data but was unexpected "+
				"type: %q. Status output may be lacking some detail.", relationId_)
		}
	}
	return -1
}

// findOtherEndpoint searches the provided endpoints for an endpoint
// that *doesn't* match applicationName. The returned bool indicates if
// such an endpoint was found.
func findOtherEndpoint(endpoints []params.EndpointStatus, applicationName string) (params.EndpointStatus, bool) {
	for _, endpoint := range endpoints {
		if endpoint.ApplicationName != applicationName {
			return endpoint, true
		}
	}
	return params.EndpointStatus{}, false
}
