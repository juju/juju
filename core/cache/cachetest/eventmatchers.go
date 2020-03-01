// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cachetest

import "github.com/juju/juju/core/cache"

func ControllerEvents(change interface{}) bool {
	switch change.(type) {
	case cache.ControllerConfigChange:
		return true
	}
	return false
}

func ModelEvents(change interface{}) bool {
	switch change.(type) {
	case cache.ModelChange:
		return true
	case cache.RemoveModel:
		return true
	}
	return false
}

func ApplicationEvents(change interface{}) bool {
	switch change.(type) {
	case cache.ApplicationChange:
		return true
	case cache.RemoveApplication:
		return true
	}
	return false
}

func MachineEvents(change interface{}) bool {
	switch change.(type) {
	case cache.MachineChange:
		return true
	case cache.RemoveMachine:
		return true
	}
	return false
}

func CharmEvents(change interface{}) bool {
	switch change.(type) {
	case cache.CharmChange:
		return true
	case cache.RemoveCharm:
		return true
	}
	return false
}

func UnitEvents(change interface{}) bool {
	switch change.(type) {
	case cache.UnitChange:
		return true
	case cache.RemoveUnit:
		return true
	}
	return false
}

func BranchEvents(change interface{}) bool {
	switch change.(type) {
	case cache.BranchChange:
		return true
	case cache.RemoveBranch:
		return true
	}
	return false
}
