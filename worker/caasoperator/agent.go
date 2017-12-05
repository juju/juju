// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator

import "github.com/juju/juju/status"

// setAgentStatus sets the application's status if it has changed since last time this method was called.
func setAgentStatus(op *caasOperator, agentStatus status.Status, info string, data map[string]interface{}) error {
	op.setStatusMutex.Lock()
	defer op.setStatusMutex.Unlock()
	if op.lastReportedStatus == agentStatus && op.lastReportedMessage == info {
		return nil
	}
	op.lastReportedStatus = agentStatus
	op.lastReportedMessage = info
	logger.Infof("[AGENT-STATUS] %s: %s", agentStatus, info)
	// TODO(caas)
	return nil //op.SetAgentStatus(agentStatus, info, data)
}

// reportAgentError reports if there was an error performing an agent operation.
func reportAgentError(op *caasOperator, userMessage string, err error) {
	// If a non-nil error is reported (e.g. due to an operation failing),
	// set the agent status to Failed.
	if err == nil {
		return
	}
	logger.Errorf("%s: %v", userMessage, err)
	err2 := setAgentStatus(op, status.Failed, userMessage, nil)
	if err2 != nil {
		logger.Errorf("updating agent status: %v", err2)
	}
}
