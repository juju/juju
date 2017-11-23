// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package auditlog

import (
	"encoding/hex"
	"math/rand"

	"github.com/juju/errors"
)

// Call represents a high-level juju command from the juju client (or
// other client). There'll be one Call per API connection, with zero
// or more associated FacadeMethods.
type Call struct {
	Who          string `yaml:"who"`        // username@idm
	What         string `yaml:"what"`       // "juju deploy ./foo/bar"
	When         string `yaml:"when"`       // ISO 8601 to second precision
	ModelName    string `yaml:"model-name"` // full representation "user/name"
	ModelUUID    string `yaml:"model-uuid"`
	ConnectionID string `yaml:"connection-id"` // uint64 in hex
}

// CallArgs is the information needed to create a method recorder.
type CallArgs struct {
	Who       string
	What      string
	When      string
	ModelName string
	ModelUUID string
}

// FacadeMethod represents a call to an API facade made as part of
// executing a specific high-level command.
type FacadeMethod struct {
	ConnectionID string `yaml:"connection-id"`
	Facade       string `yaml:"facade"`
	Method       string `yaml:"method"`
	Version      int    `yaml:"version"`
	Args         string `yaml:"args"`
}

// MethodArgs is the information about an API call that we want to
// record.
type MethodArgs struct {
	Facade  string
	Method  string
	Version int
	Args    string
}

// AuditLog represents something that can store calls and methods
// somewhere.
type AuditLog interface {
	AddCall(c Call) error
	AddMethod(m FacadeMethod) error
}

// Recorder records method calls for a specific API connection.
type Recorder struct {
	log          AuditLog
	connectionID string
}

// NewRecorder creates a Recorder for the connection described (and
// stores details of the initial call in the log).
func NewRecorder(log AuditLog, c CallArgs) (*Recorder, error) {
	connectionID := newConnectionID()
	err := log.AddCall(Call{
		ConnectionID: connectionID,
		Who:          c.Who,
		What:         c.What,
		When:         c.When,
		ModelName:    c.ModelName,
		ModelUUID:    c.ModelUUID,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &Recorder{log: log, connectionID: connectionID}, nil
}

// Add records a method call to the API.
func (r *Recorder) Add(m MethodArgs) error {
	return errors.Trace(r.log.AddMethod(FacadeMethod{
		ConnectionID: r.connectionID,
		Facade:       m.Facade,
		Method:       m.Method,
		Version:      m.Version,
		Args:         m.Args,
	}))
}

func newConnectionID() string {
	buf := make([]byte, 8)
	rand.Read(buf) // Can't fail
	return hex.EncodeToString(buf)
}
