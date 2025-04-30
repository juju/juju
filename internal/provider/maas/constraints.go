// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"context"
	"net/url"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/gomaasapi/v2"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/constraints"
)

var unsupportedConstraints = []string{
	constraints.CpuPower,
	constraints.InstanceType,
	constraints.VirtType,
	constraints.AllocatePublicIP,
}

// ConstraintsValidator is defined on the Environs interface.
func (env *maasEnviron) ConstraintsValidator(ctx context.Context) (constraints.Validator, error) {
	validator := constraints.NewValidator()
	validator.RegisterUnsupported(unsupportedConstraints)
	supportedArches, err := env.getSupportedArchitectures(ctx)
	if err != nil {
		return nil, err
	}

	// Only consume supported juju architectures for this release. This will
	// also remove any duplicate architectures.
	maasArches := set.NewStrings(supportedArches...)
	supported := set.NewStrings(arch.AllSupportedArches...).Intersection(maasArches)

	validator.RegisterVocabulary(constraints.Arch, supported.SortedValues())

	return validator, nil
}

// convertConstraints converts the given constraints into a
// gomaasapi.AllocateMachineArgs for passing to MAAS.
func convertConstraints(cons constraints.Value) gomaasapi.AllocateMachineArgs {
	params := gomaasapi.AllocateMachineArgs{}
	if cons.Arch != nil {
		params.Architecture = *cons.Arch
	}
	if cons.CpuCores != nil {
		params.MinCPUCount = int(*cons.CpuCores)
	}
	if cons.Mem != nil {
		params.MinMemory = int(*cons.Mem)
	}
	if cons.Tags != nil {
		positives, negatives := parseDelimitedValues(*cons.Tags)
		if len(positives) > 0 {
			params.Tags = positives
		}
		if len(negatives) > 0 {
			params.NotTags = negatives
		}
	}
	if cons.CpuPower != nil {
		logger.Warningf(context.TODO(), "ignoring unsupported constraint 'cpu-power'")
	}
	return params
}

// convertTagsToParams converts a list of positive/negative tags from
// constraints into two comma-delimited lists of values, which can then be
// passed to MAAS using the "tags" and "not_tags" arguments to acquire. If
// either list of tags is empty, the respective argument is not added to params.
func convertTagsToParams(params url.Values, tags *[]string) {
	if tags == nil || len(*tags) == 0 {
		return
	}
	positives, negatives := parseDelimitedValues(*tags)
	if len(positives) > 0 {
		params.Add("tags", strings.Join(positives, ","))
	}
	if len(negatives) > 0 {
		params.Add("not_tags", strings.Join(negatives, ","))
	}
}

// convertSpacesFromConstraints extracts spaces from constraints and converts
// them to two lists of positive and negative spaces.
func convertSpacesFromConstraints(spaces *[]string) ([]string, []string) {
	if spaces == nil || len(*spaces) == 0 {
		return nil, nil
	}
	return parseDelimitedValues(*spaces)
}

// parseDelimitedValues parses a slice of raw values coming from constraints
// (Tags or Spaces). The result is split into two slices - positives and
// negatives (prefixed with "^"). Empty values are ignored.
func parseDelimitedValues(rawValues []string) (positives, negatives []string) {
	for _, value := range rawValues {
		if value == "" || value == "^" {
			// Neither of these cases should happen in practise, as constraints
			// are validated before setting them and empty names for spaces or
			// tags are not allowed.
			continue
		}
		if strings.HasPrefix(value, "^") {
			negatives = append(negatives, strings.TrimPrefix(value, "^"))
		} else {
			positives = append(positives, value)
		}
	}
	return positives, negatives
}

func addInterfaces(params *gomaasapi.AllocateMachineArgs, positiveSpaceIDs, negativeSpaceIDs set.Strings) {
	if len(positiveSpaceIDs) > 0 {
		for _, providerSpaceID := range positiveSpaceIDs.SortedValues() {
			// NOTE(achilleasa): use the provider ID as the label for the
			// iface. Using the space name might seem to be more
			// user-friendly but space names can change after the machine
			// gets provisioned so they should not be used for labeling things.
			params.Interfaces = append(
				params.Interfaces,
				gomaasapi.InterfaceSpec{
					Label: providerSpaceID,
					Space: providerSpaceID,
				},
			)
		}
	}

	if len(negativeSpaceIDs) != 0 {
		params.NotSpace = negativeSpaceIDs.SortedValues()
	}
}

// addStorage adds volume information onto a gomaasapi.AllocateMachineArgs
// object suitable to pass to MAAS 2 when acquiring a node.
func addStorage(params *gomaasapi.AllocateMachineArgs, volumes []volumeInfo) {
	if len(volumes) == 0 {
		return
	}
	var volParams []gomaasapi.StorageSpec
	for _, v := range volumes {
		volSpec := gomaasapi.StorageSpec{
			Label: v.name,
			Size:  int(v.sizeInGB),
			Tags:  v.tags,
		}
		volParams = append(volParams, volSpec)
	}
	params.Storage = volParams
}
