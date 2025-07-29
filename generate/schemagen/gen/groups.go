// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gen

import (
	"reflect"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/rpcreflect"

	"github.com/juju/juju/apiserver/facade"
)

// FacadeGroup defines the grouping you want to export.
type FacadeGroup string

const (
	// Latest gets the latest facades from all the facades.
	Latest FacadeGroup = "latest"
	// All gets all the facades no matter the version.
	All FacadeGroup = "all"
	// Client facades returns just the client facades along with some required
	// facades that the client can use.
	Client FacadeGroup = "client"
	// Agent facades returns just the agent facades along with some required
	// facades that the agent can use (such as admin for login).
	Agent FacadeGroup = "agent"
	// JIMM facade group defines a very select set of facades that only work
	// with JIMM. This does not include the JIMM facade as defined in JIMM.
	JIMM FacadeGroup = "jimm"
)

// ParseFacadeGroup will attempt to parse the facade group
func ParseFacadeGroup(s string) (FacadeGroup, error) {
	switch s {
	case "latest", "all", "client", "agent", "jimm":
		return FacadeGroup(s), nil
	default:
		return FacadeGroup(""), errors.NotValidf("facade group %q", s)
	}
}

// Filter the facades based on the group.
func Filter(g FacadeGroup, facades []facade.Details, registry Registry) []facade.Details {
	switch g {
	case Latest:
		return latestFacades(facades)
	case All:
		return allFacades(facades)
	case Client:
		return clientFacades(facades, registry)
	case Agent:
		return agentFacades(facades, registry)
	case JIMM:
		return jimmFacades(facades)
	}
	return facades
}

func latestFacades(facades []facade.Details) []facade.Details {
	latest := make(map[string]facade.Details)
	for _, facade := range facades {
		if f, ok := latest[facade.Name]; ok && facade.Version < f.Version {
			continue
		}
		latest[facade.Name] = facade
	}
	latestFacades := make([]facade.Details, 0, len(latest))
	for _, v := range latest {
		latestFacades = append(latestFacades, v)
	}
	return latestFacades
}

func allFacades(facades []facade.Details) []facade.Details {
	return facades
}

func clientFacades(facades []facade.Details, registry Registry) []facade.Details {
	required := map[string]struct{}{
		"Admin":               {},
		"AllWatcher":          {},
		"AllModelWatcher":     {},
		"ModelSummaryManager": {},
		"Pinger":              {},
	}

	results := make([]facade.Details, 0)
	latest := latestFacades(facades)
	for _, v := range latest {
		if _, ok := required[v.Name]; ok {
			results = append(results, v)
			continue
		}

		var objType *rpcreflect.ObjType
		kind, err := registry.GetType(v.Name, v.Version)
		if err == nil {
			objType = rpcreflect.ObjTypeOf(kind)
		} else {
			objType = rpcreflect.ObjTypeOf(v.Type)
			if objType == nil {
				continue
			}
		}
		pkg := packageName(objType.GoType())
		if !strings.HasPrefix(pkg, "github.com/juju/juju/apiserver/facades/client/") {
			continue
		}
		results = append(results, v)
	}
	return results
}

func agentFacades(facades []facade.Details, registry Registry) []facade.Details {
	required := map[string]struct{}{
		"Admin":  {},
		"Pinger": {},
	}

	results := make([]facade.Details, 0)
	latest := latestFacades(facades)
	for _, v := range latest {
		if _, ok := required[v.Name]; ok {
			results = append(results, v)
			continue
		}

		var objType *rpcreflect.ObjType
		kind, err := registry.GetType(v.Name, v.Version)
		if err == nil {
			objType = rpcreflect.ObjTypeOf(kind)
		} else {
			objType = rpcreflect.ObjTypeOf(v.Type)
			if objType == nil {
				continue
			}
		}
		pkg := packageName(objType.GoType())
		if !strings.HasPrefix(pkg, "github.com/juju/juju/apiserver/facades/agent/") {
			continue
		}
		results = append(results, v)
	}
	return results
}

func jimmFacades(facades []facade.Details) []facade.Details {
	// The JIMM facades are the ones that are baked into JIMM directly. If JIMM
	// ever updates it's baked in facade schemas, then we should also update the
	// ones here.
	required := map[string][]int{
		"Bundle":              {1},
		"Cloud":               {1, 2, 3, 4, 5},
		"Controller":          {3, 4, 5, 6, 7, 8, 9, 10, 11},
		"ModelManager":        {2, 3, 4, 5},
		"ModelSummaryManager": {1},
		"Pinger":              {1},
		"UserManager":         {1},
	}

	result := make([]facade.Details, 0)
	for _, v := range facades {
		versions, ok := required[v.Name]
		if !ok {
			continue
		}

		for _, i := range versions {
			if v.Version == i {
				result = append(result, v)
				break
			}
		}
	}
	return result
}

func packageName(v reflect.Type) string {
	if v.Kind() == reflect.Ptr {
		return v.Elem().PkgPath()
	}
	return v.PkgPath()
}
