// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"errors"
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider/application"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/testing"
)

type applyConstraintsSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&applyConstraintsSuite{})

func (s *applyConstraintsSuite) TestMemory(c *gc.C) {
	podSpec := &corev1.PodSpec{}
	configureConstraint := func(pod *corev1.PodSpec, resourceName corev1.ResourceName, value string) (err error) {
		c.Assert(pod, gc.Equals, podSpec)
		c.Assert(resourceName, gc.Equals, corev1.ResourceName("memory"))
		c.Assert(value, gc.Equals, "4096Mi")
		return errors.New("boom")
	}
	err := application.ApplyWorkloadConstraints(podSpec, "foo", constraints.MustParse("mem=4G"), configureConstraint)
	c.Assert(err, gc.ErrorMatches, "configuring workload container memory constraint for foo: boom")

	charmConfigureConstraint := func(pod *corev1.PodSpec, resourceName corev1.ResourceName, memReq, memLimit string) (err error) {
		c.Assert(pod, gc.Equals, podSpec)
		c.Assert(resourceName, gc.Equals, corev1.ResourceName("memory"))
		c.Assert(memReq, gc.Equals, fmt.Sprintf("%dMi", (caas.CharmMemRequestMiB)))
		c.Assert(memLimit, gc.Equals, fmt.Sprintf("%dMi", (caas.CharmMemLimitMiB)))
		return errors.New("boom")
	}
	charmConstraintVal := caas.CharmValue{
		MemRequest: caas.CharmMemRequestMiB,
		MemLimit:   caas.CharmMemLimitMiB,
	}
	err = application.ApplyCharmConstraints(podSpec, "foo", charmConstraintVal, charmConfigureConstraint)
	c.Assert(err, gc.ErrorMatches, "configuring charm container memory constraint for foo: boom")
}

func (s *applyConstraintsSuite) TestCPU(c *gc.C) {
	pod := &corev1.PodSpec{}
	configureConstraint := func(got *corev1.PodSpec, resourceName corev1.ResourceName, value string) (err error) {
		c.Assert(got, gc.Equals, pod)
		c.Assert(resourceName, gc.Equals, corev1.ResourceName("cpu"))
		c.Assert(value, gc.Equals, "2m")
		return errors.New("boom")
	}
	err := application.ApplyWorkloadConstraints(pod, "foo", constraints.MustParse("cpu-power=2"), configureConstraint)
	c.Assert(err, gc.ErrorMatches, "configuring workload container cpu constraint for foo: boom")
}

func (s *applyConstraintsSuite) TestArch(c *gc.C) {
	configureConstraint := func(got *corev1.PodSpec, resourceName corev1.ResourceName, value string) (err error) {
		return errors.New("unexpected")
	}
	pod := &corev1.PodSpec{}
	err := application.ApplyWorkloadConstraints(pod, "foo", constraints.MustParse("arch=arm64"), configureConstraint)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pod.NodeSelector, jc.DeepEquals, map[string]string{"kubernetes.io/arch": "arm64"})
}

func (s *applyConstraintsSuite) TestPodAffinityJustTopologyKey(c *gc.C) {
	configureConstraint := func(pod *corev1.PodSpec, resourceName corev1.ResourceName, value string) (err error) {
		return errors.New("unexpected")
	}
	pod := &corev1.PodSpec{}
	err := application.ApplyWorkloadConstraints(pod, "foo", constraints.MustParse("tags=pod.topology-key=foo"), configureConstraint)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pod.Affinity.PodAffinity, jc.DeepEquals, &corev1.PodAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{{
			LabelSelector: &metav1.LabelSelector{},
			TopologyKey:   "foo",
		}},
	})
	c.Assert(pod.Affinity.PodAntiAffinity, gc.IsNil)
	c.Assert(pod.Affinity.NodeAffinity, gc.IsNil)
}

func (s *applyConstraintsSuite) TestAffinityPod(c *gc.C) {
	configureConstraint := func(pod *corev1.PodSpec, resourceName corev1.ResourceName, value string) (err error) {
		return errors.New("unexpected")
	}
	pod := &corev1.PodSpec{}
	err := application.ApplyWorkloadConstraints(pod, "foo", constraints.MustParse("tags=pod.hello=world|universe,pod.^goodbye=world"), configureConstraint)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pod.Affinity.PodAffinity, jc.DeepEquals, &corev1.PodAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{{
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: nil,
				MatchExpressions: []metav1.LabelSelectorRequirement{{
					Key:      "goodbye",
					Operator: metav1.LabelSelectorOpNotIn,
					Values:   []string{"world"},
				}, {
					Key:      "hello",
					Operator: metav1.LabelSelectorOpIn,
					Values:   []string{"world", "universe"},
				}},
			},
		}},
	})
	c.Assert(pod.Affinity.PodAntiAffinity, gc.IsNil)
	c.Assert(pod.Affinity.NodeAffinity, gc.IsNil)
}

func (s *applyConstraintsSuite) TestPodAffinityAll(c *gc.C) {
	configureConstraint := func(pod *corev1.PodSpec, resourceName corev1.ResourceName, value string) (err error) {
		return errors.New("unexpected")
	}
	pod := &corev1.PodSpec{}
	err := application.ApplyWorkloadConstraints(pod, "foo", constraints.MustParse("tags=pod.hello=world,pod.^goodbye=world,pod.topology-key=foo"), configureConstraint)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pod.Affinity.PodAffinity, jc.DeepEquals, &corev1.PodAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{{
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: nil,
				MatchExpressions: []metav1.LabelSelectorRequirement{{
					Key:      "goodbye",
					Operator: metav1.LabelSelectorOpNotIn,
					Values:   []string{"world"},
				}, {
					Key:      "hello",
					Operator: metav1.LabelSelectorOpIn,
					Values:   []string{"world"},
				}},
			},
			TopologyKey: "foo",
		}},
	})
}

func (s *applyConstraintsSuite) TestAntiPodAffinityJustTopologyKey(c *gc.C) {
	configureConstraint := func(pod *corev1.PodSpec, resourceName corev1.ResourceName, value string) (err error) {
		return errors.New("unexpected")
	}
	pod := &corev1.PodSpec{}
	err := application.ApplyWorkloadConstraints(pod, "foo", constraints.MustParse("tags=anti-pod.topology-key=foo"), configureConstraint)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pod.Affinity.PodAntiAffinity, jc.DeepEquals, &corev1.PodAntiAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{{
			LabelSelector: &metav1.LabelSelector{},
			TopologyKey:   "foo",
		}},
	})
	c.Assert(pod.Affinity.PodAffinity, gc.IsNil)
	c.Assert(pod.Affinity.NodeAffinity, gc.IsNil)
}

func (s *applyConstraintsSuite) TestAntiPodAffinity(c *gc.C) {
	configureConstraint := func(pod *corev1.PodSpec, resourceName corev1.ResourceName, value string) (err error) {
		return errors.New("unexpected")
	}
	pod := &corev1.PodSpec{}
	err := application.ApplyWorkloadConstraints(pod, "foo", constraints.MustParse("tags=anti-pod.hello=world|universe,anti-pod.^goodbye=world"), configureConstraint)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pod.Affinity.PodAntiAffinity, jc.DeepEquals, &corev1.PodAntiAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{{
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: nil,
				MatchExpressions: []metav1.LabelSelectorRequirement{{
					Key:      "goodbye",
					Operator: metav1.LabelSelectorOpNotIn,
					Values:   []string{"world"},
				}, {
					Key:      "hello",
					Operator: metav1.LabelSelectorOpIn,
					Values:   []string{"world", "universe"},
				}},
			},
		}},
	})
	c.Assert(pod.Affinity.PodAffinity, gc.IsNil)
	c.Assert(pod.Affinity.NodeAffinity, gc.IsNil)
}

func (s *applyConstraintsSuite) TestAntiPodAffinityAll(c *gc.C) {
	configureConstraint := func(pod *corev1.PodSpec, resourceName corev1.ResourceName, value string) (err error) {
		return errors.New("unexpected")
	}
	pod := &corev1.PodSpec{}
	err := application.ApplyWorkloadConstraints(pod, "foo", constraints.MustParse("tags=anti-pod.hello=world,anti-pod.^goodbye=world,anti-pod.topology-key=foo"), configureConstraint)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pod.Affinity.PodAntiAffinity, jc.DeepEquals, &corev1.PodAntiAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{{
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: nil,
				MatchExpressions: []metav1.LabelSelectorRequirement{{
					Key:      "goodbye",
					Operator: metav1.LabelSelectorOpNotIn,
					Values:   []string{"world"},
				}, {
					Key:      "hello",
					Operator: metav1.LabelSelectorOpIn,
					Values:   []string{"world"},
				}},
			},
			TopologyKey: "foo",
		}},
	})
	c.Assert(pod.Affinity.PodAffinity, gc.IsNil)
	c.Assert(pod.Affinity.NodeAffinity, gc.IsNil)
}

func (s *applyConstraintsSuite) TestNodeAntiAffinity(c *gc.C) {
	configureConstraint := func(pod *corev1.PodSpec, resourceName corev1.ResourceName, value string) (err error) {
		return errors.New("unexpected")
	}
	pod := &corev1.PodSpec{}
	err := application.ApplyWorkloadConstraints(pod, "foo", constraints.MustParse("tags=node.hello=world|universe,node.^goodbye=world"), configureConstraint)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pod.Affinity.NodeAffinity, jc.DeepEquals, &corev1.NodeAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{{
				MatchExpressions: []corev1.NodeSelectorRequirement{{
					Key:      "goodbye",
					Operator: corev1.NodeSelectorOpNotIn,
					Values:   []string{"world"},
				}, {
					Key:      "hello",
					Operator: corev1.NodeSelectorOpIn,
					Values:   []string{"world", "universe"},
				}},
			}},
		},
	})
	c.Assert(pod.Affinity.PodAffinity, gc.IsNil)
	c.Assert(pod.Affinity.PodAntiAffinity, gc.IsNil)
}

func (s *applyConstraintsSuite) TestRoundNumDownToPowerOfTwo(c *gc.C) {

	tests := []struct {
		name     string
		input    uint64
		expected uint64
	}{
		// Basic powers of two
		{"Zero", 0, 0},
		{"One", 1, 1},
		{"Two", 2, 2},
		{"Four", 4, 4},
		{"Eight", 8, 8},
		{"Sixteen", 16, 16},
		{"ThirtyTwo", 32, 32},
		{"SixtyFour", 64, 64},
		{"MaxPowerOfTwo", 1 << 63, 1 << 63},

		// Edge cases just above or below powers of two
		{"JustBelow4", 3, 2},
		{"JustAbove4", 5, 4},
		{"JustBelow256", 255, 128},
		{"JustAbove256", 257, 256},
		{"JustBelow1024", 1023, 512},
		{"JustAbove1024", 1025, 1024},

		// Large values
		{"MaxUint64", ^uint64(0), 1 << 63},
		{"HighValueBit62", 1<<62 + 123456, 1 << 62},

		// Random mid range values
		{"Random45", 45, 32},
		{"Random123", 123, 64},
		{"Random999", 999, 512},
		{"Random2049", 2049, 1024},
		{"Random60000", 60000, 32768},
	}

	for _, tt := range tests {
		c.Assert(application.RoundNumDownToPowerOfTwo(tt.input), gc.Equals, tt.expected)
	}
}
