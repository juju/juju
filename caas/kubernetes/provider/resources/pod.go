// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"
	"time"

	"github.com/juju/errors"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	types "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

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
func ListPods(ctx context.Context, client kubernetes.Interface, namespace string, opts metav1.ListOptions) ([]Pod, error) {
	api := client.CoreV1().Pods(namespace)
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
func (r *Pod) ID() ID {
	return ID{"Pod", r.Name, r.Namespace}
}

// Apply patches the resource change.
func (p *Pod) Apply(ctx context.Context, client kubernetes.Interface) error {
	api := client.CoreV1().Pods(p.Namespace)
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
	if err != nil {
		return errors.Trace(err)
	}
	p.Pod = *res
	return nil
}

// Get refreshes the resource.
func (p *Pod) Get(ctx context.Context, client kubernetes.Interface) error {
	api := client.CoreV1().Pods(p.Namespace)
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
func (p *Pod) Delete(ctx context.Context, client kubernetes.Interface) error {
	api := client.CoreV1().Pods(p.Namespace)
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
func (p *Pod) Events(ctx context.Context, client kubernetes.Interface) ([]corev1.Event, error) {
	return ListEventsForObject(ctx, client, p.Namespace, p.Name, "Pod")
}

// ComputeStatus returns a juju status for the resource.
func (p *Pod) ComputeStatus(ctx context.Context, client kubernetes.Interface, now time.Time) (string, status.Status, time.Time, error) {
	if p.DeletionTimestamp != nil {
		return "", status.Terminated, p.DeletionTimestamp.Time, nil
	}
	jujuStatus := status.Unknown
	switch p.Status.Phase {
	case corev1.PodRunning:
		jujuStatus = status.Running
	case corev1.PodFailed:
		jujuStatus = status.Error
	case corev1.PodPending:
		jujuStatus = status.Allocating
	}
	statusMessage := p.Status.Message
	since := now
	if statusMessage == "" {
		for _, cond := range p.Status.Conditions {
			statusMessage = cond.Message
			since = cond.LastProbeTime.Time
			if cond.Type == corev1.PodScheduled && cond.Reason == corev1.PodReasonUnschedulable {
				jujuStatus = status.Blocked
				break
			}
		}
	}
	if statusMessage == "" {
		// If there are any events for this pod we can use the
		// most recent to set the status.
		eventList, err := p.Events(ctx, client)
		if err != nil {
			return "", "", time.Time{}, errors.Trace(err)
		}
		// Take the most recent event.
		if count := len(eventList); count > 0 {
			statusMessage = eventList[count-1].Message
		}
	}
	return statusMessage, jujuStatus, since, nil
}
