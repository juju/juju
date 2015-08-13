// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"fmt"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/state/multiwatcher"
)

type statusFormatter struct {
	status        *params.FullStatus
	relations     map[int]params.RelationStatus
	isoTime       bool
	compatVersion int
}

func newStatusFormatter(status *params.FullStatus, compatVersion int, isoTime bool) *statusFormatter {
	sf := statusFormatter{
		status:        status,
		relations:     make(map[int]params.RelationStatus),
		compatVersion: compatVersion,
		isoTime:       isoTime,
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
		Environment: sf.status.EnvironmentName,
		Machines:    make(map[string]machineStatus),
		Services:    make(map[string]serviceStatus),
	}
	for k, m := range sf.status.Machines {
		out.Machines[k] = sf.formatMachine(m)
	}
	for sn, s := range sf.status.Services {
		out.Services[sn] = sf.formatService(sn, s)
	}
	for k, n := range sf.status.Networks {
		if out.Networks == nil {
			out.Networks = make(map[string]networkStatus)
		}
		out.Networks[k] = sf.formatNetwork(n)
	}
	return out
}

func (sf *statusFormatter) formatMachine(machine params.MachineStatus) machineStatus {
	var out machineStatus

	if machine.Agent.Status == "" {
		// Older server
		// TODO: this will go away at some point (v1.21?).
		out = machineStatus{
			AgentState:     machine.AgentState,
			AgentStateInfo: machine.AgentStateInfo,
			AgentVersion:   machine.AgentVersion,
			Life:           machine.Life,
			Err:            machine.Err,
			DNSName:        machine.DNSName,
			InstanceId:     machine.InstanceId,
			InstanceState:  machine.InstanceState,
			Series:         machine.Series,
			Id:             machine.Id,
			Containers:     make(map[string]machineStatus),
			Hardware:       machine.Hardware,
		}
	} else {
		// New server
		agent := machine.Agent
		out = machineStatus{
			AgentState:     machine.AgentState,
			AgentStateInfo: adjustInfoIfMachineAgentDown(machine.AgentState, agent.Status, agent.Info),
			AgentVersion:   agent.Version,
			Life:           agent.Life,
			Err:            agent.Err,
			DNSName:        machine.DNSName,
			InstanceId:     machine.InstanceId,
			InstanceState:  machine.InstanceState,
			Series:         machine.Series,
			Id:             machine.Id,
			Containers:     make(map[string]machineStatus),
			Hardware:       machine.Hardware,
		}
	}

	for k, m := range machine.Containers {
		out.Containers[k] = sf.formatMachine(m)
	}

	for _, job := range machine.Jobs {
		if job == multiwatcher.JobManageEnviron {
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
		Networks:      make(map[string][]string),
		CanUpgradeTo:  service.CanUpgradeTo,
		SubordinateTo: service.SubordinateTo,
		Units:         make(map[string]unitStatus),
		StatusInfo:    sf.getServiceStatusInfo(service),
	}
	if len(service.Networks.Enabled) > 0 {
		out.Networks["enabled"] = service.Networks.Enabled
	}
	if len(service.Networks.Disabled) > 0 {
		out.Networks["disabled"] = service.Networks.Disabled
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
	info := statusInfoContents{
		Err:     service.Status.Err,
		Current: service.Status.Status,
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
		AgentStatusInfo:    sf.getAgentStatusInfo(info.unit),
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

	// These legacy fields will be dropped for Juju 2.0.
	if sf.compatVersion < 2 || out.AgentStatusInfo.Current == "" {
		out.Err = info.unit.Err
		out.AgentState = info.unit.AgentState
		out.AgentStateInfo = info.unit.AgentStateInfo
		out.Life = info.unit.Life
		out.AgentVersion = info.unit.AgentVersion
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

func (sf *statusFormatter) getWorkloadStatusInfo(unit params.UnitStatus) statusInfoContents {
	info := statusInfoContents{
		Err:     unit.Workload.Err,
		Current: unit.Workload.Status,
		Message: unit.Workload.Info,
		Version: unit.Workload.Version,
	}
	if unit.Workload.Since != nil {
		info.Since = common.FormatTime(unit.Workload.Since, sf.isoTime)
	}
	return info
}

func (sf *statusFormatter) getAgentStatusInfo(unit params.UnitStatus) statusInfoContents {
	info := statusInfoContents{
		Err:     unit.UnitAgent.Err,
		Current: unit.UnitAgent.Status,
		Message: unit.UnitAgent.Info,
		Version: unit.UnitAgent.Version,
	}
	if unit.UnitAgent.Since != nil {
		info.Since = common.FormatTime(unit.UnitAgent.Since, sf.isoTime)
	}
	return info
}

func (sf *statusFormatter) updateUnitStatusInfo(unit *params.UnitStatus, serviceName string) {
	// This logic has no business here but can't be moved until Juju 2.0.
	statusInfo := unit.Workload.Info
	if unit.Workload.Status == "" {
		// Old server that doesn't support this field and others.
		// Just use the info string as-is.
		statusInfo = unit.AgentStateInfo
	}
	if unit.Workload.Status == params.StatusError {
		if relation, ok := sf.relations[getRelationIdFromData(unit)]; ok {
			// Append the details of the other endpoint on to the status info string.
			if ep, ok := findOtherEndpoint(relation.Endpoints, serviceName); ok {
				unit.Workload.Info = statusInfo + " for " + ep.String()
				unit.AgentStateInfo = unit.Workload.Info
			}
		}
	}
}

func (sf *statusFormatter) formatNetwork(network params.NetworkStatus) networkStatus {
	return networkStatus{
		Err:        network.Err,
		ProviderId: network.ProviderId,
		CIDR:       network.CIDR,
		VLANTag:    network.VLANTag,
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
	if relationId_, ok := unit.Workload.Data["relation-id"]; ok {
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

// adjustInfoIfMachineAgentDown modifies the agent status info string if the
// agent is down. The original status and info is included in
// parentheses.
func adjustInfoIfMachineAgentDown(status, origStatus params.Status, info string) string {
	if status == params.StatusDown {
		if info == "" {
			return fmt.Sprintf("(%s)", origStatus)
		}
		return fmt.Sprintf("(%s: %s)", origStatus, info)
	}
	return info
}
