// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils_test

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/juju/juju/caas/kubernetes/provider/utils"
)

type LabelSuite struct {
	client *fake.Clientset
}

var _ = gc.Suite(&LabelSuite{})

func (l *LabelSuite) SetUpTest(c *gc.C) {
	l.client = fake.NewSimpleClientset()
}

func (l *LabelSuite) TestHasLabels(c *gc.C) {
	tests := []struct {
		Src    labels.Set
		Has    labels.Set
		Result bool
	}{
		{
			Src: labels.Set{
				"foo":  "bar",
				"test": "test",
			},
			Has: labels.Set{
				"foo": "bar",
			},
			Result: true,
		},
		{
			Src: labels.Set{
				"foo":  "bar",
				"test": "test",
			},
			Has: labels.Set{
				"doesnot": "exist",
			},
			Result: false,
		},
	}

	for _, test := range tests {
		res := utils.HasLabels(test.Src, test.Has)
		c.Assert(res, gc.Equals, test.Result)
	}
}

func (l *LabelSuite) TestIsLegacyModelLabels(c *gc.C) {
	tests := []struct {
		IsLegacy  bool
		Model     string
		Namespace *core.Namespace
	}{
		{
			IsLegacy: false,
			Model:    "legacy-model-label-test-1",
			Namespace: &core.Namespace{
				ObjectMeta: meta.ObjectMeta{
					Name:   "legacy-model-label-test-1",
					Labels: utils.LabelsForModel("legacy-model-label-test-1", false),
				},
			},
		},
	}

	for _, test := range tests {
		_, err := l.client.CoreV1().Namespaces().Create(context.TODO(), test.Namespace, meta.CreateOptions{})
		c.Assert(err, jc.ErrorIsNil)

		legacy, err := utils.IsLegacyModelLabels(test.Model, l.client.CoreV1().Namespaces())
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(legacy, gc.Equals, test.IsLegacy)
	}
}

func (l *LabelSuite) TestLabelSetToSelector(c *gc.C) {
	tests := []struct {
		Labels   labels.Set
		Selector string
	}{
		{
			Labels: labels.Set{
				"foo": "bar",
			},
			Selector: "foo=bar",
		},
		{
			Labels: labels.Set{
				"foo":  "bar",
				"test": "mctest",
			},
			Selector: "foo=bar,test=mctest",
		},
	}

	for _, test := range tests {
		rval := utils.LabelSetToSelector(test.Labels)
		c.Assert(test.Selector, gc.Equals, rval.String())
	}
}

func (l *LabelSuite) TestLabelsForApp(c *gc.C) {
	tests := []struct {
		AppName        string
		ExpectedLabels labels.Set
		Legacy         bool
	}{
		{
			AppName: "tlm-boom",
			ExpectedLabels: labels.Set{
				"app.kubernetes.io/name": "tlm-boom",
			},
			Legacy: false,
		},
		{
			AppName: "tlm-boom",
			ExpectedLabels: labels.Set{
				"juju-app": "tlm-boom",
			},
			Legacy: true,
		},
	}

	for _, test := range tests {
		rval := utils.LabelsForApp(test.AppName, test.Legacy)
		c.Assert(rval, jc.DeepEquals, test.ExpectedLabels)
	}
}

func (l *LabelSuite) TestLabelForKeyValue(c *gc.C) {
	tests := []struct {
		Key            string
		Value          string
		ExpectedLabels labels.Set
	}{
		{
			Key:   "foo",
			Value: "bar",
			ExpectedLabels: labels.Set{
				"foo": "bar",
			},
		},
	}

	for _, test := range tests {
		rval := utils.LabelForKeyValue(test.Key, test.Value)
		c.Assert(rval, jc.DeepEquals, test.ExpectedLabels)
	}
}

func (l *LabelSuite) TestLabelMerge(c *gc.C) {
}
