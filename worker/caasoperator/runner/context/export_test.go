// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"github.com/juju/utils/clock"
	"github.com/juju/utils/proxy"

	"github.com/juju/juju/worker/caasoperator/commands"
	"github.com/juju/juju/worker/caasoperator/runner/runnertesting"
)

func NewHookContext(
	hookAPI hookAPI,
	id,
	uuid,
	applicationName string,
	modelName string,
	relationId int,
	remoteUnitName string,
	relations map[int]*ContextRelation,
	apiAddrs []string,
	proxySettings proxy.Settings,
	clock clock.Clock,
) (*HookContext, error) {
	ctx := &HookContext{
		hookAPI:         hookAPI,
		id:              id,
		uuid:            uuid,
		modelName:       modelName,
		applicationName: applicationName,
		relationId:      relationId,
		remoteUnitName:  remoteUnitName,
		relations:       relations,
		apiAddrs:        apiAddrs,
		proxySettings:   proxySettings,
		clock:           clock,
	}
	return ctx, nil
}

// SetEnvironmentHookContextRelation exists purely to set the fields used in hookVars.
// It makes no assumptions about the validity of context.
func SetEnvironmentHookContextRelation(
	context *HookContext,
	relationId int, endpointName, remoteUnitName string,
) {
	context.relationId = relationId
	context.remoteUnitName = remoteUnitName
	context.relations = map[int]*ContextRelation{
		relationId: {
			endpointName: endpointName,
			relationId:   relationId,
		},
	}
}

func PatchCachedStatus(ctx commands.Context, status, info string, data map[string]interface{}) func() {
	hctx := ctx.(*HookContext)
	oldStatus := hctx.status
	appStatusInfo := &commands.StatusInfo{
		Status: status,
		Info:   info,
		Data:   data,
	}
	hctx.status = appStatusInfo
	return func() {
		hctx.status = oldStatus
	}
}

// NewModelHookContext exists purely to set the fields used in rs.
// The returned value is not otherwise valid.
func NewModelHookContext(
	id, modelUUID, modelName, applicationName string,
	apiAddresses []string, proxySettings proxy.Settings,
) *HookContext {
	return &HookContext{
		id:              id,
		applicationName: applicationName,
		uuid:            modelUUID,
		modelName:       modelName,
		apiAddrs:        apiAddresses,
		proxySettings:   proxySettings,
		relationId:      -1,
	}
}

func ContextModelInfo(hctx *HookContext) (name, uuid string) {
	return hctx.modelName, hctx.uuid
}

func UpdateCachedSettings(cf0 ContextFactory, relId int, unitName string, settings map[string]string) {
	cf := cf0.(*contextFactory)
	members := cf.relationCaches[relId].members
	if members[unitName] == nil {
		members[unitName] = make(runnertesting.Settings)
	}
	for key, value := range settings {
		members[unitName].Set(key, value)
	}
}

func CachedSettings(cf0 ContextFactory, relId int, unitName string) (commands.Settings, bool) {
	cf := cf0.(*contextFactory)
	settings, found := cf.relationCaches[relId].members[unitName]
	return settings, found
}
