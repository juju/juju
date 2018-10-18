// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"fmt"
	"strings"

	"github.com/juju/os"
	"github.com/juju/os/series"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state/multiwatcher"
)

type statusFormatter struct {
	status         *params.FullStatus
	controllerName string
	relations      map[int]params.RelationStatus
	isoTime        bool
	showRelations  bool
}

// NewStatusFormatter takes stored model information (params.FullStatus) and populates
// the statusFormatter struct used in various status formatting methods
func NewStatusFormatter(status *params.FullStatus, isoTime bool) *statusFormatter {
	return newStatusFormatter(status, "", isoTime, true)
}

func newStatusFormatter(status *params.FullStatus, controllerName string, isoTime, showRelations bool) *statusFormatter {
	sf := statusFormatter{
		status:         status,
		controllerName: controllerName,
		relations:      make(map[int]params.RelationStatus),
		isoTime:        isoTime,
		showRelations:  showRelations,
	}
	if showRelations {
		for _, relation := range status.Relations {
			sf.relations[relation.Id] = relation
		}
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
			Type:             sf.status.Model.Type,
			Controller:       sf.controllerName,
			Cloud:            cloudTag.Id(),
			CloudRegion:      sf.status.Model.CloudRegion,
			Version:          sf.status.Model.Version,
			AvailableVersion: sf.status.Model.AvailableVersion,
			Status:           sf.getStatusInfoContents(sf.status.Model.ModelStatus),
			SLA:              sf.status.Model.SLA,
		},
		Machines:           make(map[string]machineStatus),
		Applications:       make(map[string]applicationStatus),
		RemoteApplications: make(map[string]remoteApplicationStatus),
		Offers:             make(map[string]offerStatus),
		Relations:          make([]relationStatus, len(sf.relations)),
	}
	if sf.status.Model.MeterStatus.Color != "" {
		out.Model.MeterStatus = &meterStatus{
			Color:   sf.status.Model.MeterStatus.Color,
			Message: sf.status.Model.MeterStatus.Message,
		}
	}
	if sf.status.ControllerTimestamp != nil {
		out.Controller = &controllerStatus{
			Timestamp: common.FormatTimeAsTimestamp(sf.status.ControllerTimestamp, sf.isoTime),
		}
	}
	for k, m := range sf.status.Machines {
		out.Machines[k] = sf.formatMachine(m)
	}
	for sn, s := range sf.status.Applications {
		out.Applications[sn] = sf.formatApplication(sn, s)
	}
	for sn, s := range sf.status.RemoteApplications {
		out.RemoteApplications[sn] = sf.formatRemoteApplication(sn, s)
	}
	for name, offer := range sf.status.Offers {
		out.Offers[name] = sf.formatOffer(name, offer)
	}
	i := 0
	for _, rel := range sf.relations {
		out.Relations[i] = sf.formatRelation(rel)
		i++
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
		JujuStatus:        sf.getStatusInfoContents(machine.AgentStatus),
		DNSName:           machine.DNSName,
		IPAddresses:       machine.IPAddresses,
		InstanceId:        machine.InstanceId,
		MachineStatus:     sf.getStatusInfoContents(machine.InstanceStatus),
		Series:            machine.Series,
		Id:                machine.Id,
		NetworkInterfaces: make(map[string]networkInterface),
		Containers:        make(map[string]machineStatus),
		Constraints:       machine.Constraints,
		Hardware:          machine.Hardware,
		LXDProfiles:       make(map[string]lxdProfileContents),
	}

	for k, d := range machine.NetworkInterfaces {
		out.NetworkInterfaces[k] = networkInterface{
			IPAddresses:    d.IPAddresses,
			MACAddress:     d.MACAddress,
			Gateway:        d.Gateway,
			DNSNameservers: d.DNSNameservers,
			Space:          d.Space,
			IsUp:           d.IsUp,
		}
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

	for k, v := range machine.LXDProfiles {
		out.LXDProfiles[k] = lxdProfileContents{
			Config:      v.Config,
			Description: v.Description,
			Devices:     v.Devices,
		}
	}

	return out
}

func (sf *statusFormatter) formatApplication(name string, application params.ApplicationStatus) applicationStatus {
	var osInfo string
	appOS, _ := series.GetOSFromSeries(application.Series)
	osInfo = strings.ToLower(appOS.String())

	// TODO(caas) - enhance GetOSFromSeries
	if appOS == os.Unknown && sf.status.Model.Type == "caas" {
		osInfo = application.Series
	}
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
		Err:              application.Err,
		Charm:            application.Charm,
		Series:           application.Series,
		OS:               osInfo,
		CharmOrigin:      charmOrigin,
		CharmName:        charmName,
		CharmRev:         charmRev,
		CharmVersion:     application.CharmVersion,
		Exposed:          application.Exposed,
		Life:             application.Life,
		Scale:            application.Scale,
		ProviderId:       application.ProviderId,
		Address:          application.PublicAddress,
		Relations:        application.Relations,
		CanUpgradeTo:     application.CanUpgradeTo,
		SubordinateTo:    application.SubordinateTo,
		Units:            make(map[string]unitStatus),
		StatusInfo:       sf.getApplicationStatusInfo(application),
		Version:          application.WorkloadVersion,
		EndpointBindings: application.EndpointBindings,
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

func (sf *statusFormatter) formatRemoteApplication(name string, application params.RemoteApplicationStatus) remoteApplicationStatus {
	out := remoteApplicationStatus{
		Err:        application.Err,
		OfferURL:   application.OfferURL,
		Life:       application.Life,
		Relations:  application.Relations,
		StatusInfo: sf.getRemoteApplicationStatusInfo(application),
	}
	out.Endpoints = make(map[string]remoteEndpoint)
	for _, ep := range application.Endpoints {
		out.Endpoints[ep.Name] = remoteEndpoint{
			Interface: ep.Interface,
			Role:      string(ep.Role),
		}
	}
	return out
}

func (sf *statusFormatter) formatRelation(rel params.RelationStatus) relationStatus {
	var provider, requirer params.EndpointStatus
	for _, ep := range rel.Endpoints {
		switch charm.RelationRole(ep.Role) {
		case charm.RolePeer:
			provider = ep
			requirer = ep
		case charm.RoleProvider:
			provider = ep
		case charm.RoleRequirer:
			requirer = ep
		}
	}
	var relType string
	switch {
	case rel.Scope == "container":
		relType = "subordinate"
	case provider.ApplicationName == requirer.ApplicationName:
		relType = "peer"
	default:
		relType = "regular"
	}
	out := relationStatus{
		Provider:  fmt.Sprintf("%s:%s", provider.ApplicationName, provider.Name),
		Requirer:  fmt.Sprintf("%s:%s", requirer.ApplicationName, requirer.Name),
		Interface: rel.Interface,
		Type:      relType,
		Status:    rel.Status.Status,
		Message:   rel.Status.Info,
	}
	return out
}

func (sf *statusFormatter) getApplicationStatusInfo(application params.ApplicationStatus) statusInfoContents {
	// TODO(perrito66) add status validation.
	info := statusInfoContents{
		Err:     application.Status.Err,
		Current: status.Status(application.Status.Status),
		Message: application.Status.Info,
		Version: application.Status.Version,
	}
	if application.Status.Since != nil {
		info.Since = common.FormatTime(application.Status.Since, sf.isoTime)
	}
	return info
}

func (sf *statusFormatter) getRemoteApplicationStatusInfo(application params.RemoteApplicationStatus) statusInfoContents {
	// TODO(perrito66) add status validation.
	info := statusInfoContents{
		Err:     application.Status.Err,
		Current: status.Status(application.Status.Status),
		Message: application.Status.Info,
		Version: application.Status.Version,
	}
	if application.Status.Since != nil {
		info.Since = common.FormatTime(application.Status.Since, sf.isoTime)
	}
	return info
}

func (sf *statusFormatter) formatOffer(name string, offer params.ApplicationOfferStatus) offerStatus {
	out := offerStatus{
		Err:                  offer.Err,
		ApplicationName:      offer.ApplicationName,
		CharmURL:             offer.CharmURL,
		ActiveConnectedCount: offer.ActiveConnectedCount,
		TotalConnectedCount:  offer.TotalConnectedCount,
	}
	out.Endpoints = make(map[string]remoteEndpoint)
	for alias, ep := range offer.Endpoints {
		out.Endpoints[alias] = remoteEndpoint{
			Name:      ep.Name,
			Interface: ep.Interface,
			Role:      string(ep.Role),
		}
	}
	return out
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
		ProviderId:         info.unit.ProviderId,
		Address:            info.unit.Address,
		PublicAddress:      info.unit.PublicAddress,
		Charm:              info.unit.Charm,
		Subordinates:       make(map[string]unitStatus),
		Leader:             info.unit.Leader,
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
	if unit.WorkloadStatus.Status == "" {
		return statusInfoContents{}
	}
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
