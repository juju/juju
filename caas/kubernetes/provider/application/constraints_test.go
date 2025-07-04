// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"errors"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/caas/kubernetes/provider/application"
	"github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/testing"
)

type applyConstraintsSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&applyConstraintsSuite{})

func (s *applyConstraintsSuite) TestMemory(c *gc.C) {
	pod := &corev1.PodSpec{}
	configureConstraint := func(got *corev1.PodSpec, resourceName corev1.ResourceName, value string) (err error) {
		c.Assert(got, gc.Equals, pod)
		c.Assert(resourceName, gc.Equals, corev1.ResourceName("memory"))
		c.Assert(value, gc.Equals, "4096Mi")
		return errors.New("boom")
	}
	err := application.ApplyWorkloadConstraints(pod, "foo", constraints.MustParse("mem=4G"), configureConstraint)
	c.Assert(err, gc.ErrorMatches, "configuring memory constraint for foo: boom")
}

func (s *applyConstraintsSuite) TestCharmMemory(c *gc.C) {
	testCases := []struct {
		desc     string
		memReq   string
		memLimit string
		err      string
	}{
		{
			desc:     "(invalid) 0Mi request",
			memReq:   "0Mi",
			memLimit: "1024Mi",
			err:      "charm container mem request value not valid",
		},
		{
			desc:     "(invalid) limit equals zero",
			memReq:   "128Mi",
			memLimit: "0Mi",
			err:      "charm container mem limit value not valid",
		},
		{
			desc:     "(invalid) negative limit",
			memReq:   "64Mi",
			memLimit: "-20Mi",
			err:      "charm container mem limit value not valid",
		},
		{
			desc:     "(invalid) negative request",
			memReq:   "-64Mi",
			memLimit: "1024Mi",
			err:      "charm container mem request value not valid",
		},
		{
			desc:     "(invalid) both limit and request negative",
			memReq:   "-64Mi",
			memLimit: "-20Mi",
			err:      "charm container mem limit value not valid",
		},
		{
			desc:     "(invalid) unsupported suffix",
			memReq:   "24Mi",
			memLimit: "1024Mb",
			err:      "charm container mem limit value not valid",
		},
		{
			desc:     "(invalid) empty request value",
			memReq:   "",
			memLimit: "512Mi",
			err:      "charm container mem request value not valid",
		},
		{
			desc:     "(invalid) empty limit value",
			memReq:   "128Mi",
			memLimit: "",
			err:      "charm container mem limit value not valid",
		},
		{
			desc:     "(invalid) non-numeric request",
			memReq:   "abcMi",
			memLimit: "512Mi",
			err:      "charm container mem request value not valid",
		},
		{
			desc:     "(invalid) limit with float",
			memReq:   "128Mi",
			memLimit: "128.5Mi",
			err:      "charm container mem limit value not valid",
		},
		{
			desc:     "(valid) small values",
			memReq:   "1Mi",
			memLimit: "2Mi",
		},
		{
			desc:     "(valid) large values",
			memReq:   "99999999Mi",
			memLimit: "22103103Mi",
		},
		{
			desc:     "(valid) request and limit",
			memReq:   "64Mi",
			memLimit: "1024Mi",
		},
	}

	for _, tc := range testCases {
		c.Logf("case: %s", tc.desc)

		pod := &corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: constants.ApplicationCharmContainer},
			},
		}

		err := application.ApplyCharmConstraints(pod, "foo",
			application.CharmContainerResourceRequirements{
				MemRequestMi: tc.memReq,
				MemLimitMi:   tc.memLimit,
			})

		if tc.err == "" {
			c.Assert(err, jc.ErrorIsNil)
		} else {
			c.Assert(err, gc.ErrorMatches, tc.err)
		}
	}
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
	c.Assert(err, gc.ErrorMatches, "configuring cpu constraint for foo: boom")
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
