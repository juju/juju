// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/trace"
	controllernodeerrors "github.com/juju/juju/domain/controllernode/errors"
	"github.com/juju/juju/internal/errors"
)

// State describes retrieval and persistence
// methods for controller node concerns.
type State interface {
	// CurateNodes adds and removes controller node records according to the
	// input slices.
	CurateNodes(context.Context, []string, []string) error

	// UpdateDqliteNode sets the Dqlite node ID and bind address
	// for the input controller ID.
	// The controller ID must be a valid controller node.
	UpdateDqliteNode(context.Context, string, uint64, string) error

	// IsControllerNode returns true if the supplied nodeID is a controller
	// node.
	IsControllerNode(context.Context, string) (bool, error)

	// SelectDatabaseNamespace returns the database namespace for the supplied
	// namespace.
	SelectDatabaseNamespace(context.Context, string) (string, error)

	// SetRunningAgentBinaryVersion sets the agent version for the supplied
	// controllerID. Version represents the version of the controller node's
	// agent binary.
	SetRunningAgentBinaryVersion(context.Context, string, coreagentbinary.Version) error
}

// Service provides the API for working with controller nodes.
type Service struct {
	st State
}

// NewService returns a new service reference wrapping the input state.
func NewService(st State) *Service {
	return &Service{st}
}

// CurateNodes modifies the known control plane by adding and removing
// controller node records according to the input slices.
func (s *Service) CurateNodes(ctx context.Context, toAdd, toRemove []string) (err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	if err := s.st.CurateNodes(ctx, toAdd, toRemove); err != nil {
		return errors.Errorf("curating controller codes; adding %v, removing %v: %w", toAdd, toRemove, err)
	}
	return nil
}

// UpdateDqliteNode sets the Dqlite node ID and bind address for the input
// controller ID.
func (s *Service) UpdateDqliteNode(ctx context.Context, controllerID string, nodeID uint64, addr string) (err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	if err := s.st.UpdateDqliteNode(ctx, controllerID, nodeID, addr); err != nil {
		return errors.Errorf("updating Dqlite node details for %q: %w", controllerID, err)
	}
	return nil
}

// IsKnownDatabaseNamespace reports if the namespace is known to the controller.
// If the namespace is not valid an error satisfying [errors.NotValid] is
// returned.
func (s *Service) IsKnownDatabaseNamespace(ctx context.Context, namespace string) (_ bool, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	if namespace == "" {
		return false, errors.Errorf("namespace %q is %w, cannot be empty", namespace, coreerrors.NotValid)
	}

	ns, err := s.st.SelectDatabaseNamespace(ctx, namespace)
	if err != nil && !errors.Is(err, controllernodeerrors.NotFound) {
		return false, errors.Errorf("determining namespace existence: %w", err)
	}

	return ns == namespace, nil
}

// SetControllerNodeReportedAgentVersion sets the agent version for the
// supplied controllerID. Version represents the version of the controller node's agent binary.
//
// The following errors are possible:
// - [coreerrors.NotValid] if the version is not valid.
// - [coreerrors.NotSupported] if the architecture is not supported.
// - [controllernodeerrors.NotFound] if the controller node does not exist.
func (s *Service) SetControllerNodeReportedAgentVersion(ctx context.Context, controllerID string, version coreagentbinary.Version) (err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	if err := version.Validate(); err != nil {
		return errors.Errorf("agent version %+v is not valid: %w", version, err)
	}

	if err := s.st.SetRunningAgentBinaryVersion(ctx, controllerID, version); err != nil {
		return errors.Errorf(
			"setting controller node %q agent version (%s): %w",
			controllerID,
			version.Number.String(),
			err,
		)
	}

	return nil
}

// IsControllerNode returns true if the supplied nodeID is a controller node.
func (s *Service) IsControllerNode(ctx context.Context, nodeID string) (bool, error) {
	if nodeID == "" {
		return false, errors.Errorf("node ID %q is %w, cannot be empty", nodeID, coreerrors.NotValid)
	}

	isController, err := s.st.IsControllerNode(ctx, nodeID)
	if err != nil {
		return false, errors.Errorf("checking is controller node: %w", err)
	}
	return isController, nil
}
