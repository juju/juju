// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/status"
)

type statusFormatter struct {
	status    *params.FullStatus
	relations map[int]params.RelationStatus
	isoTime   bool
}

// NewStatusFormatter takes stored model information (params.FullStatus) and populates
// the statusFormatter struct used in various status formatting methods
func NewStatusFormatter(status *params.FullStatus, isoTime bool) *statusFormatter {
	sf := statusFormatter{
		status:    status,
		relations: make(map[int]params.RelationStatus),
		isoTime:   isoTime,
	}
	for _, relation := range status.Relations {
		sf.relations[relation.Id] = relation
	}
	return &sf
}

func (sf *statusFormatter) format() formattedStatus {
	if sf.status == nil {
		return formattedStatus{}
	}
	out := formattedStatus{
		Model:    sf.status.ModelName,
		Machines: make(map[string]machineStatus),
		Services: make(map[string]serviceStatus),
	}
	if sf.status.AvailableVersion != "" {
		out.ModelStatus = &modelStatus{
			AvailableVersion: sf.status.AvailableVersion,
		}
	}

	for k, m := range sf.status.Machines {
		out.Machines[k] = sf.formatMachine(m)
	}
	for sn, s := range sf.status.Services {
		out.Services[sn] = sf.formatService(sn, s)
	}
	return out
}

// MachineFormat takes stored model information (params.FullStatus) and formats machine status info.
func (sf *statusFormatter) MachineFormat(machineId []string) formattedMachineStatus {
	if sf.status == nil {
		return formattedMachineStatus{}
	}
	out := formattedMachineStatus{
		Model:    sf.status.ModelName,
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

func (sf *statusFormatter) formatService(name string, service params.ServiceStatus) serviceStatus {
	out := serviceStatus{
		Err:           service.Err,
		Charm:         service.Charm,
		Exposed:       service.Exposed,
		Life:          service.Life,
		Relations:     service.Relations,
		CanUpgradeTo:  service.CanUpgradeTo,
		SubordinateTo: service.SubordinateTo,
		Units:         make(map[string]unitStatus),
		StatusInfo:    sf.getServiceStatusInfo(service),
	}
	for k, m := range service.Units {
		out.Units[k] = sf.formatUnit(unitFormatInfo{
			unit:          m,
			unitName:      k,
			serviceName:   name,
			meterStatuses: service.MeterStatuses,
		})
	}
	return out
}

func (sf *statusFormatter) getServiceStatusInfo(service params.ServiceStatus) statusInfoContents {
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
	unit          params.UnitStatus
	unitName      string
	serviceName   string
	meterStatuses map[string]params.MeterStatus
}

func (sf *statusFormatter) formatUnit(info unitFormatInfo) unitStatus {
	// TODO(Wallyworld) - this should be server side but we still need to support older servers.
	sf.updateUnitStatusInfo(&info.unit, info.serviceName)

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
			unit:          m,
			unitName:      k,
			serviceName:   info.serviceName,
			meterStatuses: info.meterStatuses,
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

func (sf *statusFormatter) updateUnitStatusInfo(unit *params.UnitStatus, serviceName string) {
	// TODO(perrito66) add status validation.
	if status.Status(unit.WorkloadStatus.Status) == status.StatusError {
		if relation, ok := sf.relations[getRelationIdFromData(unit)]; ok {
			// Append the details of the other endpoint on to the status info string.
			if ep, ok := findOtherEndpoint(relation.Endpoints, serviceName); ok {
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
// that *doesn't* match serviceName. The returned bool indicates if
// such an endpoint was found.
func findOtherEndpoint(endpoints []params.EndpointStatus, serviceName string) (params.EndpointStatus, bool) {
	for _, endpoint := range endpoints {
		if endpoint.ServiceName != serviceName {
			return endpoint, true
		}
	}
	return params.EndpointStatus{}, false
}
