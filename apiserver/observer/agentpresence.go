// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package observer

import (
	"context"

	"github.com/juju/names/v6"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/rpc"
)

// ApplicationService provides methods to interact with the application
// model.
type StatusService interface {
	// SetUnitPresence marks the presence of the unit in the model. It is called by
	// the unit agent accesses the API server. If the unit is not found, an error
	// satisfying [applicationerrors.UnitNotFound] is returned. The unit life is not
	// considered when setting the presence.
	SetUnitPresence(ctx context.Context, unitName unit.Name) error

	// DeleteUnitPresence removes the presence of the unit in the model. If the unit
	// is not found, it ignores the error. The unit life is not considered when
	// deleting the presence.
	DeleteUnitPresence(ctx context.Context, unitName unit.Name) error
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

	if !n.IsAgent() {
		return
	}

	services, err := n.serviceGetter.ServicesForModel(ctx, modelUUID)
	if err != nil {
		n.logger.Infof(ctx, "recording presence for agent %s: unable to get domain services model: %s %v", entity, modelTag, err)
		return
	}

	n.modelService = services

	switch t := entity.(type) {
	case names.UnitTag:
		err := n.modelService.StatusService().SetUnitPresence(ctx, unit.Name(t.Id()))
		if err != nil {
			n.logger.Infof(ctx, "recording presence for agent %s: unable to set unit presence: %v", entity, err)
		}
	case names.MachineTag:
		// TODO (stickupkid): Handle machine agents.
	}
}

// Leave removes the agent presence to the database based on the entity type.
// Units and machines are the only entities that can have presence.
func (n *AgentPresence) Leave(ctx context.Context) {
	// This guards against the case where the agent has not logged in and
	// the agent tag is nil.
	if !n.IsAgent() {
		return
	}

	if n.modelService == nil {
		return
	}

	switch t := n.AgentTag().(type) {
	case names.UnitTag:
		err := n.modelService.StatusService().DeleteUnitPresence(ctx, unit.Name(t.Id()))
		if err != nil {
			n.logger.Infof(ctx, "recording presence for agent %s: unable to set unit presence: %v", t, err)
		}
	case names.MachineTag:
		// TODO (stickupkid): Handle machine agents.
	}
}

// RPCObserver returns an rpc.Observer for the agent presence that doesn't
// do anything.
func (n *AgentPresence) RPCObserver() rpc.Observer {
	return nil
}
