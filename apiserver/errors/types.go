// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"fmt"
	"strings"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/permission"
)

const (
	// DeadlineExceededError is for when a raft operation is enqueued, but the
	// deadline is exceeded.
	DeadlineExceededError = errors.ConstError("deadline exceeded")

	// IncompatibleSeriesError indicates the series selected is not supported by the
	// charm.
	IncompatibleSeriesError = errors.ConstError("incompatible series for charm")

	NoAddressSetError = errors.ConstError("no address set")

	// UnknownModelError is for when an operation failed to find a model by
	// a given model uuid.
	UnknownModelError = errors.ConstError("unknown model")
)

func NotSupportedError(tag names.Tag, operation string) error {
	return errors.Errorf("entity %q does not support %s", tag, operation)
}

func NewNoAddressSetError(unitTag names.UnitTag, addressName string) error {
	return fmt.Errorf("%q has no %s address set%w",
		unitTag,
		addressName,
		errors.Hide(NoAddressSetError),
	)
}

// DischargeRequiredError is the error returned when a macaroon requires
// discharging to complete authentication.
type DischargeRequiredError struct {
	Cause          error
	LegacyMacaroon *macaroon.Macaroon
	Macaroon       *bakery.Macaroon
}

// Error implements the error interface.
func (e *DischargeRequiredError) Error() string {
	return e.Cause.Error()
}

// UpgradeSeriesValidationError is the error returns when an upgrade-machine
// can not be run because of a validation error.
type UpgradeSeriesValidationError struct {
	Cause  error
	Status string
}

// Error implements the error interface.
func (e *UpgradeSeriesValidationError) Error() string {
	return e.Cause.Error()
}

func NewErrIncompatibleSeries(seriesList []string, series, charmName string) error {
	return fmt.Errorf("series %q not supported by charm %q, supported series are: %s%w",
		series,
		charmName,
		strings.Join(seriesList, ", "),
		errors.Hide(IncompatibleSeriesError),
	)
}

// RedirectError is the error returned when a model (previously accessible by
// the user) has been migrated to a different controller.
type RedirectError struct {
	// Servers holds the sets of addresses of the redirected servers.
	// TODO (manadart 2019-11-08): Change this to be either MachineHostPorts
	// or the HostPorts indirection. We don't care about space info here.
	// We can then delete the API params helpers for conversion for this type
	// as it will no longer be used.
	Servers []network.ProviderHostPorts `json:"servers"`

	// CACert holds the certificate of the remote server.
	CACert string `json:"ca-cert"`

	// ControllerTag uniquely identifies the controller being redirected to.
	ControllerTag names.ControllerTag `json:"controller-tag,omitempty"`

	// An optional alias for the controller where the model got redirected to.
	ControllerAlias string `json:"controller-alias,omitempty"`
}

// Error implements the error interface.
func (e *RedirectError) Error() string {
	return "redirection to alternative server required"
}

// NotLeaderError creates a typed error for when a raft operation is applied,
// but the raft state shows that it's not the leader. The error will help
// redirect the consumer of the error to workout where they can try and find
// the leader.
type NotLeaderError struct {
	serverAddress string
	serverID      string
}

func (e *NotLeaderError) Error() string {
	return fmt.Sprintf("not currently the leader, try %q", e.serverID)
}

// ServerAddress returns the address of the potential current leader. It's not
// guaranteed to be the leader, as things may of changed when attempting the
// same request on the new leader.
func (e *NotLeaderError) ServerAddress() string {
	return e.serverAddress
}

// Is returns true if the target is the NotLeaderError.
//func (e *NotLeaderError) Is(target error) bool {
//	_, ok := target.(*NotLeaderError)
//	return ok
//}

// ServerID returns the server ID from the raft state. This should align with
// the controller machine ID of Juju.
func (e *NotLeaderError) ServerID() string {
	return e.serverID
}

// AsMap returns a map of the error. Useful when crossing the facade boundary
// and wanting information in the client.
func (e *NotLeaderError) AsMap() map[string]interface{} {
	return map[string]interface{}{
		"server-address": e.serverAddress,
		"server-id":      e.serverID,
	}
}

// NewNotLeaderError creates a new NotLeaderError with the server address and/or
// server ID of the current raft state leader.
func NewNotLeaderError(serverAddress, serverID string) error {
	return &NotLeaderError{
		serverAddress: serverAddress,
		serverID:      serverID,
	}
}

// AccessRequiredError is the error returned when an api
// request needs a login token with specified permissions.
type AccessRequiredError struct {
	RequiredAccess map[names.Tag]permission.Access
}

// AsMap returns the data for the info part of an error param struct.
func (e *AccessRequiredError) AsMap() map[string]interface{} {
	result := make(map[string]interface{})
	for t, a := range e.RequiredAccess {
		result[t.String()] = a
	}
	return result
}

// Error implements the error interface.
func (e *AccessRequiredError) Error() string {
	return fmt.Sprintf("access permissions required: %v", e.RequiredAccess)
}
