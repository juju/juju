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
	"github.com/juju/juju/core/params"
	stateerrors "github.com/juju/juju/state/errors"
)

func NotSupportedError(tag names.Tag, operation string) error {
	return errors.Errorf("entity %q does not support %s", tag, operation)
}

type noAddressSetError struct {
	unitTag     names.UnitTag
	addressName string
}

func (e *noAddressSetError) Error() string {
	return fmt.Sprintf("%q has no %s address set", e.unitTag, e.addressName)
}

func NoAddressSetError(unitTag names.UnitTag, addressName string) error {
	return &noAddressSetError{unitTag: unitTag, addressName: addressName}
}

func isNoAddressSetError(err error) bool {
	_, ok := err.(*noAddressSetError)
	return ok
}

type unknownModelError struct {
	uuid string
}

func (e *unknownModelError) Error() string {
	return fmt.Sprintf("unknown model: %q", e.uuid)
}

func UnknownModelError(uuid string) error {
	return &unknownModelError{uuid: uuid}
}

func isUnknownModelError(err error) bool {
	_, ok := err.(*unknownModelError)
	return ok
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

// IsDischargeRequiredError reports whether the cause
// of the error is a *DischargeRequiredError.
func IsDischargeRequiredError(err error) bool {
	_, ok := errors.Cause(err).(*DischargeRequiredError)
	return ok
}

// IsUpgradeInProgressError returns true if this error is caused
// by an upgrade in progress.
func IsUpgradeInProgressError(err error) bool {
	if stateerrors.IsUpgradeInProgressError(err) {
		return true
	}
	return errors.Cause(err) == params.UpgradeInProgressError
}

// UpgradeSeriesValidationError is the error returns when a upgrade-series
// can not be run because of a validation error.
type UpgradeSeriesValidationError struct {
	Cause  error
	Status string
}

// Error implements the error interface.
func (e *UpgradeSeriesValidationError) Error() string {
	return e.Cause.Error()
}

// IsUpgradeSeriesValidationError returns true if this error is caused by a
// upgrade-series validation error.
func IsUpgradeSeriesValidationError(err error) bool {
	_, ok := errors.Cause(err).(*UpgradeSeriesValidationError)
	return ok
}

// errIncompatibleSeries is a standard error to indicate that the series
// requested is not compatible with the charm of the application.
type errIncompatibleSeries struct {
	seriesList []string
	series     string
	charmName  string
}

func NewErrIncompatibleSeries(seriesList []string, series, charmName string) error {
	return &errIncompatibleSeries{
		seriesList: seriesList,
		series:     series,
		charmName:  charmName,
	}
}

func (e *errIncompatibleSeries) Error() string {
	return fmt.Sprintf("series %q not supported by charm %q, supported series are: %s",
		e.series, e.charmName, strings.Join(e.seriesList, ", "))
}

// IsIncompatibleSeriesError returns if the given error or its cause is
// errIncompatibleSeries.
func IsIncompatibleSeriesError(err interface{}) bool {
	if err == nil {
		return false
	}
	// In case of a wrapped error, check the cause first.
	value := err
	cause := errors.Cause(err.(error))
	if cause != nil {
		value = cause
	}
	_, ok := value.(*errIncompatibleSeries)
	return ok
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

// IsRedirectError returns true if err is caused by a RedirectError.
func IsRedirectError(err error) bool {
	_, ok := errors.Cause(err).(*RedirectError)
	return ok
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

// IsNotLeaderError returns true if the error is the NotLeaderError.
func IsNotLeaderError(err error) bool {
	_, ok := errors.Cause(err).(*NotLeaderError)
	return ok
}

// DeadlineExceededError creates a typed error for when a raft operation is
// enqueued, but the deadline is exceeded.
type DeadlineExceededError struct {
	message string
}

func (e *DeadlineExceededError) Error() string {
	return e.message
}

// NewDeadlineExceededError creates a new DeadlineExceededError with the
// underlying message.
func NewDeadlineExceededError(message string) error {
	return &DeadlineExceededError{
		message: message,
	}
}

// IsDeadlineExceededError returns true if the error is the DeadlineExceededError.
func IsDeadlineExceededError(err error) bool {
	_, ok := errors.Cause(err).(*DeadlineExceededError)
	return ok
}
