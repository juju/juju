// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pod

import (
	core "k8s.io/api/core/v1"
)

// getPodCondition extracts the provided condition from the given status and returns that.
// Returns nil and -1 if the condition is not present, and the index of the located condition.
// These methods come directly from the Kubernetes code base. We can't import
// them as Kubernetes forbids this. Code can be found here:
// https://github.com/kubernetes/kubernetes/blob/12d9183da03d86c65f9f17e3e28be3c7c18ed22a/pkg/api/pod/util.go
func GetPodCondition(status *core.PodStatus, conditionType core.PodConditionType) (int, *core.PodCondition) {
	if status == nil {
		return -1, nil
	}
	return GetPodConditionFromList(status.Conditions, conditionType)
}

// getPodConditionFromList extracts the provided condition from the given list of condition and
// returns the index of the condition and the condition. Returns -1 and nil if the condition is not present.
// These methods come directly from the Kubernetes code base. We can't import
// them as Kubernetes forbids this. Code can be found here:
// https://github.com/kubernetes/kubernetes/blob/12d9183da03d86c65f9f17e3e28be3c7c18ed22a/pkg/api/pod/util.go
func GetPodConditionFromList(conditions []core.PodCondition, conditionType core.PodConditionType) (int, *core.PodCondition) {
	if conditions == nil {
		return -1, nil
	}
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return i, &conditions[i]
		}
	}
	return -1, nil
}

// IsPodRunning checks all the conditions of a pod to make sure PodReady is true
func IsPodRunning(pod *core.Pod) bool {
	_, cond := GetPodCondition(&pod.Status, core.PodReady)
	if cond == nil {
		return false
	}
	return cond.Status == core.ConditionTrue
}
