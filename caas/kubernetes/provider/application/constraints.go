// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/constraints"
)

// ConstraintApplier defines the function type for configuring constraint for a pod.
type ConstraintApplier func(pod *core.PodSpec, resourceName core.ResourceName, value string) error

// ApplyConstraints applies the specified constraints to the pod.
func ApplyConstraints(pod *core.PodSpec, appName string, cons constraints.Value, configureConstraint ConstraintApplier) error {
	// TODO(allow resource limits to be applied to each container).
	// For now we only do resource requests, one container is sufficient for
	// scheduling purposes.
	if mem := cons.Mem; mem != nil {
		if err := configureConstraint(pod, core.ResourceMemory, fmt.Sprintf("%dMi", *mem)); err != nil {
			return errors.Annotatef(err, "configuring memory constraint for %s", appName)
		}
	}
	if cpu := cons.CpuPower; cpu != nil {
		if err := configureConstraint(pod, core.ResourceCPU, fmt.Sprintf("%dm", *cpu)); err != nil {
			return errors.Annotatef(err, "configuring cpu constraint for %s", appName)
		}
	}
	nodeSelector := map[string]string(nil)
	if cons.HasArch() {
		cpuArch := *cons.Arch
		cpuArch = arch.NormaliseArch(cpuArch)
		// Convert to Golang arch string
		switch cpuArch {
		case arch.AMD64:
			cpuArch = "amd64"
		case arch.ARM64:
			cpuArch = "arm64"
		case arch.PPC64EL:
			cpuArch = "ppc64le"
		case arch.S390X:
			cpuArch = "s390x"
		default:
			return errors.NotSupportedf("architecture %q", cpuArch)
		}
		nodeSelector = map[string]string{"kubernetes.io/arch": cpuArch}
	}
	if pod.NodeSelector != nil {
		for k, v := range nodeSelector {
			pod.NodeSelector[k] = v
		}
	} else if nodeSelector != nil {
		pod.NodeSelector = nodeSelector
	}

	// Translate tags to pod or node affinity.
	// Tag names are prefixed with "pod.", "anti-pod.", or "node."
	// with the default being "node".
	// The tag 'topology-key', if set, is used for the affinity topology key value.
	if cons.Tags != nil {
		affinityLabels := make(map[string]string)
		for _, labelPair := range *cons.Tags {
			parts := strings.Split(labelPair, "=")
			if len(parts) != 2 {
				return errors.Errorf("invalid affinity constraints: %v", affinityLabels)
			}
			key := strings.Trim(parts[0], " ")
			value := strings.Trim(parts[1], " ")
			affinityLabels[key] = value
		}

		if err := processNodeAffinity(pod, affinityLabels); err != nil {
			return errors.Annotatef(err, "configuring node affinity for %s", appName)
		}
		if err := processPodAffinity(pod, affinityLabels); err != nil {
			return errors.Annotatef(err, "configuring pod affinity for %s", appName)
		}
		if err := processTopologySpreadConstraints(pod, affinityLabels, appName); err != nil {
			return errors.Annotatef(err, "configuring topology spread constraints for %s", appName)
		}

	}
	if cons.Zones != nil {
		zones := *cons.Zones
		affinity := pod.Affinity
		if affinity == nil {
			affinity = &core.Affinity{
				NodeAffinity: &core.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &core.NodeSelector{
						NodeSelectorTerms: []core.NodeSelectorTerm{{}},
					},
				},
			}
			pod.Affinity = affinity
		}
		selector := &affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0]
		selector.MatchExpressions = append(selector.MatchExpressions,
			core.NodeSelectorRequirement{
				Key:      "topology.kubernetes.io/zone",
				Operator: core.NodeSelectorOpIn,
				Values:   zones,
			})
	}
	return nil
}

const (
	PodPrefix            = "pod."
	AntiPodPrefix        = "anti-pod."
	TopologyKeyTag       = "topology-key"
	NodePrefix           = "node."
	TopologySpreadPrefix = "topology-spread."
)

func processNodeAffinity(pod *core.PodSpec, affinityLabels map[string]string) error {
	affinityTags := make(map[string]string)
	for key, value := range affinityLabels {
		keyVal := key
		if strings.HasPrefix(key, "^") {
			if len(key) == 1 {
				return errors.Errorf("invalid affinity constraints: %v", affinityLabels)
			}
			key = key[1:]
		}
		if !strings.HasPrefix(key, NodePrefix) {
			continue
		}
		key = strings.TrimPrefix(keyVal, NodePrefix)
		affinityTags[key] = value
	}

	updateSelectorTerms := func(nodeSelectorTerm *core.NodeSelectorTerm, tags map[string]string) {
		// Sort for stable ordering.
		var keys []string
		for k := range tags {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, tag := range keys {
			allValues := strings.Split(tags[tag], "|")
			for i, v := range allValues {
				allValues[i] = strings.Trim(v, " ")
			}
			op := core.NodeSelectorOpIn
			if strings.HasPrefix(tag, "^") {
				tag = tag[1:]
				op = core.NodeSelectorOpNotIn
			}
			nodeSelectorTerm.MatchExpressions = append(nodeSelectorTerm.MatchExpressions, core.NodeSelectorRequirement{
				Key:      tag,
				Operator: op,
				Values:   allValues,
			})
		}
	}
	var nodeSelectorTerm core.NodeSelectorTerm
	updateSelectorTerms(&nodeSelectorTerm, affinityTags)
	if len(nodeSelectorTerm.MatchExpressions) > 0 {
		if pod.Affinity == nil {
			pod.Affinity = &core.Affinity{}
		}
		pod.Affinity.NodeAffinity = &core.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &core.NodeSelector{
				NodeSelectorTerms: []core.NodeSelectorTerm{nodeSelectorTerm},
			},
		}
	}
	return nil
}

func processPodAffinity(pod *core.PodSpec, affinityLabels map[string]string) error {
	affinityTags := make(map[string]string)
	antiAffinityTags := make(map[string]string)
	for key, value := range affinityLabels {
		notVal := false
		if strings.HasPrefix(key, "^") {
			if len(key) == 1 {
				return errors.Errorf("invalid affinity constraints: %v", affinityLabels)
			}
			notVal = true
			key = key[1:]
		}
		if !strings.HasPrefix(key, PodPrefix) && !strings.HasPrefix(key, AntiPodPrefix) {
			continue
		}
		if strings.HasPrefix(key, PodPrefix) {
			key = strings.TrimPrefix(key, PodPrefix)
			if notVal {
				key = "^" + key
			}
			affinityTags[key] = value
		}
		if strings.HasPrefix(key, AntiPodPrefix) {
			key = strings.TrimPrefix(key, AntiPodPrefix)
			if notVal {
				key = "^" + key
			}
			antiAffinityTags[key] = value
		}
	}
	if len(affinityTags) == 0 && len(antiAffinityTags) == 0 {
		return nil
	}

	updateAffinityTerm := func(affinityTerm *core.PodAffinityTerm, tags map[string]string) {
		// Sort for stable ordering.
		var keys []string
		for k := range tags {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var (
			labelSelector v1.LabelSelector
			topologyKey   string
		)
		for _, tag := range keys {
			if tag == TopologyKeyTag {
				topologyKey = tags[tag]
				continue
			}
			allValues := strings.Split(tags[tag], "|")
			for i, v := range allValues {
				allValues[i] = strings.Trim(v, " ")
			}
			op := v1.LabelSelectorOpIn
			if strings.HasPrefix(tag, "^") {
				tag = tag[1:]
				op = v1.LabelSelectorOpNotIn
			}
			labelSelector.MatchExpressions = append(labelSelector.MatchExpressions, v1.LabelSelectorRequirement{
				Key:      tag,
				Operator: op,
				Values:   allValues,
			})
		}
		affinityTerm.LabelSelector = &labelSelector
		if topologyKey != "" {
			affinityTerm.TopologyKey = topologyKey
		}
	}
	var affinityTerm core.PodAffinityTerm
	updateAffinityTerm(&affinityTerm, affinityTags)
	if len(affinityTerm.LabelSelector.MatchExpressions) > 0 || affinityTerm.TopologyKey != "" {
		if pod.Affinity == nil {
			pod.Affinity = &core.Affinity{}
		}
		pod.Affinity.PodAffinity = &core.PodAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []core.PodAffinityTerm{affinityTerm},
		}
	}

	var antiAffinityTerm core.PodAffinityTerm
	updateAffinityTerm(&antiAffinityTerm, antiAffinityTags)
	if len(antiAffinityTerm.LabelSelector.MatchExpressions) > 0 || antiAffinityTerm.TopologyKey != "" {
		if pod.Affinity == nil {
			pod.Affinity = &core.Affinity{}
		}
		pod.Affinity.PodAntiAffinity = &core.PodAntiAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []core.PodAffinityTerm{antiAffinityTerm},
		}
	}
	return nil
}

const (
	topologySpreadMaxSkew         = "maxSkew"
	topologySpreadMinDomains      = "minDomains"
	topologySpreadNodeTaintPolicy = "nodeTaintsPolicy"
	topologySpreadMatchLabels     = "matchLabelKeys"
)

func processTopologySpreadConstraints(pod *core.PodSpec, affinityLabels map[string]string, appName string) error {
	topologySpreadValues := make(map[string]string)

	validKeys := set.NewStrings(
		topologySpreadMaxSkew,
		topologySpreadMinDomains,
		topologySpreadNodeTaintPolicy,
		topologySpreadMatchLabels,
		TopologyKeyTag,
	)

	for key, value := range affinityLabels {
		if !strings.HasPrefix(key, TopologySpreadPrefix) {
			continue
		}

		key = strings.TrimPrefix(key, TopologySpreadPrefix)
		if !validKeys.Contains(key) {
			return errors.Errorf("invalid topology spread constraint key %q", key)
		}
		if key == topologySpreadMaxSkew {
			val, err := strconv.Atoi(value)
			if err != nil {
				return errors.Errorf("invalid value %q for topology spread max skew", value)
			}
			if val < 1 {
				return errors.Errorf("invalid value, maxSkew must be greater or equal to 1")
			}
		}
		topologySpreadValues[key] = value
	}
	if len(topologySpreadValues) == 0 {
		return nil
	}
	_, present := topologySpreadValues[TopologyKeyTag]
	if !present {
		return errors.Errorf("topology-key not set for topology spread constraints: %v", affinityLabels)
	}

	var appNameLabel string = "app.kubernetes.io/name"

	updateTopologyTerm := func(topologyTerms *core.TopologySpreadConstraint, tags map[string]string) {
		// Sort for stable ordering.
		var keys []string
		for k := range tags {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var (
			labelSelector    v1.LabelSelector
			topologyKey      string = "kubernetes.io/zone"
			maxSkew          int    = 1
			minDomains       int    = 3
			nodeTaintsPolicy *core.NodeInclusionPolicy
		)

		for _, key := range keys {

			if key == TopologyKeyTag {
				topologyKey = tags[TopologyKeyTag]
				continue
			}
			if key == topologySpreadMinDomains {
				domains, ok := strconv.Atoi(tags[topologySpreadMinDomains])
				if ok != nil || domains < 2 {
					minDomains = 2
				} else {
					minDomains = domains
				}
				continue
			}
			if key == topologySpreadMaxSkew {
				skew, ok := strconv.Atoi(tags[topologySpreadMaxSkew])
				if ok != nil || skew < 1 {
					maxSkew = 1
				} else {
					maxSkew = skew
				}
				continue
			}

			if key == topologySpreadNodeTaintPolicy {
				taintPolicy, err := strconv.ParseBool(tags[key])
				if err != nil {
					taintPolicy = true
				}
				if taintPolicy {
					honorPolicy := core.NodeInclusionPolicy("Honor")
					nodeTaintsPolicy = &honorPolicy
				} else {
					ignorePolicy := core.NodeInclusionPolicy("Ignore")
					nodeTaintsPolicy = &ignorePolicy
				}
				continue
			}

			if key == appNameLabel {
				// Nothing to do, we will add this key anyways
				continue
			}

			allValues := strings.Split(tags[key], "|")
			for i, v := range allValues {
				allValues[i] = strings.Trim(v, " ")
			}
			op := v1.LabelSelectorOpIn
			if strings.HasPrefix(key, "^") {
				key = key[1:]
				op = v1.LabelSelectorOpNotIn
			}
			labelSelector.MatchExpressions = append(labelSelector.MatchExpressions, v1.LabelSelectorRequirement{
				Key:      key,
				Operator: op,
				Values:   allValues,
			})
		}
		if topologyKey != "" {
			topologyTerms.TopologyKey = topologyKey
		}
		topologyTerms.WhenUnsatisfiable = core.DoNotSchedule
		topologyTerms.MaxSkew = int32(maxSkew)
		minimumDomains := int32(minDomains)
		topologyTerms.MinDomains = &minimumDomains

		honorPolicy := core.NodeInclusionPolicy("Honor")
		if nodeTaintsPolicy != nil {
			topologyTerms.NodeTaintsPolicy = nodeTaintsPolicy
		} else {
			topologyTerms.NodeTaintsPolicy = &honorPolicy
		}
		topologyTerms.NodeAffinityPolicy = &honorPolicy
		topologyTerms.LabelSelector = &labelSelector
	}

	var topologyTerm core.TopologySpreadConstraint
	updateTopologyTerm(&topologyTerm, topologySpreadValues)

	topologyTerm.LabelSelector.MatchExpressions = append(topologyTerm.LabelSelector.MatchExpressions, v1.LabelSelectorRequirement{
		Key:      appNameLabel,
		Operator: v1.LabelSelectorOpIn,
		Values:   []string{appName},
	})

	if len(topologyTerm.LabelSelector.MatchExpressions) > 0 || topologyTerm.TopologyKey != "" {
		if pod.TopologySpreadConstraints == nil {
			pod.TopologySpreadConstraints = make([]core.TopologySpreadConstraint, 0)
		}
		pod.TopologySpreadConstraints = append(pod.TopologySpreadConstraints, topologyTerm)
	}
	return nil
}

func configureConstraint(pod *core.PodSpec, resourceName core.ResourceName, value string) (err error) {
	if len(pod.Containers) == 0 {
		return nil
	}
	pod.Containers[0].Resources.Requests, err = MergeConstraint(resourceName, value, pod.Containers[0].Resources.Requests)
	if err != nil {
		return errors.Annotatef(err, "merging request constraint %s=%s", resourceName, value)
	}
	for i := range pod.Containers {
		pod.Containers[i].Resources.Limits, err = MergeConstraint(resourceName, value, pod.Containers[i].Resources.Limits)
		if err != nil {
			return errors.Annotatef(err, "merging limit constraint %s=%s", resourceName, value)
		}
	}
	return nil
}

// MergeConstraint merges constraint spec.
func MergeConstraint(resourceName core.ResourceName, value string, resourcesList core.ResourceList) (core.ResourceList, error) {
	if resourcesList == nil {
		resourcesList = core.ResourceList{}
	}
	if v, ok := resourcesList[resourceName]; ok {
		return nil, errors.NotValidf("resource list for %q has already been set to %v", resourceName, v)
	}
	parsedValue, err := resource.ParseQuantity(value)
	if err != nil {
		return nil, errors.Annotatef(err, "invalid constraint value %q for %q", value, resourceName)
	}
	resourcesList[resourceName] = parsedValue
	return resourcesList, nil
}
