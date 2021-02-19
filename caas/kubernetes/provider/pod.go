// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	core "k8s.io/api/core/v1"

	k8spod "github.com/juju/juju/caas/kubernetes/pod"
	"github.com/juju/juju/core/status"
)

type EventGetter func() ([]core.Event, error)

const (
	PodReasonCompleted                = "Completed"
	PodReasonContainerCreating        = "ContainerCreating"
	PodReasonContainersNotInitialized = "ContainersNotInitialized"
	PodReasonContainersNotReady       = "ContainersNotReady"
	PodReasonCrashLoopBackoff         = "CrashLoopBackOff"
	PodReasonError                    = "Error"
	PodReasonImagePull                = "ErrImagePull"
	PodReasonInitializing             = "PodInitializing"
)

var (
	podContainersReadyReasonsMap = map[string]status.Status{
		PodReasonContainersNotReady: status.Maintenance,
	}

	podInitializedReasonsMap = map[string]status.Status{
		PodReasonContainersNotInitialized: status.Maintenance,
	}

	podReadyReasonMap = map[string]status.Status{
		PodReasonContainersNotReady:       status.Maintenance,
		PodReasonContainersNotInitialized: status.Maintenance,
	}

	podScheduledReasonsMap = map[string]status.Status{
		core.PodReasonUnschedulable: status.Blocked,
	}
)

// podToJujuStatus takes a Kubernetes pod and translate's it to a known Juju
// status. If this function can't determine the reason for a pod's state either
// a status of error or unknown is returned. Function returns the status message,
// juju status, the time of the status event and any errors that occurred.
func podToJujuStatus(
	pod core.Pod,
	now time.Time,
	events EventGetter,
) (string, status.Status, time.Time, error) {
	since := now
	defaultStatusMessage := pod.Status.Message

	if pod.DeletionTimestamp != nil {
		return defaultStatusMessage, status.Terminated, since, nil
	}

	// conditionHandler tries to handle the state of the supplied condition.
	// if the condition status is true true is returned from this function.
	// Otherwise if the condition is unknown or false the function attempts to
	// map the condition reason onto a known juju status
	conditionHandler := func(
		pc *core.PodCondition,
		reasonMapper func(reason string) status.Status,
	) (bool, status.Status, string) {
		if pc.Status == core.ConditionTrue {
			return true, "", ""
		} else if pc.Status == core.ConditionUnknown {
			return false, status.Unknown, pc.Message
		}
		return false, reasonMapper(pc.Reason), pc.Message
	}

	// reasonMapper takes a mapping of Kubernetes pod reasons to juju statuses.
	// If no reason is found in the map the default reason supplied is returned
	reasonMapper := func(
		reasons map[string]status.Status,
		def status.Status) func(string) status.Status {
		return func(r string) status.Status {
			if stat, ok := reasons[r]; ok {
				return stat
			}
			return def
		}
	}

	// Start by processing the pod conditions in their lifecycle order
	// Has the pod been scheduled?
	_, cond := k8spod.GetPodCondition(&pod.Status, core.PodScheduled)
	if cond == nil { // Doesn't have scheduling information. Should not get here
		return defaultStatusMessage, status.Unknown, since, nil
	} else if r, s, m := conditionHandler(
		cond, reasonMapper(podScheduledReasonsMap, status.Allocating)); !r {
		return m, s, cond.LastProbeTime.Time, nil
	}

	// Have the init containers run?
	if _, cond := k8spod.GetPodCondition(&pod.Status, core.PodInitialized); cond != nil {
		r, s, m := conditionHandler(
			cond, reasonMapper(podInitializedReasonsMap, status.Maintenance))
		if errM, isErr := interrogatePodContainerStatus(pod.Status.InitContainerStatuses); !r && isErr {
			return errM, status.Error, cond.LastProbeTime.Time, nil
		} else if !r {
			return m, s, cond.LastProbeTime.Time, nil
		}
	}

	// Have the containers started/finished?
	_, cond = k8spod.GetPodCondition(&pod.Status, core.ContainersReady)
	if cond == nil {
		return defaultStatusMessage, status.Unknown, since, nil
	} else if r, s, m := conditionHandler(
		cond, reasonMapper(podContainersReadyReasonsMap, status.Maintenance)); !r {
		if errM, isErr := interrogatePodContainerStatus(pod.Status.ContainerStatuses); isErr {
			return errM, status.Error, cond.LastProbeTime.Time, nil
		}
		return m, s, cond.LastProbeTime.Time, nil
	}

	// Made it this far are we ready?
	_, cond = k8spod.GetPodCondition(&pod.Status, core.PodReady)
	if cond == nil {
		return defaultStatusMessage, status.Unknown, since, nil
	} else if r, s, m := conditionHandler(
		cond, reasonMapper(podReadyReasonMap, status.Maintenance)); !r {
		return m, s, cond.LastProbeTime.Time, nil
	} else if r {
		return "", status.Running, since, nil
	}

	// If we have made it this far then something is very wrong in the state
	// of the pod.

	// If we can't find a status message lets take a look at the event log
	if defaultStatusMessage == "" {
		eventList, err := events()
		if err != nil {
			return "", "", time.Time{}, errors.Trace(err)
		}

		if count := len(eventList); count > 0 {
			defaultStatusMessage = eventList[count-1].Message
		}
	}

	return defaultStatusMessage, status.Unknown, since, nil
}

// interrogatePodContainerStatus combs a set of container statuses. If a
// container is found to be in an error state it's error message and true are
// returned, Otherwise an empty message and false
func interrogatePodContainerStatus(containers []core.ContainerStatus) (string, bool) {
	for _, c := range containers {
		if c.State.Running != nil {
			continue
		}

		if c.State.Waiting != nil {
			m, isError := isContainerReasonError(c.State.Waiting.Reason)
			if isError {
				m = fmt.Sprintf("%s: %s", m, c.State.Waiting.Message)
			}
			return m, isError
		}

		if c.State.Terminated != nil {
			m, isError := isContainerReasonError(c.State.Terminated.Reason)
			if isError {
				m = fmt.Sprintf("%s: %s", m, c.State.Terminated.Message)
			}
			return m, isError
		}
	}
	return "", false
}

// isContainerReasonError decides if a reason on a container status is
// considered to be an error. If an error is found on the reason then a
// description of the error is returned with true. Otherwise an empty
// description and false.
func isContainerReasonError(reason string) (string, bool) {
	switch reason {
	case PodReasonContainerCreating:
		return "creating pod container(s)", false
	case PodReasonError:
		return "container error", true
	case PodReasonImagePull:
		return "OCI image pull error", true
	case PodReasonCrashLoopBackoff:
		return "crash loop backoff", true
	case PodReasonCompleted:
		return "", false
	case PodReasonInitializing:
		return "pod initializing", false
	default:
		return fmt.Sprintf("unknown container reason %q", reason), true
	}
}
