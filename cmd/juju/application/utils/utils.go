// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"fmt"

	"github.com/juju/charm/v8"
	charmresource "github.com/juju/charm/v8/resource"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/resource"
)

var logger = loggo.GetLogger("juju.cmd.juju.application.utils")

// GetMetaResources retrieves metadata resources for the given
// charm.URL.
func GetMetaResources(charmURL *charm.URL, client CharmClient) (map[string]charmresource.Meta, error) {
	charmInfo, err := client.CharmInfo(charmURL.String())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return charmInfo.Meta.Resources, nil
}

// ParsePlacement validates provided placement of a unit and
// returns instance.Placement.
func ParsePlacement(spec string) (*instance.Placement, error) {
	if spec == "" {
		return nil, nil
	}
	placement, err := instance.ParsePlacement(spec)
	if err == instance.ErrPlacementScopeMissing {
		spec = fmt.Sprintf("model-uuid:%s", spec)
		placement, err = instance.ParsePlacement(spec)
	}
	if err != nil {
		return nil, errors.Errorf("invalid --to parameter %q", spec)
	}
	return placement, nil
}

// GetFlags returns the flags with the given names. Only flags that are set and
// whose name is included in flagNames are included.
func GetFlags(flagSet *gnuflag.FlagSet, flagNames []string) []string {
	flags := make([]string, 0, flagSet.NFlag())
	flagSet.Visit(func(flag *gnuflag.Flag) {
		for _, name := range flagNames {
			if flag.Name == name {
				flags = append(flags, flagWithMinus(name))
			}
		}
	})
	return flags
}

func flagWithMinus(name string) string {
	if len(name) > 1 {
		return "--" + name
	}
	return "-" + name
}

// GetUpgradeResources
func GetUpgradeResources(
	resourceLister ResourceLister,
	applicationID string,
	cliResources map[string]string,
	meta map[string]charmresource.Meta,
) (map[string]charmresource.Meta, error) {
	if len(meta) == 0 {
		return nil, nil
	}

	current, err := getResources(applicationID, resourceLister)
	if err != nil {
		return nil, errors.Trace(err)
	}
	filtered := filterResources(meta, current, cliResources)
	return filtered, nil
}

func getResources(
	applicationID string,
	resourceLister ResourceLister,
) (map[string]resource.Resource, error) {
	svcs, err := resourceLister.ListResources([]string{applicationID})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return resource.AsMap(svcs[0].Resources), nil
}

func filterResources(
	meta map[string]charmresource.Meta,
	current map[string]resource.Resource,
	uploads map[string]string,
) map[string]charmresource.Meta {
	filtered := make(map[string]charmresource.Meta)
	for name, res := range meta {
		if shouldUpgradeResource(res, uploads, current) {
			filtered[name] = res
		}
	}
	return filtered
}

// shouldUpgradeResource reports whether we should upload the metadata for the given
// resource.  This is always true for resources we're adding with the --resource
// flag. For resources we're not adding with --resource, we only upload metadata
// for charmstore resources.  Previously uploaded resources stay pinned to the
// data the user uploaded.
func shouldUpgradeResource(res charmresource.Meta, uploads map[string]string, current map[string]resource.Resource) bool {
	// Always upload metadata for resources the user is uploading during
	// upgrade-charm.
	if _, ok := uploads[res.Name]; ok {
		return true
	}
	cur, ok := current[res.Name]
	if !ok {
		// If there's no information on the server, there should be.
		return true
	}
	// Never override existing resources a user has already uploaded.
	if cur.Origin == charmresource.OriginUpload {
		return false
	}
	return true
}
