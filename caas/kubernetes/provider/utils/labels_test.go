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

	"github.com/juju/juju/caas/kubernetes/provider/constants"
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
					Labels: map[string]string{"model.juju.is/name": "legacy-model-label-test-1"},
				},
			},
		},
		{
			IsLegacy: false,
			Model:    "legacy-model-label-test-2",
			Namespace: &core.Namespace{
				ObjectMeta: meta.ObjectMeta{
					Name:   "legacy-model-label-test-2",
					Labels: map[string]string{},
				},
			},
		},
		{
			IsLegacy: true,
			Model:    "legacy-model-label-test-3",
			Namespace: &core.Namespace{
				ObjectMeta: meta.ObjectMeta{
					Name:   "legacy-model-label-test-3",
					Labels: map[string]string{"juju-model": "legacy-model-label-test-3"},
				},
			},
		},
	}

	for _, test := range tests {
		_, err := l.client.CoreV1().Namespaces().Create(context.Background(), test.Namespace, meta.CreateOptions{})
		c.Assert(err, jc.ErrorIsNil)

		legacy, err := utils.IsLegacyModelLabels(test.Namespace.Name, test.Model, l.client.CoreV1().Namespaces())
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(legacy, gc.Equals, test.IsLegacy)
	}
}

func (l *LabelSuite) TestLabelsToSelector(c *gc.C) {
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
		rval := utils.LabelsToSelector(test.Labels)
		c.Assert(test.Selector, gc.Equals, rval.String())
	}
}

func (l *LabelSuite) TestSelectorLabelsForApp(c *gc.C) {
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
		rval := utils.SelectorLabelsForApp(test.AppName, test.Legacy)
		c.Assert(rval, jc.DeepEquals, test.ExpectedLabels)
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
				"app.kubernetes.io/name":       "tlm-boom",
				"app.kubernetes.io/managed-by": "juju",
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

func (l *LabelSuite) TestLabelsForStorage(c *gc.C) {
	tests := []struct {
		AppName        string
		ExpectedLabels labels.Set
		Legacy         bool
	}{
		{
			AppName: "tlm-boom",
			ExpectedLabels: labels.Set{
				"storage.juju.is/name": "tlm-boom",
			},
			Legacy: false,
		},
		{
			AppName: "tlm-boom",
			ExpectedLabels: labels.Set{
				"juju-storage": "tlm-boom",
			},
			Legacy: true,
		},
	}

	for _, test := range tests {
		rval := utils.LabelsForStorage(test.AppName, test.Legacy)
		c.Assert(rval, jc.DeepEquals, test.ExpectedLabels)
	}
}

func (l *LabelSuite) TestLabelsForModel(c *gc.C) {
	tests := []struct {
		AppName        string
		ExpectedLabels labels.Set
		Legacy         bool
	}{
		{
			AppName: "tlm-boom",
			ExpectedLabels: labels.Set{
				"model.juju.is/name": "tlm-boom",
			},
			Legacy: false,
		},
		{
			AppName: "tlm-boom",
			ExpectedLabels: labels.Set{
				"juju-model": "tlm-boom",
			},
			Legacy: true,
		},
	}

	for _, test := range tests {
		rval := utils.LabelsForModel(test.AppName, test.Legacy)
		c.Assert(rval, jc.DeepEquals, test.ExpectedLabels)
	}
}

func (l *LabelSuite) TestLabelsForOperator(c *gc.C) {
	tests := []struct {
		AppName        string
		Target         string
		ExpectedLabels labels.Set
		Legacy         bool
	}{
		{
			AppName: "tlm-boom",
			Target:  "harry",
			ExpectedLabels: labels.Set{
				"operator.juju.is/name":   "tlm-boom",
				"operator.juju.is/target": "harry",
			},
			Legacy: false,
		},
		{
			AppName: "tlm-boom",
			ExpectedLabels: labels.Set{
				"juju-operator": "tlm-boom",
			},
			Legacy: true,
		},
	}

	for _, test := range tests {
		rval := utils.LabelsForOperator(test.AppName, test.Target, test.Legacy)
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

func (l *LabelSuite) TestLabelsMerge(c *gc.C) {
	one := labels.Set{"foo": "bar"}
	two := labels.Set{"foo": "baz", "up": "down"}
	result := utils.LabelsMerge(one, two)
	c.Assert(result, jc.DeepEquals, labels.Set{
		"foo": "baz",
		"up":  "down",
	})
}

func (l *LabelSuite) TestStorageNameFromLabels(c *gc.C) {
	tests := []struct {
		Labels   labels.Set
		Expected string
	}{
		{
			Labels:   labels.Set{constants.LabelJujuStorageName: "test1"},
			Expected: "test1",
		},
		{
			Labels:   labels.Set{constants.LegacyLabelStorageName: "test2"},
			Expected: "test2",
		},
		{
			Labels:   labels.Set{"foo": "bar"},
			Expected: "",
		},
	}

	for _, test := range tests {
		c.Assert(utils.StorageNameFromLabels(test.Labels), gc.Equals, test.Expected)
	}
}
