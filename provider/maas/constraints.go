// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/gomaasapi"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs/context"
)

var unsupportedConstraints = []string{
	constraints.CpuPower,
	constraints.InstanceType,
	constraints.VirtType,
}

// ConstraintsValidator is defined on the Environs interface.
func (env *maasEnviron) ConstraintsValidator(ctx context.ProviderCallContext) (constraints.Validator, error) {
	validator := constraints.NewValidator()
	validator.RegisterUnsupported(unsupportedConstraints)
	supportedArches, err := env.getSupportedArchitectures(ctx)
	if err != nil {
		return nil, err
	}
	validator.RegisterVocabulary(constraints.Arch, supportedArches)
	return validator, nil
}

// convertConstraints converts the given constraints into an url.Values object
// suitable to pass to MAAS when acquiring a node. CpuPower is ignored because
// it cannot be translated into something meaningful for MAAS right now.
func convertConstraints(cons constraints.Value) url.Values {
	params := url.Values{}
	if cons.Arch != nil {
		// Note: Juju and MAAS use the same architecture names.
		// MAAS also accepts a subarchitecture (e.g. "highbank"
		// for ARM), which defaults to "generic" if unspecified.
		params.Add("arch", *cons.Arch)
	}
	if cons.CpuCores != nil {
		params.Add("cpu_count", fmt.Sprintf("%d", *cons.CpuCores))
	}
	if cons.Mem != nil {
		params.Add("mem", fmt.Sprintf("%d", *cons.Mem))
	}
	convertTagsToParams(params, cons.Tags)
	if cons.CpuPower != nil {
		logger.Warningf("ignoring unsupported constraint 'cpu-power'")
	}
	return params
}

// convertConstraints2 converts the given constraints into a
// gomaasapi.AllocateMachineArgs for passing to MAAS 2.
func convertConstraints2(cons constraints.Value) gomaasapi.AllocateMachineArgs {
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
		logger.Warningf("ignoring unsupported constraint 'cpu-power'")
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

// interfaceBinding defines a requirement that a node interface must satisfy in
// order for that node to get selected and started, based on deploy-time
// bindings of a service.
//
// TODO(dimitern): Once the services have bindings defined in state, a version
// of this should go to the network package (needs to be non-MAAS-specifc
// first). Also, we need to transform Juju space names from constraints into
// MAAS space provider IDs.
type interfaceBinding struct {
	Name            string
	SpaceProviderId string

	// add more as needed.
}

// numericLabelLimit is a sentinel value used in addInterfaces to limit the
// number of disambiguation inner loop iterations in case named labels clash
// with numeric labels for spaces coming from constraints. It's defined here to
// facilitate testing this behavior.
var numericLabelLimit uint = 0xffff

// addInterfaces converts a slice of interface bindings, positiveSpaces and
// negativeSpaces coming from constraints to the format MAAS expects for the
// "interfaces" and "not_networks" arguments to acquire node. Returns an error
// satisfying errors.IsNotValid() if the bindings contains duplicates, empty
// Name/SpaceProviderId, or if negative spaces clash with specified bindings.
// Duplicates between specified bindings and positiveSpaces are silently
// skipped.
func addInterfaces(
	params url.Values,
	bindings []interfaceBinding,
	positiveSpaces, negativeSpaces []network.SpaceInfo,
) error {
	combinedBindings, negatives, err := getBindings(bindings, positiveSpaces, negativeSpaces)
	if err != nil {
		return errors.Trace(err)
	}
	if len(combinedBindings) > 0 {
		combinedBindingsString := make([]string, len(combinedBindings))
		for i, binding := range combinedBindings {
			combinedBindingsString[i] = fmt.Sprintf("%s:space=%s", binding.Name, binding.SpaceProviderId)
		}
		params.Add("interfaces", strings.Join(combinedBindingsString, ";"))
	}
	if len(negatives) > 0 {
		for _, binding := range negatives {
			notNetwork := fmt.Sprintf("space:%s", binding.SpaceProviderId)
			params.Add("not_networks", notNetwork)
		}
	}
	return nil
}

func getBindings(
	bindings []interfaceBinding,
	positiveSpaces, negativeSpaces []network.SpaceInfo,
) ([]interfaceBinding, []interfaceBinding, error) {
	var (
		index            uint
		combinedBindings []interfaceBinding
	)
	namesSet := set.NewStrings()
	spacesSet := set.NewStrings()
	createLabel := func(index uint, namesSet set.Strings) (string, uint, error) {
		var label string
		for {
			label = fmt.Sprintf("%v", index)
			if !namesSet.Contains(label) {
				break
			}
			if index > numericLabelLimit { // ...just to make sure we won't loop forever.
				return "", index, errors.Errorf("too many conflicting numeric labels, giving up.")
			}
			index++
		}
		namesSet.Add(label)
		return label, index, nil
	}
	for _, binding := range bindings {
		switch {
		case binding.SpaceProviderId == "":
			return nil, nil, errors.NewNotValid(nil, fmt.Sprintf(
				"invalid interface binding %q: space provider ID is required",
				binding.Name,
			))
		case binding.Name == "":
			var label string
			var err error
			label, index, err = createLabel(index, namesSet)
			if err != nil {
				return nil, nil, errors.Trace(err)
			}
			binding.Name = label
		case namesSet.Contains(binding.Name):
			return nil, nil, errors.NewNotValid(nil, fmt.Sprintf(
				"duplicated interface binding %q",
				binding.Name,
			))
		}
		namesSet.Add(binding.Name)
		spacesSet.Add(binding.SpaceProviderId)

		combinedBindings = append(combinedBindings, binding)
	}

	for _, space := range positiveSpaces {
		if spacesSet.Contains(string(space.ProviderId)) {
			// Skip duplicates in positiveSpaces.
			continue
		}
		spacesSet.Add(string(space.ProviderId))

		var label string
		var err error
		label, index, err = createLabel(index, namesSet)
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
		// Make sure we pick a label that doesn't clash with possible bindings.
		combinedBindings = append(combinedBindings, interfaceBinding{label, string(space.ProviderId)})
	}

	var negatives []interfaceBinding
	for _, space := range negativeSpaces {
		if spacesSet.Contains(string(space.ProviderId)) {
			return nil, nil, errors.NewNotValid(nil, fmt.Sprintf(
				"negative space %q from constraints clashes with interface bindings",
				space.Name,
			))
		}
		var label string
		var err error
		label, index, err = createLabel(index, namesSet)
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
		negatives = append(negatives, interfaceBinding{label, string(space.ProviderId)})
	}
	return combinedBindings, negatives, nil
}

func addInterfaces2(
	params *gomaasapi.AllocateMachineArgs,
	bindings []interfaceBinding,
	positiveSpaces, negativeSpaces []network.SpaceInfo,
) error {
	combinedBindings, negatives, err := getBindings(bindings, positiveSpaces, negativeSpaces)
	if err != nil {
		return errors.Trace(err)
	}

	if len(combinedBindings) > 0 {
		interfaceSpecs := make([]gomaasapi.InterfaceSpec, len(combinedBindings))
		for i, space := range combinedBindings {
			interfaceSpecs[i] = gomaasapi.InterfaceSpec{Label: space.Name, Space: space.SpaceProviderId}
		}
		params.Interfaces = interfaceSpecs
	}
	if len(negatives) > 0 {
		negativeStrings := make([]string, len(negatives))
		for i, space := range negatives {
			negativeStrings[i] = space.SpaceProviderId
		}
		params.NotSpace = negativeStrings
	}
	return nil
}

// addStorage converts volume information into url.Values object suitable to
// pass to MAAS when acquiring a node.
func addStorage(params url.Values, volumes []volumeInfo) {
	if len(volumes) == 0 {
		return
	}
	// Requests for specific values are passed to the acquire URL
	// as a storage URL parameter of the form:
	// [volume-name:]sizeinGB[tag,...]
	// See http://maas.ubuntu.com/docs/api.html#nodes

	// eg storage=root:0(ssd),data:20(magnetic,5400rpm),45
	makeVolumeParams := func(v volumeInfo) string {
		var params string
		if v.name != "" {
			params = v.name + ":"
		}
		params += fmt.Sprintf("%d", v.sizeInGB)
		if len(v.tags) > 0 {
			params += fmt.Sprintf("(%s)", strings.Join(v.tags, ","))
		}
		return params
	}
	var volParms []string
	for _, v := range volumes {
		params := makeVolumeParams(v)
		volParms = append(volParms, params)
	}
	params.Add("storage", strings.Join(volParms, ","))
}

// addStorage2 adds volume information onto a gomaasapi.AllocateMachineArgs
// object suitable to pass to MAAS 2 when acquiring a node.
func addStorage2(params *gomaasapi.AllocateMachineArgs, volumes []volumeInfo) {
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
