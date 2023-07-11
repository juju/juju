// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

// AppContext holds the data necessary to derive a status
// value for an application.
type AppContext struct {
	IsCaas    bool
	AppStatus StatusInfo
	UnitCtx   []UnitContext

	// These are only set for CAAS models.

	OperatorStatus StatusInfo
}

// UnitContext holds the data necessary to derive a status
// // value for a unit.
type UnitContext struct {
	WorkloadStatus StatusInfo

	// These are only set for CAAS models.

	ContainerStatus StatusInfo
}

// DisplayApplicationStatus returns the application status we show to the user.
// For CAAS models, this is a synthetic status based on
// the application status and the operator status.
func DisplayApplicationStatus(ctx AppContext) StatusInfo {
	info := ctx.AppStatus
	if info.Status == Unset {
		unitStatus := make([]StatusInfo, len(ctx.UnitCtx))
		for i, u := range ctx.UnitCtx {
			if ctx.IsCaas {
				unitStatus[i] = UnitDisplayStatus(u.WorkloadStatus, u.ContainerStatus)
			} else {
				unitStatus[i] = u.WorkloadStatus
			}

		}
		info = DeriveStatus(unitStatus)
	}
	if info.Since == nil {
		info.Since = ctx.AppStatus.Since
	}
	if ctx.OperatorStatus.Status == "" {
		return info
	}
	return ApplicationDisplayStatus(info, ctx.OperatorStatus)
}
