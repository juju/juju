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

func (l *LabelSuite) TestDectectModelLabelVersion(c *gc.C) {
	tests := []struct {
		LabelVersion   constants.LabelVersion
		ModelName      string
		ModelUUID      string
		ControllerUUID string
		Namespace      *core.Namespace
		ErrorString    string
	}{
		{
			LabelVersion:   constants.LegacyLabelVersion,
			ModelName:      "model-label-test-3",
			ModelUUID:      "badf00d3",
			ControllerUUID: "d0gf00d3",
			Namespace: &core.Namespace{
				ObjectMeta: meta.ObjectMeta{
					Name:   "model-label-test-3",
					Labels: map[string]string{"juju-model": "model-label-test-3"},
				},
			},
		},
		{
			LabelVersion:   constants.LabelVersion1,
			ModelName:      "model-label-test-1",
			ModelUUID:      "badf00d1",
			ControllerUUID: "d0gf00d1",
			Namespace: &core.Namespace{
				ObjectMeta: meta.ObjectMeta{
					Name:   "model-label-test-1",
					Labels: map[string]string{"model.juju.is/name": "model-label-test-1"},
				},
			},
		},
		{
			LabelVersion:   constants.LabelVersion2,
			ModelName:      "model-label-test-2",
			ModelUUID:      "badf00d2",
			ControllerUUID: "d0gf00d2",
			Namespace: &core.Namespace{
				ObjectMeta: meta.ObjectMeta{
					Name:   "model-label-test-2",
					Labels: map[string]string{"model.juju.is/name": "model-label-test-2", "model.juju.is/id": "badf00d2"},
				},
			},
		},
		{
			LabelVersion:   constants.LabelVersion2,
			ModelName:      "controller",
			ModelUUID:      "badf00d4",
			ControllerUUID: "d0gf00d4",
			Namespace: &core.Namespace{
				ObjectMeta: meta.ObjectMeta{
					Name:   "controller-foo",
					Labels: map[string]string{"model.juju.is/name": "controller", "controller.juju.is/id": "d0gf00d4"},
				},
			},
		},
		{
			LabelVersion:   -1,
			ModelName:      "controller",
			ModelUUID:      "badf00d4",
			ControllerUUID: "d0gf00d4",
			Namespace: &core.Namespace{
				ObjectMeta: meta.ObjectMeta{
					Name:   "controller-bar",
					Labels: map[string]string{"foo.juju.is/bar": "nope", "controller.juju.is/id": "d0gf00d"},
				},
			},
			ErrorString: "unexpected model labels",
		},
	}

	for t, test := range tests {
		_, err := l.client.CoreV1().Namespaces().Create(context.TODO(), test.Namespace, meta.CreateOptions{})
		c.Assert(err, jc.ErrorIsNil)

		labelVersion, err := utils.DetectModelLabelVersion(test.Namespace.Name, test.ModelName, test.ModelUUID, test.ControllerUUID, l.client.CoreV1().Namespaces())
		if test.ErrorString != "" {
			c.Assert(err, gc.ErrorMatches, test.ErrorString, gc.Commentf("test %d", t))
		} else {
			c.Assert(err, jc.ErrorIsNil, gc.Commentf("test %d", t))
		}
		c.Check(labelVersion, gc.Equals, test.LabelVersion, gc.Commentf("test %d", t))
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
		LabelVersion   constants.LabelVersion
	}{
		{
			AppName: "tlm-boom",
			ExpectedLabels: labels.Set{
				"app.kubernetes.io/name": "tlm-boom",
			},
			LabelVersion: constants.LabelVersion1,
		},
		{
			AppName: "tlm-boom",
			ExpectedLabels: labels.Set{
				"juju-app": "tlm-boom",
			},
			LabelVersion: constants.LegacyLabelVersion,
		},
	}

	for _, test := range tests {
		rval := utils.SelectorLabelsForApp(test.AppName, test.LabelVersion)
		c.Assert(rval, jc.DeepEquals, test.ExpectedLabels)
	}
}

func (l *LabelSuite) TestLabelsForApp(c *gc.C) {
	tests := []struct {
		AppName        string
		ExpectedLabels labels.Set
		LabelVersion   constants.LabelVersion
	}{
		{
			AppName: "tlm-boom",
			ExpectedLabels: labels.Set{
				"app.kubernetes.io/name":       "tlm-boom",
				"app.kubernetes.io/managed-by": "juju",
			},
			LabelVersion: constants.LabelVersion1,
		},
		{
			AppName: "tlm-boom",
			ExpectedLabels: labels.Set{
				"juju-app": "tlm-boom",
			},
			LabelVersion: constants.LegacyLabelVersion,
		},
	}

	for _, test := range tests {
		rval := utils.LabelsForApp(test.AppName, test.LabelVersion)
		c.Assert(rval, jc.DeepEquals, test.ExpectedLabels)
	}
}

func (l *LabelSuite) TestLabelsForStorage(c *gc.C) {
	tests := []struct {
		AppName        string
		ExpectedLabels labels.Set
		LabelVersion   constants.LabelVersion
	}{
		{
			AppName: "tlm-boom",
			ExpectedLabels: labels.Set{
				"storage.juju.is/name": "tlm-boom",
			},
			LabelVersion: constants.LabelVersion1,
		},
		{
			AppName: "tlm-boom",
			ExpectedLabels: labels.Set{
				"juju-storage": "tlm-boom",
			},
			LabelVersion: constants.LegacyLabelVersion,
		},
	}

	for _, test := range tests {
		rval := utils.LabelsForStorage(test.AppName, test.LabelVersion)
		c.Assert(rval, jc.DeepEquals, test.ExpectedLabels)
	}
}

func (l *LabelSuite) TestLabelsForModel(c *gc.C) {
	tests := []struct {
		ModelName      string
		ModelUUID      string
		ControllerUUID string
		ExpectedLabels labels.Set
		LabelVersion   constants.LabelVersion
	}{
		{
			ModelName:      "tlm-boom",
			ModelUUID:      "d0gf00d",
			ControllerUUID: "badf00d",
			ExpectedLabels: labels.Set{
				"model.juju.is/name": "tlm-boom",
			},
			LabelVersion: constants.LabelVersion1,
		},
		{
			ModelName:      "tlm-boom",
			ModelUUID:      "d0gf00d",
			ControllerUUID: "badf00d",
			ExpectedLabels: labels.Set{
				"juju-model": "tlm-boom",
			},
			LabelVersion: constants.LegacyLabelVersion,
		},
	}

	for _, test := range tests {
		rval := utils.LabelsForModel(test.ModelName, test.ModelUUID, test.ControllerUUID, test.LabelVersion)
		c.Assert(rval, jc.DeepEquals, test.ExpectedLabels)
	}
}

func (l *LabelSuite) TestLabelsForOperator(c *gc.C) {
	tests := []struct {
		AppName        string
		Target         string
		ExpectedLabels labels.Set
		LabelVersion   constants.LabelVersion
	}{
		{
			AppName: "tlm-boom",
			Target:  "harry",
			ExpectedLabels: labels.Set{
				"operator.juju.is/name":   "tlm-boom",
				"operator.juju.is/target": "harry",
			},
			LabelVersion: constants.LabelVersion1,
		},
		{
			AppName: "tlm-boom",
			ExpectedLabels: labels.Set{
				"juju-operator": "tlm-boom",
			},
			LabelVersion: constants.LegacyLabelVersion,
		},
	}

	for _, test := range tests {
		rval := utils.LabelsForOperator(test.AppName, test.Target, test.LabelVersion)
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
