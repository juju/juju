// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package observer

import (
	"fmt"
	"net/http"
	"time"

	"github.com/juju/errors"
	"github.com/juju/version"

	"github.com/juju/juju/audit"
	"github.com/juju/juju/rpc"
)

// PersistAuditEntryFn defines a function which will persist an
// AuditEntry to a backing store and return an error upon failure.
type AuditEntrySinkFn func(audit.AuditEntry) error

// Context defines things an Audit observer need know about to operate
// correctly.
type AuditContext struct {

	// JujuServerVersion is the version of jujud.
	JujuServerVersion version.Number

	// ModelUUID is the UUID of the model the audit observer is
	// currently running on.
	ModelUUID string
}

type ErrorHandler func(error)

// NewAudit creates a new Audit with the information provided via the Context.
func NewAudit(ctx *AuditContext, handleAuditEntry AuditEntrySinkFn, errorHandler ErrorHandler) *Audit {
	return &Audit{
		jujuServerVersion: ctx.JujuServerVersion,
		modelUUID:         ctx.ModelUUID,
		errorHandler:      errorHandler,
		handleAuditEntry:  handleAuditEntry,
	}
}

// Audit is an observer which will log APIServer requests using the
// function provided.
type Audit struct {
	jujuServerVersion version.Number
	modelUUID         string
	errorHandler      ErrorHandler
	handleAuditEntry  AuditEntrySinkFn

	// state represents information that's built up as methods on this
	// type are called. We segregate this to ensure it's clear what
	// information is transient in case we want to extract it
	// later. It's an anonymous struct so this doesn't leak outside
	// this type.
	state struct {
		remoteAddress    string
		authenticatedTag string
	}
}

// Login implements Observer.
func (a *Audit) Login(tag string) {
	a.state.authenticatedTag = tag
}

// ServerRequest implements Observer.
func (a *Audit) ServerRequest(hdr *rpc.Header, body interface{}) {
	auditEntry := a.boilerplateAuditEntry()
	//auditEntry.OriginIP =
	auditEntry.OriginType = "API request"
	auditEntry.OriginName = a.state.authenticatedTag
	auditEntry.Operation = rpcRequestToOperation(hdr.Request)
	auditEntry.Data = map[string]interface{}{"request-body": body}
	err := a.handleAuditEntry(auditEntry)
	if err != nil {
		a.errorHandler(errors.Trace(err))
	}
}

// Join implements Observer.
func (a *Audit) Join(req *http.Request) {
	a.state.remoteAddress = req.RemoteAddr
}

// Leave implements Observer.
func (a *Audit) Leave() {
	a.state.remoteAddress = ""
	a.state.authenticatedTag = ""
}

// ClientRequest implements Observer.
func (a *Audit) ClientRequest(hdr *rpc.Header, body interface{}) {}

// ServerReply implements Observer.
func (a *Audit) ServerReply(rpc.Request, *rpc.Header, interface{}) {}

// ClientReply implements Observer.
func (a *Audit) ClientReply(req rpc.Request, hdr *rpc.Header, body interface{}) {}

func (a *Audit) boilerplateAuditEntry() audit.AuditEntry {
	return audit.AuditEntry{
		JujuServerVersion: a.jujuServerVersion,
		ModelUUID:         a.modelUUID,
		Timestamp:         time.Now().UTC(),
		RemoteAddress:     a.state.remoteAddress,
		OriginName:        a.state.authenticatedTag,
	}
}

func rpcRequestToOperation(req rpc.Request) string {
	return fmt.Sprintf("%s:v%d - %s", req.Type, req.Version, req.Action)
}
