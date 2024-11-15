// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"context"
	stdcontext "context"

	"github.com/juju/juju/core/status"
)

// setAgentStatus sets the unit's status if it has changed since last time this method was called.
func setAgentStatus(ctx stdcontext.Context, u *Uniter, agentStatus status.Status, info string, data map[string]interface{}) error {
	u.setStatusMutex.Lock()
	defer u.setStatusMutex.Unlock()
	if u.lastReportedStatus == agentStatus && u.lastReportedMessage == info {
		return nil
	}
	u.lastReportedStatus = agentStatus
	u.lastReportedMessage = info
	u.logger.Debugf(context.TODO(), "[AGENT-STATUS] %s: %s", agentStatus, info)
	return u.unit.SetAgentStatus(ctx, agentStatus, info, data)
}

// reportAgentError reports if there was an error performing an agent operation.
func reportAgentError(ctx stdcontext.Context, u *Uniter, userMessage string, err error) {
	// If a non-nil error is reported (e.g. due to an operation failing),
	// set the agent status to Failed.
	if err == nil {
		return
	}
	u.logger.Errorf(context.TODO(), "%s: %v", userMessage, err)
	err2 := setAgentStatus(ctx, u, status.Failed, userMessage, nil)
	if err2 != nil {
		u.logger.Errorf(context.TODO(), "updating agent status: %v", err2)
	}
}
