// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"fmt"
	"time"

	"github.com/juju/charm/v12"
	"github.com/juju/collections/set"
	"github.com/juju/names/v5"

	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/juju/storage"
	corebase "github.com/juju/juju/core/base"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/naturalsort"
	"github.com/juju/juju/rpc/params"
)

type statusFormatter struct {
	status                 *params.FullStatus
	controllerName         string
	outputName             string
	relations              map[int]params.RelationStatus
	storage                *storage.CombinedStorage
	isoTime, showRelations bool

	// Ideally this map should not be here.  It is used to facilitate
	// getting an active branch ref number for a subordinate unit.
	// Additionally it is used to set the active Branch as we get it locally and not from the facade.
	formattedBranches map[string]branchStatus
	activeBranch      string
}

// NewStatusFormatterParams contains the parameters required
// to be formatted for CLI output.
type NewStatusFormatterParams struct {
	Storage        *storage.CombinedStorage
	Status         *params.FullStatus
	ControllerName string
	OutputName     string
	ActiveBranch   string
	ISOTime        bool
	ShowRelations  bool
}

// NewStatusFormatter returns a new status formatter used in various
// formatting methods.
func NewStatusFormatter(p NewStatusFormatterParams) *statusFormatter {
	sf := statusFormatter{
		storage:        p.Storage,
		status:         p.Status,
		controllerName: p.ControllerName,
		relations:      make(map[int]params.RelationStatus),
		isoTime:        p.ISOTime,
		showRelations:  p.ShowRelations,
		outputName:     p.OutputName,
		activeBranch:   p.ActiveBranch,
	}
	if p.ShowRelations {
		for _, relation := range p.Status.Relations {
			sf.relations[relation.Id] = relation
		}
	}
	return &sf
}

// Format returns the formatted model status.
func (sf *statusFormatter) Format() (formattedStatus, error) {
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
		Branches:           make(map[string]branchStatus),
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
	// format Branch status before Applications to create reference
	// numbers to be used by Units.  Sort here to give continuity to
	// branch ref numbers provided the map of active branches does not
	// change.
	i := 1
	for _, name := range naturalsort.Sort(stringKeysFromMap(sf.status.Branches)) {
		s := sf.status.Branches[name]
		isActiveBranch := name == sf.activeBranch
		out.Branches[name] = sf.formatBranch(i, s, isActiveBranch)
		i += 1
	}
	sf.formattedBranches = out.Branches
	for name, app := range sf.status.Applications {
		out.Applications[name] = sf.formatApplication(name, app)
	}
	for name, app := range sf.status.RemoteApplications {
		out.RemoteApplications[name] = sf.formatRemoteApplication(name, app)
	}
	for name, offer := range sf.status.Offers {
		out.Offers[name] = sf.formatOffer(name, offer)
	}
	i = 0
	for _, rel := range sf.relations {
		out.Relations[i] = sf.formatRelation(rel)
		i++
	}
	if sf.storage != nil {
		out.Storage = sf.storage
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

	var base *formattedBase
	if machine.Base.Channel != "" {
		channel, err := corebase.ParseChannel(machine.Base.Channel)
		if err == nil {
			base = &formattedBase{Name: machine.Base.Name, Channel: channel.DisplayString()}
		}
	}
	out = machineStatus{
		JujuStatus:         sf.getStatusInfoContents(machine.AgentStatus),
		Hostname:           machine.Hostname,
		DNSName:            machine.DNSName,
		IPAddresses:        machine.IPAddresses,
		InstanceId:         machine.InstanceId,
		DisplayName:        machine.DisplayName,
		MachineStatus:      sf.getStatusInfoContents(machine.InstanceStatus),
		ModificationStatus: sf.getStatusInfoContents(machine.ModificationStatus),
		Base:               base,
		Id:                 machine.Id,
		NetworkInterfaces:  make(map[string]networkInterface),
		Containers:         make(map[string]machineStatus),
		Constraints:        machine.Constraints,
		Hardware:           machine.Hardware,
		LXDProfiles:        make(map[string]lxdProfileContents),
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
		if job == coremodel.JobManageModel {
			out.HAStatus = makeHAStatus(machine.HasVote, machine.WantsVote)
			isPrimary := machine.PrimaryControllerMachine
			if isPrimary != nil {
				out.HAPrimary = *isPrimary
			}
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
	var (
		charmAlias  = ""
		charmOrigin = ""
		charmName   = ""
		charmRev    = 0
	)
	if curl, err := charm.ParseURL(application.Charm); err != nil {
		// We should never fail to parse a charm url sent back
		// but if we do, don't crash.
		logger.Errorf("failed to parse charm: %v", err)
	} else {
		switch {
		case charm.CharmHub.Matches(curl.Schema):
			charmOrigin = "charmhub"
			charmAlias = curl.Name
		case charm.Local.Matches(curl.Schema):
			charmOrigin = "local"
			charmAlias = application.Charm
		default:
			charmOrigin = "unknown"
			charmAlias = application.Charm
		}
		charmRev = curl.Revision
		charmName = curl.Name
	}

	var base *formattedBase
	channel, err := corebase.ParseChannel(application.Base.Channel)
	if err == nil {
		base = &formattedBase{Name: application.Base.Name, Channel: channel.DisplayString()}
	}
	out := applicationStatus{
		Err:              typedNilCheck(application.Err),
		Charm:            charmAlias,
		Base:             base,
		CharmOrigin:      charmOrigin,
		CharmName:        charmName,
		CharmRev:         charmRev,
		CharmVersion:     application.CharmVersion,
		CharmProfile:     application.CharmProfile,
		CharmChannel:     application.CharmChannel,
		Exposed:          application.Exposed,
		Life:             string(application.Life),
		Scale:            application.Scale,
		ProviderId:       application.ProviderId,
		Address:          application.PublicAddress,
		Relations:        sf.processApplicationRelations(name, application.Relations),
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
			branchRef:       sf.branchRefForUnit(k),
		})
	}

	return out
}

func (sf *statusFormatter) processApplicationRelations(appName string, rels map[string][]string) map[string][]applicationStatusRelation {
	out := make(map[string][]applicationStatusRelation)
	for relName, theOtherSideAppNames := range rels {
		out[relName] = []applicationStatusRelation{}
		for _, endpointAppName := range theOtherSideAppNames {
			relStatus := sf.findRelationStatus(appName, relName, endpointAppName)
			if relStatus == nil {
				continue
			}
			out[relName] = append(out[relName], applicationStatusRelation{
				RelatedApplicationName: endpointAppName,
				Interface:              relStatus.Interface,
				Scope:                  relStatus.Scope,
			})
		}
	}
	return out
}

func (sf *statusFormatter) findRelationStatus(appName, relName, theOtherSideAppName string) *params.RelationStatus {
	for _, rel := range sf.relations {
		if appName == theOtherSideAppName {
			// peer relation.
			if len(rel.Endpoints) != 1 {
				continue
			}
			ep := rel.Endpoints[0]
			if ep.Name != relName || ep.ApplicationName != appName {
				continue
			}
			return &rel
		} else {
			if endpointsMactch(appName, relName, theOtherSideAppName, rel.Endpoints) {
				return &rel
			}
		}
	}
	return nil
}

func endpointsMactch(appName, relName, theOtherSideAppName string, eps []params.EndpointStatus) (equal bool) {
	if len(eps) != 2 {
		return false
	}
	for idx, ep := range eps {
		if ep.ApplicationName == appName && ep.Name == relName {
			if idx == 0 {
				return eps[1].ApplicationName == theOtherSideAppName
			}
			return eps[0].ApplicationName == theOtherSideAppName
		}
	}
	return false
}

func (sf *statusFormatter) formatRemoteApplication(name string, application params.RemoteApplicationStatus) remoteApplicationStatus {
	out := remoteApplicationStatus{
		Err:        typedNilCheck(application.Err),
		OfferURL:   application.OfferURL,
		Life:       string(application.Life),
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

func typedNilCheck(err *params.Error) error {
	// When the api comes back over the wire, the error is a typed-nil.
	// Maning that when we check against nil, we see nil, but it isn't nil.
	if err == nil {
		return nil
	}
	return err
}

func (sf *statusFormatter) getApplicationStatusInfo(application params.ApplicationStatus) statusInfoContents {
	// TODO(perrito66) add status validation.
	info := statusInfoContents{
		Err:     typedNilCheck(application.Status.Err),
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
		Err:     typedNilCheck(application.Status.Err),
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
		Err:                  typedNilCheck(offer.Err),
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
	branchRef       string
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
		Branch:             info.branchRef,
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
			branchRef:       sf.branchRefForUnit(k),
		})
	}
	return out
}

func (sf *statusFormatter) getStatusInfoContents(inst params.DetailedStatus) statusInfoContents {
	// TODO(perrito66) add status validation.
	info := statusInfoContents{
		Err: typedNilCheck(inst.Err),
		// NOTE: why use a status.Status here, but a string for Life?
		Current: status.Status(inst.Status),
		Message: inst.Info,
		Reason:  common.ModelStatusReason(inst.Data),
		Version: inst.Version,
		Life:    string(inst.Life),
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
		Err:     typedNilCheck(unit.WorkloadStatus.Err),
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
		Err:     typedNilCheck(unit.AgentStatus.Err),
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
func (sf *statusFormatter) formatBranch(ref int, branch params.BranchStatus, isActiveBranch bool) branchStatus {
	created := time.Unix(branch.Created, 0)
	if sf.outputName == "tabular" {
		return branchStatus{
			Ref:       fmt.Sprintf("#%d", ref),
			Created:   common.UserFriendlyDuration(created, time.Now()),
			CreatedBy: branch.CreatedBy,
			Active:    isActiveBranch,
		}
	}
	return branchStatus{
		Created:   common.FormatTimeAsTimestamp(&created, sf.isoTime),
		CreatedBy: branch.CreatedBy,
		Active:    isActiveBranch,
	}
}

func (sf *statusFormatter) branchRefForUnit(unitName string) string {
	for branchName, bs := range sf.status.Branches {
		for _, units := range bs.AssignedUnits {
			unitSet := set.NewStrings(units...)
			if unitSet.Contains(unitName) {
				if sf.outputName == "tabular" {
					return sf.formattedBranches[branchName].Ref
				}
				return branchName
			}
		}
	}
	return ""
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
