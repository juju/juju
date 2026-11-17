// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package observer

import (
	"context"

	"github.com/juju/names/v6"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/status"
	"github.com/juju/juju/rpc"
)

// ApplicationService provides methods to interact with the application
// model.
type StatusService interface {
	// SetUnitPresence marks the presence of the unit in the model and returns
	// a token for this login. It is called when the unit agent logs in to the
	// API server. The unit life is not considered when setting the presence.
	SetUnitPresence(ctx context.Context, unitName unit.Name) (status.PresenceToken, error)

	// DeleteUnitPresenceWithToken removes the presence of the unit in the
	// model only if it was recorded by the login that got token. If the unit
	// is not found, it ignores the error. The unit life is not considered when
	// deleting the presence.
	DeleteUnitPresenceWithToken(ctx context.Context, unitName unit.Name, token status.PresenceToken) error

	// SetMachinePresence marks the presence of the machine in the model and
	// returns a token for this login. It is called when the machine agent logs
	// in to the API server. The machine life is not considered when setting
	// the presence.
	SetMachinePresence(ctx context.Context, machineName machine.Name) (status.PresenceToken, error)

	// DeleteMachinePresenceWithToken removes the presence of the machine in
	// the model only if it was recorded by the login that got token. If the
	// machine is not found, it ignores the error. The machine life is not
	// considered when deleting the presence.
	DeleteMachinePresenceWithToken(ctx context.Context, machineName machine.Name, token status.PresenceToken) error
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

	// token identifies the presence recorded when this connection logged in.
	// An agent can reconnect, to this or another controller, before this
	// connection is closed. Leave only removes the presence if it still has
	// this token, so it does not remove the presence of the newer connection.
	// It is empty if no presence was recorded.
	token status.PresenceToken
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
		token, err := statusService.SetUnitPresence(ctx, unit.Name(t.Id()))
		if err != nil {
			n.logger.Infof(ctx, "recording presence for agent %s: unable to set unit presence: %v", t, err)
			return
		}
		n.token = token
	case names.MachineTag:
		token, err := statusService.SetMachinePresence(ctx, machine.Name(t.Id()))
		if err != nil {
			n.logger.Infof(ctx, "recording presence for agent %s: unable to set machine presence: %v", t, err)
			return
		}
		n.token = token
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
	if n.modelService == nil || n.token.IsEmpty() {
		return
	}

	statusService := n.modelService.StatusService()

	switch t := n.AgentTag().(type) {
	case names.UnitTag:
		err := statusService.DeleteUnitPresenceWithToken(ctx, unit.Name(t.Id()), n.token)
		if err != nil {
			n.logger.Infof(ctx, "recording presence for agent %s: unable to delete unit presence: %v", t, err)
		}
	case names.MachineTag:
		if err := statusService.DeleteMachinePresenceWithToken(ctx, machine.Name(t.Id()), n.token); err != nil {
			n.logger.Infof(ctx, "recording presence for agent %s: unable to delete machine presence: %v", t, err)
		}
	}
}

// RPCObserver returns an rpc.Observer for the agent presence that doesn't
// do anything.
func (n *AgentPresence) RPCObserver() rpc.Observer {
	return nil
}
