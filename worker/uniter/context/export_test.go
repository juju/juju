// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

var (
	MergeEnvironment  = mergeEnvironment
	SearchHook        = searchHook
	HookCommand       = hookCommand
	LookPath          = lookPath
	ValidatePortRange = validatePortRange
	TryOpenPorts      = tryOpenPorts
	TryClosePorts     = tryClosePorts
)

// PatchMeterStatus changes the meter status of the context.
func (ctx *HookContext) PatchMeterStatus(code, info string) func() {
	oldMeterStatus := ctx.meterStatus
	ctx.meterStatus = &meterStatus{
		code: code,
		info: info,
	}
	return func() {
		ctx.meterStatus = oldMeterStatus
	}
}

func (c *HookContext) ActionResultsMap() map[string]interface{} {
	if c.actionData == nil {
		panic("context not running an action")
	}
	return c.actionData.ResultsMap
}

func (c *HookContext) ActionFailed() bool {
	if c.actionData == nil {
		panic("context not running an action")
	}
	return c.actionData.ActionFailed
}

func GetStubActionContext(in map[string]interface{}) *HookContext {
	return &HookContext{
		actionData: &ActionData{
			ResultsMap: in,
		},
	}
}
