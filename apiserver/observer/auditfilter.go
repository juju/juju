// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package observer

import (
	"fmt"
	"sync"

	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/auditlog"
)

// bufferedLog defers writing records to its destination audit log
// until it sees an interesting request - then all buffered messages
// and subsequent ones get forwarded on.
type bufferedLog struct {
	mu          sync.Mutex
	buffer      []interface{}
	dest        auditlog.AuditLog
	interesting func(auditlog.Request) bool
}

// NewAuditLogFilter returns an auditlog.AuditLog that will only log
// conversations to the underlying log passed in if they include a
// request that satisfies the filter function passed in.
func NewAuditLogFilter(log auditlog.AuditLog, filter func(auditlog.Request) bool) auditlog.AuditLog {
	return &bufferedLog{
		dest:        log,
		interesting: filter,
	}
}

// AddConversation implements auditlog.AuditLog.
func (l *bufferedLog) AddConversation(c auditlog.Conversation) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	// We always buffer the conversation, since we don't know whether
	// it will have any interesting requests yet.
	l.deferMessage(c)
	return nil
}

// AddRequest implements auditlog.AuditLog.
func (l *bufferedLog) AddRequest(r auditlog.Request) error {
	l.mu.Lock()
	if len(l.buffer) > 0 {
		l.deferMessage(r)
		var err error
		if l.interesting(r) {
			err = l.flush()
		}
		l.mu.Unlock()
		return err
	}
	l.mu.Unlock()
	// We've already flushed messages, forward this on
	// immediately.
	return l.dest.AddRequest(r)
}

// AddResponse implements auditlog.AuditLog.
func (l *bufferedLog) AddResponse(r auditlog.ResponseErrors) error {
	l.mu.Lock()
	if len(l.buffer) > 0 {
		l.deferMessage(r)
		l.mu.Unlock()
		return nil
	}
	l.mu.Unlock()
	// We've already flushed messages, forward this on
	// immediately.
	return l.dest.AddResponse(r)
}

// Close implements auditlog.AuditLog.
func (l *bufferedLog) Close() error {
	return errors.Trace(l.dest.Close())
}

func (l *bufferedLog) deferMessage(m interface{}) {
	l.buffer = append(l.buffer, m)
}

func (l *bufferedLog) flush() error {
	for _, message := range l.buffer {
		var err error
		switch m := message.(type) {
		case auditlog.Conversation:
			err = l.dest.AddConversation(m)
		case auditlog.Request:
			err = l.dest.AddRequest(m)
		case auditlog.ResponseErrors:
			err = l.dest.AddResponse(m)
		default:
			err = errors.Errorf("unknown audit log message type %T %+v", m, m)
		}
		if err != nil {
			return errors.Trace(err)
		}
	}
	l.buffer = nil
	return nil
}

// MakeInterestingRequestFilter takes a set of method names (as
// facade.method, e.g. "Client.FullStatus") that aren't very
// interesting from an auditing perspective, and returns a filter
// function for audit logging that will mark the request as
// interesting if it's a call to a method that isn't listed. If one of
// the entries is "ReadOnlyMethods", any method matching the fixed
// list of read-only methods below will also be considered
// uninteresting.
func MakeInterestingRequestFilter(excludeMethods set.Strings) func(auditlog.Request) bool {
	return func(req auditlog.Request) bool {
		methodName := fmt.Sprintf("%s.%s", req.Facade, req.Method)
		if excludeMethods.Contains(methodName) {
			return false
		}
		if excludeMethods.Contains(controller.ReadOnlyMethodsWildcard) {
			return !readonlyMethods.Contains(methodName)
		}
		return true
	}
}

var readonlyMethods = set.NewStrings(
	// Collected by running read-only commands.
	"Action.Actions",
	"Action.ApplicationsCharmsActions",
	"Application.GetConstraints",
	"ApplicationOffers.ApplicationOffers",
	"Backups.Info",
	"Client.FullStatus",
	"Client.GetModelConstraints",
	"Client.StatusHistory",
	"Controller.AllModels",
	"Controller.LegacyControllerConfig",
	"Controller.GetControllerAccess",
	"Controller.ModelConfig",
	"Controller.ModelStatus",
	"MetricsDebug.GetMetrics",
	"ModelConfig.ModelGet",
	"ModelManager.ModelInfo",
	"ModelManager.ModelDefaults",
	"Pinger.Ping",
	"UserManager.UserInfo",

	// Don't filter out Application.Get - since it includes secrets
	// it's worthwhile to track when it's run, and it's not likely to
	// swamp the log.

	// All client facade methods that start with List.
	"Action.ListOperations",
	"ApplicationOffers.ListApplicationOffers",
	"Backups.List",
	"Block.List",
	"Charms.List",
	"Controller.ListBlockedModels",
	"FirewallRules.ListFirewallRules",
	"ImageMetadata.List",
	"KeyManager.ListKeys",
	"ModelManager.ListModels",
	"ModelManager.ListModelSummaries",
	"Payloads.List",
	"PayloadsHookContext.List",
	"Resources.ListResources",
	"ResourcesHookContext.ListResources",
	"Spaces.ListSpaces",
	"Storage.ListStorageDetails",
	"Storage.ListPools",
	"Storage.ListVolumes",
	"Storage.ListFilesystems",
	"Subnets.ListSubnets",
)
