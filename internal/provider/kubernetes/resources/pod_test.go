// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/tc"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/provider/kubernetes/resources"
)

type podSuite struct {
	resourceSuite
}

func TestPodSuite(t *testing.T) {
	tc.Run(t, &podSuite{})
}

func (s *podSuite) TestApply(c *tc.C) {
	ds := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ds1",
			Namespace: "test",
		},
	}
	// Create.
	dsResource := resources.NewPod("ds1", "test", ds)
	c.Assert(dsResource.Apply(c.Context(), s.client), tc.ErrorIsNil)
	result, err := s.client.CoreV1().Pods("test").Get(c.Context(), "ds1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(result.GetAnnotations()), tc.Equals, 0)

	// Update.
	ds.SetAnnotations(map[string]string{"a": "b"})
	dsResource = resources.NewPod("ds1", "test", ds)
	c.Assert(dsResource.Apply(c.Context(), s.client), tc.ErrorIsNil)

	result, err = s.client.CoreV1().Pods("test").Get(c.Context(), "ds1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.GetName(), tc.Equals, `ds1`)
	c.Assert(result.GetNamespace(), tc.Equals, `test`)
	c.Assert(result.GetAnnotations(), tc.DeepEquals, map[string]string{"a": "b"})
}

func (s *podSuite) TestGet(c *tc.C) {
	template := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ds1",
			Namespace: "test",
		},
	}
	ds1 := template
	ds1.SetAnnotations(map[string]string{"a": "b"})
	_, err := s.client.CoreV1().Pods("test").Create(c.Context(), &ds1, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	dsResource := resources.NewPod("ds1", "test", &template)
	c.Assert(len(dsResource.GetAnnotations()), tc.Equals, 0)
	err = dsResource.Get(c.Context(), s.client)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(dsResource.GetName(), tc.Equals, `ds1`)
	c.Assert(dsResource.GetNamespace(), tc.Equals, `test`)
	c.Assert(dsResource.GetAnnotations(), tc.DeepEquals, map[string]string{"a": "b"})
}

func (s *podSuite) TestDelete(c *tc.C) {
	ds := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ds1",
			Namespace: "test",
		},
	}
	_, err := s.client.CoreV1().Pods("test").Create(c.Context(), &ds, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	result, err := s.client.CoreV1().Pods("test").Get(c.Context(), "ds1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.GetName(), tc.Equals, `ds1`)

	dsResource := resources.NewPod("ds1", "test", &ds)
	err = dsResource.Delete(c.Context(), s.client)
	c.Assert(err, tc.ErrorIsNil)

	err = dsResource.Get(c.Context(), s.client)
	c.Assert(err, tc.ErrorIs, errors.NotFound)

	_, err = s.client.CoreV1().Pods("test").Get(c.Context(), "ds1", metav1.GetOptions{})
	c.Assert(err, tc.Satisfies, k8serrors.IsNotFound)
}

func TestTerminatedPodJujuStatus(t *testing.T) {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			DeletionTimestamp: &metav1.Time{Time: time.Now()},
		},
		Status: corev1.PodStatus{
			Message: "test",
		},
	}

	testTime := time.Now()
	message, poStatus, now, err := resources.PodToJujuStatus(
		pod,
		testTime,
		func() ([]corev1.Event, error) {
			return []corev1.Event{}, nil
		},
	)

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
		Pod     corev1.Pod
		Status  status.Status
		Message string
	}{
		{
			// We are testing the juju status here when a pod is considered
			// unschedulable. We want to see a juju status of blocked and
			// the scheduling message echoed as this will provide the best
			// reason.
			Name: "pod scheduling status unschedulable",
			Pod: corev1.Pod{
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:    corev1.PodScheduled,
							Status:  corev1.ConditionFalse,
							Reason:  corev1.PodReasonUnschedulable,
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
			Pod: corev1.Pod{
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:    corev1.PodScheduled,
							Status:  corev1.ConditionFalse,
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
			Pod: corev1.Pod{
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{},
				},
			},
			Status:  status.Unknown,
			Message: "",
		},
		{
			// We are testing here that when the pod is in it's init stage
			// the correct juju status of maintenance is being reported.
			Name: "pod init status waiting",
			Pod: corev1.Pod{
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodScheduled,
							Status: corev1.ConditionTrue,
						},
						{
							Type:    corev1.PodInitialized,
							Status:  corev1.ConditionFalse,
							Reason:  resources.PodReasonContainersNotInitialized,
							Message: "initializing containers",
						},
					},
				},
			},
			Status:  status.Maintenance,
			Message: "initializing containers",
		},
		{
			// We are testing here that when the pod is running the init steps
			// the correct status of maintenance is being reported.
			Name: "pod init status running",
			Pod: corev1.Pod{
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodScheduled,
							Status: corev1.ConditionTrue,
						},
						{
							Type:    corev1.PodInitialized,
							Status:  corev1.ConditionFalse,
							Reason:  resources.PodReasonInitializing,
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
			Pod: corev1.Pod{
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodScheduled,
							Status: corev1.ConditionTrue,
						},
						{
							Type:    corev1.PodInitialized,
							Status:  corev1.ConditionFalse,
							Reason:  resources.PodReasonContainersNotInitialized,
							Message: "initializing containers",
						},
					},
					InitContainerStatuses: []corev1.ContainerStatus{
						{
							Name: "test-init-container",
							State: corev1.ContainerState{
								Waiting: &corev1.ContainerStateWaiting{
									Reason:  resources.PodReasonCrashLoopBackoff,
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
			Pod: corev1.Pod{
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodScheduled,
							Status: corev1.ConditionTrue,
						},
						{
							Type:    corev1.ContainersReady,
							Status:  corev1.ConditionFalse,
							Reason:  resources.PodReasonContainersNotReady,
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
			Pod: corev1.Pod{
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodScheduled,
							Status: corev1.ConditionTrue,
						},
						{
							Type:    corev1.ContainersReady,
							Status:  corev1.ConditionFalse,
							Reason:  resources.PodReasonContainersNotReady,
							Message: "starting containers",
						},
					},
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name: "test-container",
							State: corev1.ContainerState{
								Waiting: &corev1.ContainerStateWaiting{
									Reason:  resources.PodReasonCrashLoopBackoff,
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
			// We want to  test here the pod container creating message for init
			// containers. This addresses lp-1914088
			Name: "pod container status creating init",
			Pod: corev1.Pod{
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodScheduled,
							Status: corev1.ConditionTrue,
						},
						{
							Type:    corev1.PodInitialized,
							Status:  corev1.ConditionFalse,
							Reason:  resources.PodReasonContainersNotInitialized,
							Message: "initializing containers",
						},
					},
					InitContainerStatuses: []corev1.ContainerStatus{
						{
							Name: "test-container",
							State: corev1.ContainerState{
								Waiting: &corev1.ContainerStateWaiting{
									Reason: resources.PodReasonContainerCreating,
								},
							},
						},
					},
				},
			},
			Status:  status.Maintenance,
			Message: "initializing containers",
		},
		{
			// We want to  test here the pod container creating message on pod
			// containers. This addresses lp-1914088
			Name: "pod container status creating",
			Pod: corev1.Pod{
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodScheduled,
							Status: corev1.ConditionTrue,
						},
						{
							Type:    corev1.ContainersReady,
							Status:  corev1.ConditionFalse,
							Reason:  resources.PodReasonContainersNotReady,
							Message: "creating containers",
						},
					},
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name: "test-container",
							State: corev1.ContainerState{
								Waiting: &corev1.ContainerStateWaiting{
									Reason: resources.PodReasonContainerCreating,
								},
							},
						},
					},
				},
			},
			Status:  status.Maintenance,
			Message: "creating containers",
		},
		{
			// We want to test that when the container reason is unknown we
			// report Juju status of error and propagate the message
			Name: "pod container unknown reason",
			Pod: corev1.Pod{
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodScheduled,
							Status: corev1.ConditionTrue,
						},
						{
							Type:    corev1.ContainersReady,
							Status:  corev1.ConditionFalse,
							Reason:  resources.PodReasonContainersNotReady,
							Message: "starting containers",
						},
					},
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name: "test-container",
							State: corev1.ContainerState{
								Waiting: &corev1.ContainerStateWaiting{
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
			Pod: corev1.Pod{
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodScheduled,
							Status: corev1.ConditionTrue,
						},
						{
							Type:   corev1.ContainersReady,
							Status: corev1.ConditionTrue,
						},
						{
							Type:   corev1.PodReady,
							Status: corev1.ConditionTrue,
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
			eventGetter := func() ([]corev1.Event, error) {
				return []corev1.Event{}, nil
			}
			message, poStatus, _, err := resources.PodToJujuStatus(
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
