package cachetest

import "github.com/juju/juju/core/cache"

var ModelEvents = func(change interface{}) bool {
	switch change.(type) {
	case cache.ModelChange:
		return true
	case cache.RemoveModel:
		return true
	}
	return false
}

var ApplicationEvents = func(change interface{}) bool {
	switch change.(type) {
	case cache.ApplicationChange:
		return true
	case cache.RemoveApplication:
		return true
	}
	return false
}

var MachineEvents = func(change interface{}) bool {
	switch change.(type) {
	case cache.MachineChange:
		return true
	case cache.RemoveMachine:
		return true
	}
	return false
}

var CharmEvents = func(change interface{}) bool {
	switch change.(type) {
	case cache.CharmChange:
		return true
	case cache.RemoveCharm:
		return true
	}
	return false
}

var UnitEvents = func(change interface{}) bool {
	switch change.(type) {
	case cache.UnitChange:
		return true
	case cache.RemoveUnit:
		return true
	}
	return false
}

var BranchEvents = func(change interface{}) bool {
	switch change.(type) {
	case cache.BranchChange:
		return true
	case cache.RemoveBranch:
		return true
	}
	return false
}
