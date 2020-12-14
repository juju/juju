// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// This test file is using go test framework as an internal test to go check.
// Added by tlm 16/12/2020

package provider

import (
	"testing"
	"time"

	"github.com/juju/juju/core/status"

	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestTerminatedPodJujuStatus(t *testing.T) {
	pod := core.Pod{
		ObjectMeta: meta.ObjectMeta{
			DeletionTimestamp: &meta.Time{time.Now()},
		},
		Status: core.PodStatus{
			Message: "test",
		},
	}

	testTime := time.Now()
	message, poStatus, now, err := podToJujuStatus(
		pod,
		testTime,
		func() ([]core.Event, error) {
			return []core.Event{}, nil
		})

	if err != nil {
		t.Fatalf("unexpected error getting pod status: %v", err)
	}

	if message != pod.Status.Message {
		t.Errorf("pod status messages not equal %q != %q", message, pod.Status.Message)
	}

	if poStatus != status.Terminated {
		t.Errorf("expected status %q got %q", status.Terminated, poStatus)
	}

	if !testTime.Equal(now) {
		t.Errorf("unexpected status time received, got %q wanted %q", now, testTime)
	}
}

func TestPodConditionListJujuStatus(t *testing.T) {
	tests := []struct {
		Name    string
		Pod     core.Pod
		Status  status.Status
		Message string
	}{
		{
			// We are testing the juju status here when a pod is considered
			// unschedulable. We want to see a juju status of blocked and
			// the scheduling message echoed as this will provide the best
			// reason.
			Name: "pod scheduling status unschedulable",
			Pod: core.Pod{
				Status: core.PodStatus{
					Conditions: []core.PodCondition{
						{
							Type:    core.PodScheduled,
							Status:  core.ConditionFalse,
							Reason:  core.PodReasonUnschedulable,
							Message: "not enough resources",
						},
					},
				},
			},
			Status:  status.Blocked,
			Message: "not enough resources",
		},
		{
			// We are testing the juju status here when pod scheduling is still
			// occurring. Kubernetes is still organising where to put our pod
			// so we expect a juju status of allocating and the pod scheduling
			// message to be echoed.
			Name: "pod scheduling status waiting",
			Pod: core.Pod{
				Status: core.PodStatus{
					Conditions: []core.PodCondition{
						{
							Type:    core.PodScheduled,
							Status:  core.ConditionFalse,
							Reason:  "waiting",
							Message: "waiting to be scheduled",
						},
					},
				},
			},
			Status:  status.Allocating,
			Message: "waiting to be scheduled",
		},
		{
			// We expect that every pod has a pod scheduling condition. If it's
			// missing our juju status should be Unknown as it's not safe for
			// us to test anymore of the pod conditions.
			Name: "pod scheduling status missing",
			Pod: core.Pod{
				Status: core.PodStatus{
					Conditions: []core.PodCondition{},
				},
			},
			Status:  status.Unknown,
			Message: "",
		},
		{
			// We are testing here that when the pod is in it's init stage
			// the correct juju status of maintenance is being reported.
			Name: "pod init status waiting",
			Pod: core.Pod{
				Status: core.PodStatus{
					Conditions: []core.PodCondition{
						{
							Type:   core.PodScheduled,
							Status: core.ConditionTrue,
						},
						{
							Type:    core.PodInitialized,
							Status:  core.ConditionFalse,
							Reason:  PodReasonContainersNotInitialized,
							Message: "initializing containers",
						},
					},
				},
			},
			Status:  status.Maintenance,
			Message: "initializing containers",
		},
		{
			// We are testing here that when the pod is in it's init stage
			// the correct juju status of error is displayed as one of the
			// pods is in a crash loop backoff.
			Name: "pod init status error",
			Pod: core.Pod{
				Status: core.PodStatus{
					Conditions: []core.PodCondition{
						{
							Type:   core.PodScheduled,
							Status: core.ConditionTrue,
						},
						{
							Type:    core.PodInitialized,
							Status:  core.ConditionFalse,
							Reason:  PodReasonContainersNotInitialized,
							Message: "initializing containers",
						},
					},
					InitContainerStatuses: []core.ContainerStatus{
						{
							Name: "test-init-container",
							State: core.ContainerState{
								Waiting: &core.ContainerStateWaiting{
									Reason:  PodReasonCrashLoopBackoff,
									Message: "I am broken",
								},
							},
						},
					},
				},
			},
			Status:  status.Error,
			Message: "crash loop backoff: I am broken",
		},
		{
			// We are testing here that while the main containers of the pod
			// are still being spun up and the juju status of maintenance is
			// still being reported
			Name: "pod container status waiting",
			Pod: core.Pod{
				Status: core.PodStatus{
					Conditions: []core.PodCondition{
						{
							Type:   core.PodScheduled,
							Status: core.ConditionTrue,
						},
						{
							Type:    core.ContainersReady,
							Status:  core.ConditionFalse,
							Reason:  PodReasonContainersNotReady,
							Message: "starting containers",
						},
					},
				},
			},
			Status:  status.Maintenance,
			Message: "starting containers",
		},
		{
			// We want to test here that when a container in the pod goes into
			// an error state like a crash loop backoff the subsequent juju
			// status is error
			Name: "pod container status error",
			Pod: core.Pod{
				Status: core.PodStatus{
					Conditions: []core.PodCondition{
						{
							Type:   core.PodScheduled,
							Status: core.ConditionTrue,
						},
						{
							Type:    core.ContainersReady,
							Status:  core.ConditionFalse,
							Reason:  PodReasonContainersNotReady,
							Message: "starting containers",
						},
					},
					ContainerStatuses: []core.ContainerStatus{
						{
							Name: "test-container",
							State: core.ContainerState{
								Waiting: &core.ContainerStateWaiting{
									Reason:  PodReasonCrashLoopBackoff,
									Message: "I am broken",
								},
							},
						},
					},
				},
			},
			Status:  status.Error,
			Message: "crash loop backoff: I am broken",
		},
		{
			// We want to test that when the container reason is unknown we
			// report Juju status of error and propagate the message
			Name: "pod container unknown reason",
			Pod: core.Pod{
				Status: core.PodStatus{
					Conditions: []core.PodCondition{
						{
							Type:   core.PodScheduled,
							Status: core.ConditionTrue,
						},
						{
							Type:    core.ContainersReady,
							Status:  core.ConditionFalse,
							Reason:  PodReasonContainersNotReady,
							Message: "starting containers",
						},
					},
					ContainerStatuses: []core.ContainerStatus{
						{
							Name: "test-container",
							State: core.ContainerState{
								Waiting: &core.ContainerStateWaiting{
									Reason:  "bad-reason",
									Message: "I am broken",
								},
							},
						},
					},
				},
			},
			Status:  status.Error,
			Message: "unknown container reason \"bad-reason\": I am broken",
		},
		{
			// We want to test here that when the pod ready condition the juju
			// status is running
			Name: "pod container status running",
			Pod: core.Pod{
				Status: core.PodStatus{
					Conditions: []core.PodCondition{
						{
							Type:   core.PodScheduled,
							Status: core.ConditionTrue,
						},
						{
							Type:   core.ContainersReady,
							Status: core.ConditionTrue,
						},
						{
							Type:   core.PodReady,
							Status: core.ConditionTrue,
						},
					},
				},
			},
			Status:  status.Running,
			Message: "",
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			testTime := time.Now()
			eventGetter := func() ([]core.Event, error) {
				return []core.Event{}, nil
			}
			message, poStatus, _, err := podToJujuStatus(
				test.Pod, testTime, eventGetter)

			if err != nil {
				t.Fatalf("unexpected error getting pod status: %v", err)
			}

			if message != test.Message {
				t.Errorf("pod status messages not equal %q != %q", message, test.Message)
			}

			if poStatus != test.Status {
				t.Errorf("expected status %q got %q", test.Status, poStatus)
			}
		})
	}
}
