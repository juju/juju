// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	types "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	k8spod "github.com/juju/juju/caas/kubernetes/pod"
	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/core/status"
)

// Pod extends the k8s service.
type Pod struct {
	corev1.Pod
}

// NewPod creates a new service resource.
func NewPod(name string, namespace string, in *corev1.Pod) *Pod {
	if in == nil {
		in = &corev1.Pod{}
	}
	in.SetName(name)
	in.SetNamespace(namespace)
	return &Pod{*in}
}

// ListPods returns a list of Pods.
func ListPods(ctx context.Context, coreClient kubernetes.Interface, namespace string, opts metav1.ListOptions) ([]Pod, error) {
	api := coreClient.CoreV1().Pods(namespace)
	var items []Pod
	for {
		res, err := api.List(ctx, opts)
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, v := range res.Items {
			items = append(items, Pod{Pod: v})
		}
		if res.RemainingItemCount == nil || *res.RemainingItemCount == 0 {
			break
		}
		opts.Continue = res.Continue
	}
	return items, nil
}

// Clone returns a copy of the resource.
func (p *Pod) Clone() Resource {
	clone := *p
	return &clone
}

// ID returns a comparable ID for the Resource
func (p *Pod) ID() ID {
	return ID{"Pod", p.Name, p.Namespace}
}

// Apply patches the resource change.
func (p *Pod) Apply(ctx context.Context, coreClient kubernetes.Interface, extendedClient clientset.Interface) error {
	api := coreClient.CoreV1().Pods(p.Namespace)
	data, err := runtime.Encode(unstructured.UnstructuredJSONScheme, &p.Pod)
	if err != nil {
		return errors.Trace(err)
	}
	res, err := api.Patch(ctx, p.Name, types.StrategicMergePatchType, data, metav1.PatchOptions{
		FieldManager: JujuFieldManager,
	})
	if k8serrors.IsNotFound(err) {
		res, err = api.Create(ctx, &p.Pod, metav1.CreateOptions{
			FieldManager: JujuFieldManager,
		})
	}
	if k8serrors.IsConflict(err) {
		return errors.Annotatef(errConflict, "pod %q", p.Name)
	}
	if err != nil {
		return errors.Trace(err)
	}
	p.Pod = *res
	return nil
}

// Get refreshes the resource.
func (p *Pod) Get(ctx context.Context, coreClient kubernetes.Interface, extendedClient clientset.Interface) error {
	api := coreClient.CoreV1().Pods(p.Namespace)
	res, err := api.Get(ctx, p.Name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return errors.NewNotFound(err, "k8s")
	} else if err != nil {
		return errors.Trace(err)
	}
	p.Pod = *res
	return nil
}

// Delete removes the resource.
func (p *Pod) Delete(ctx context.Context, coreClient kubernetes.Interface, extendedClient clientset.Interface) error {
	api := coreClient.CoreV1().Pods(p.Namespace)
	err := api.Delete(ctx, p.Name, metav1.DeleteOptions{
		PropagationPolicy: k8sconstants.DefaultPropagationPolicy(),
	})
	if k8serrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// Events emitted by the resource.
func (p *Pod) Events(ctx context.Context, coreClient kubernetes.Interface) ([]corev1.Event, error) {
	return ListEventsForObject(ctx, coreClient, p.Namespace, p.Name, "Pod")
}

// ComputeStatus returns a juju status for the resource.
func (p *Pod) ComputeStatus(ctx context.Context, coreClient kubernetes.Interface, now time.Time) (string, status.Status, time.Time, error) {
	return PodToJujuStatus(p.Pod, now, func() ([]corev1.Event, error) { return p.Events(ctx, coreClient) })
}

type EventGetter func() ([]corev1.Event, error)

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
		corev1.PodReasonUnschedulable: status.Blocked,
	}
)

// PodToJujuStatus takes a kubernetes.Interface pod and translates it to a known Juju
// status. If this function can't determine the reason for a pod's state either
// a status of error or unknown is returned. Function returns the status message,
// juju status, the time of the status event and any errors that occurred.
func PodToJujuStatus(
	pod corev1.Pod,
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
		pc *corev1.PodCondition,
		reasonMapper func(reason string) status.Status,
	) (bool, status.Status, string) {
		if pc.Status == corev1.ConditionTrue {
			return true, "", ""
		} else if pc.Status == corev1.ConditionUnknown {
			return false, status.Unknown, pc.Message
		}
		return false, reasonMapper(pc.Reason), pc.Message
	}

	// reasonMapper takes a mapping of kubernetes.Interface pod reasons to juju statuses.
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
	_, cond := k8spod.GetPodCondition(&pod.Status, corev1.PodScheduled)
	if cond == nil {
		// Doesn't have scheduling information. Should not get here.
		return defaultStatusMessage, status.Unknown, since, nil
	} else if r, s, m := conditionHandler(cond, reasonMapper(podScheduledReasonsMap, status.Allocating)); !r {
		return m, s, cond.LastProbeTime.Time, nil
	}

	// Have the init containers run?
	if _, cond := k8spod.GetPodCondition(&pod.Status, corev1.PodInitialized); cond != nil {
		r, s, m := conditionHandler(cond, reasonMapper(podInitializedReasonsMap, status.Maintenance))
		if errM, isErr := interrogatePodContainerStatus(pod.Status.InitContainerStatuses); !r && isErr {
			return errM, status.Error, cond.LastProbeTime.Time, nil
		} else if !r {
			return m, s, cond.LastProbeTime.Time, nil
		}
	}

	// Have the containers started/finished?
	_, cond = k8spod.GetPodCondition(&pod.Status, corev1.ContainersReady)
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
	_, cond = k8spod.GetPodCondition(&pod.Status, corev1.PodReady)
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
// container is found to be in an error state, its error message and true are
// returned, Otherwise an empty message and false.
func interrogatePodContainerStatus(containers []corev1.ContainerStatus) (string, bool) {
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
