// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package observer

import (
	"context"
	"time"

	"github.com/juju/names/v6"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/rpc"
)

// ApplicationService provides methods to interact with the application
// model.
type StatusService interface {
	// SetUnitPresence marks the presence of the unit in the model and returns
	// the time recorded for it. It is called when the unit agent logs in to
	// the API server. The unit life is not considered when setting the
	// presence.
	SetUnitPresence(ctx context.Context, unitName unit.Name) (time.Time, error)

	// DeleteUnitPresenceIfLastSeen removes the presence of the unit in the
	// model only if it is still the presence recorded at lastSeen. If the unit
	// is not found, it ignores the error. The unit life is not considered when
	// deleting the presence.
	DeleteUnitPresenceIfLastSeen(ctx context.Context, unitName unit.Name, lastSeen time.Time) error

	// SetMachinePresence marks the presence of the machine in the model and
	// returns the time recorded for it. It is called when the machine agent
	// logs in to the API server. The machine life is not considered when
	// setting the presence.
	SetMachinePresence(ctx context.Context, machineName machine.Name) (time.Time, error)

	// DeleteMachinePresenceIfLastSeen removes the presence of the machine in
	// the model only if it is still the presence recorded at lastSeen. If the
	// machine is not found, it ignores the error. The machine life is not
	// considered when deleting the presence.
	DeleteMachinePresenceIfLastSeen(ctx context.Context, machineName machine.Name, lastSeen time.Time) error
}

// ModelService provides methods to interact with the model.
type ModelService interface {
	StatusService() StatusService
}

// DomainServicesGetter is the service getter to use to get domain services.
type DomainServicesGetter interface {
	// ServicesForModel returns the services factory for a given model
	// uuid.
	ServicesForModel(context.Context, model.UUID) (ModelService, error)
}

// AgentPresenceConfig provides information needed for a
// AgentPresence to operate correctly.
type AgentPresenceConfig struct {
	// DomainServicesGetter is the service getter to use to get domain services.
	DomainServicesGetter DomainServicesGetter

	// Logger is the log to use to write log statements.
	Logger logger.Logger
}

// AgentPresence serves as a sink for API server requests and
// responses.
type AgentPresence struct {
	BaseObserver

	serviceGetter DomainServicesGetter
	logger        logger.Logger

	modelService ModelService

	// lastSeen is the presence time recorded when this connection logged in.
	// An agent can reconnect, to this or another controller, before this
	// connection is closed. Leave only removes the presence if it still has
	// this time, so it does not remove the presence of the newer connection.
	// It is zero if no presence was recorded.
	lastSeen time.Time
}

// NewAgentPresence returns a new RPCObserver.
func NewAgentPresence(cfg AgentPresenceConfig) *AgentPresence {
	// Ideally we should have a logging context so we can log into the correct
	// model rather than the api server for everything.
	return &AgentPresence{
		serviceGetter: cfg.DomainServicesGetter,
		logger:        cfg.Logger,
	}
}

// Login writes the agent presence to the database based on the entity type.
// Units and machines are the only entities that can have presence.
func (n *AgentPresence) Login(ctx context.Context, entity names.Tag, modelTag names.ModelTag, modelUUID model.UUID, fromController bool, userData string) {
	n.BaseObserver.Login(ctx, entity, modelTag, modelUUID, fromController, userData)

	// Controller connections to workload models must not update presence for
	// workload agents with the same tag.
	if !n.IsAgent() || n.FromController() {
		return
	}

	services, err := n.serviceGetter.ServicesForModel(ctx, modelUUID)
	if err != nil {
		n.logger.Infof(ctx, "recording presence for agent %s: unable to get domain services model: %s %v", entity, modelTag, err)
		return
	}

	n.modelService = services
	statusService := n.modelService.StatusService()

	switch t := entity.(type) {
	case names.UnitTag:
		lastSeen, err := statusService.SetUnitPresence(ctx, unit.Name(t.Id()))
		if err != nil {
			n.logger.Infof(ctx, "recording presence for agent %s: unable to set unit presence: %v", t, err)
			return
		}
		n.lastSeen = lastSeen
	case names.MachineTag:
		lastSeen, err := statusService.SetMachinePresence(ctx, machine.Name(t.Id()))
		if err != nil {
			n.logger.Infof(ctx, "recording presence for agent %s: unable to set machine presence: %v", t, err)
			return
		}
		n.lastSeen = lastSeen
	}
}

// Leave removes the agent presence to the database based on the entity type.
// Units and machines are the only entities that can have presence.
func (n *AgentPresence) Leave(ctx context.Context) {
	// This guards against the case where the agent has not logged in and
	// the agent tag is nil.
	// Controller connections must not remove workload agent presence.
	if !n.IsAgent() || n.FromController() {
		return
	}

	// Nothing to remove if this connection did not record presence.
	if n.modelService == nil || n.lastSeen.IsZero() {
		return
	}

	statusService := n.modelService.StatusService()

	switch t := n.AgentTag().(type) {
	case names.UnitTag:
		err := statusService.DeleteUnitPresenceIfLastSeen(ctx, unit.Name(t.Id()), n.lastSeen)
		if err != nil {
			n.logger.Infof(ctx, "recording presence for agent %s: unable to delete unit presence: %v", t, err)
		}
	case names.MachineTag:
		if err := statusService.DeleteMachinePresenceIfLastSeen(ctx, machine.Name(t.Id()), n.lastSeen); err != nil {
			n.logger.Infof(ctx, "recording presence for agent %s: unable to delete machine presence: %v", t, err)
		}
	}
}

// RPCObserver returns an rpc.Observer for the agent presence that doesn't
// do anything.
func (n *AgentPresence) RPCObserver() rpc.Observer {
	return nil
}
