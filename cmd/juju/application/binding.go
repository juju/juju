// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"fmt"
	"sort"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
)

const parseBindError = "--bind must be in the form '[<default-space>] [<endpoint-name>=<space> ...]'; %s"

// parseBindExpr parses the --bind option and returns back a map where keys
// are endpoint names and values are space names. Valid forms are:
//   - relation-name=space-name
//   - extra-binding-name=space-name
//   - space-name (equivalent to binding all endpoints to the same space, i.e. application-default)
//   - The above in a space separated list to specify multiple bindings,
//     e.g. "rel1=space1 ext1=space2 space3"
func parseBindExpr(expr string, knownSpaceNames set.Strings) (map[string]string, error) {
	if expr == "" {
		return nil, nil
	}

	parsedBindings := make(map[string]string)
	for _, s := range strings.Fields(expr) {
		v := strings.Split(s, "=")
		var endpoint, spaceName string
		switch len(v) {
		case 1:
			endpoint = ""
			spaceName = v[0]
		case 2:
			if v[0] == "" {
				return nil, errors.Errorf(parseBindError,
					`found "=" without endpoint name. Use a lone space name to set the default.`)
			}
			endpoint = v[0]
			spaceName = v[1]
		default:
			return nil, errors.Errorf(parseBindError,
				`found multiple "=" in binding. Did you forget to space-separate the binding list?`)
		}

		// This is a temporary hack to allow us to bind endpoints to the
		// default spaceName which is currently represented as a blank string.
		// It will be removed when the work for properly naming the
		// default spaceName lands.
		spaceName = strings.Trim(spaceName, `"`)

		if !knownSpaceNames.Contains(spaceName) {
			return nil, errors.NotFoundf("space %q", spaceName)
		}

		parsedBindings[endpoint] = spaceName
	}
	return parsedBindings, nil
}

// mergeBindings is invoked when upgrading a charm to merge the existing set of
// endpoint to space assignments for a deployed application and the user-defined
// bindings passed to the upgrade-charm command.
func mergeBindings(
	newCharmEndpoints set.Strings, oldEndpointsMap, userBindings map[string]string, oldDefaultSpace string,
) (map[string]string, []string) {
	var changelog []string
	if oldEndpointsMap == nil {
		oldEndpointsMap = make(map[string]string)
	}
	if userBindings == nil {
		userBindings = make(map[string]string)
	}

	newDefaultSpace, newDefaultBindingDefined := userBindings[""]
	changeDefaultSpace := newDefaultBindingDefined && oldDefaultSpace != newDefaultSpace
	mergedBindings := make(map[string]string)
	unmodifiedEndpoints := make(map[string][]string)
	for _, epName := range newCharmEndpoints.SortedValues() {
		newSpaceAssignment, newBindingDefined := userBindings[epName]

		// If this is a new endpoint and it is not specified, assign it to the default space
		oldSpaceAssignment, isOldEndpoint := oldEndpointsMap[epName]
		if !isOldEndpoint {
			// If not explicitly defined, it inherits the default space for the application
			if !newBindingDefined {
				mergedBindings[epName] = oldDefaultSpace
				changelog = append(changelog, fmt.Sprintf("adding endpoint %q to default space %q", epName, oldDefaultSpace))
			} else {
				mergedBindings[epName] = newSpaceAssignment
				changelog = append(changelog, fmt.Sprintf("adding endpoint %q to space %q", epName, newSpaceAssignment))
			}
			continue
		}

		// This is an existing endpoint. Check whether the operator
		// specified a different space for it.
		if newBindingDefined && oldSpaceAssignment != newSpaceAssignment {
			mergedBindings[epName] = newSpaceAssignment
			changelog = append(changelog,
				fmt.Sprintf("moving endpoint %q from space %q to %q", epName, oldSpaceAssignment, newSpaceAssignment))
			continue
		}

		// Next, check whether the endpoint is bound to the old default
		// space and override it to the new default space.
		if !newBindingDefined && changeDefaultSpace && oldSpaceAssignment == oldDefaultSpace {
			mergedBindings[epName] = newDefaultSpace
			changelog = append(changelog,
				fmt.Sprintf("moving endpoint %q from space %q to %q", epName, oldSpaceAssignment, newDefaultSpace))
			continue
		}

		// Retain the existing binding
		mergedBindings[epName] = oldSpaceAssignment
		unmodifiedEndpoints[oldSpaceAssignment] = append(unmodifiedEndpoints[oldSpaceAssignment], epName)
	}

	for spName, epList := range unmodifiedEndpoints {
		var pluralSuffix string
		if len(epList) > 1 {
			pluralSuffix = "s"
		}

		sort.Strings(epList)
		changelog = append(changelog,
			fmt.Sprintf("no change to endpoint%s in space %q: %s", pluralSuffix, spName, strings.Join(epList, ", ")))
	}

	if changeDefaultSpace {
		mergedBindings[""] = newDefaultSpace
	}
	return mergedBindings, changelog
}
