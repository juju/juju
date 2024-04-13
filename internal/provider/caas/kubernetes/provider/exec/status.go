// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package exec

import (
	"context"
	"time"

	"github.com/juju/errors"
	core "k8s.io/api/core/v1"
)

// StatusParams holds all the necessary parameters to query Pod status
type StatusParams struct {
	PodName string
}

func (p *StatusParams) validate() error {
	if p.PodName == "" {
		return errors.New("pod name not specified")
	}
	return nil
}

// Status of a pod.
type Status struct {
	PodName string

	ContainerStatus []ContainerStatus
}

// ContainerStatus describes status of one container inside a pod.
type ContainerStatus struct {
	// Name of the container
	Name string

	// Waiting state
	Waiting bool
	// Running state
	Running bool
	// Terminated state
	Terminated bool

	// StartedAt is filled when the container is running or terminated.
	StartedAt time.Time

	// InitContainer is true when the container is apart of the
	// init phase.
	InitContainer bool
	// EphemeralContainer is true when the container is ephemeral.
	EphemeralContainer bool
}

// Status returns information about a Pod including the status
// of each container.
func (c client) Status(ctx context.Context, params StatusParams) (*Status, error) {
	if err := params.validate(); err != nil {
		return nil, errors.Trace(err)
	}
	pod, err := getValidatedPod(ctx, c.podGetter, params.PodName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	status := &Status{
		PodName: pod.Name,
	}
	addContainerStatus := func(cs ContainerStatus, k8sCS core.ContainerStatus) {
		cs.Name = k8sCS.Name
		cs.Waiting = k8sCS.State.Waiting != nil
		cs.Running = k8sCS.State.Running != nil
		if cs.Running {
			cs.StartedAt = k8sCS.State.Running.StartedAt.Time
		}
		cs.Terminated = k8sCS.State.Terminated != nil
		if cs.Terminated {
			cs.StartedAt = k8sCS.State.Terminated.StartedAt.Time
		}
		status.ContainerStatus = append(status.ContainerStatus, cs)
	}
	for _, cs := range pod.Status.InitContainerStatuses {
		addContainerStatus(ContainerStatus{InitContainer: true}, cs)
	}
	for _, cs := range pod.Status.ContainerStatuses {
		addContainerStatus(ContainerStatus{}, cs)
	}
	for _, cs := range pod.Status.EphemeralContainerStatuses {
		addContainerStatus(ContainerStatus{EphemeralContainer: true}, cs)
	}
	return status, nil
}
