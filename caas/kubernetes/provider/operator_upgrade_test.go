// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/cloudconfig/podcfg"
)

type DummyUpgradeCAASOperator struct {
	client     *fake.Clientset
	OperatorFn func(string) (*caas.Operator, error)
}

type OperatorUpgraderSuite struct {
	broker *DummyUpgradeCAASOperator
}

var _ = gc.Suite(&OperatorUpgraderSuite{})

func (d *DummyUpgradeCAASOperator) Client() kubernetes.Interface {
	return d.client
}

func (d *DummyUpgradeCAASOperator) Clock() clock.Clock {
	return testclock.NewClock(time.Time{})
}

func (d *DummyUpgradeCAASOperator) DeploymentName(n string, _ bool) string {
	return n
}

func (d *DummyUpgradeCAASOperator) IsLegacyLabels() bool {
	return false
}

func (d *DummyUpgradeCAASOperator) Namespace() string {
	return "test"
}

func (d *DummyUpgradeCAASOperator) Operator(n string) (*caas.Operator, error) {
	if d.OperatorFn == nil {
		return nil, errors.NotImplementedf("Operator()")
	}
	return d.OperatorFn(n)
}

func (d *DummyUpgradeCAASOperator) OperatorName(n string) string {
	return n
}

func (o *OperatorUpgraderSuite) SetUpTest(c *gc.C) {
	o.broker = &DummyUpgradeCAASOperator{
		client: fake.NewSimpleClientset(),
	}
}

func (o *OperatorUpgraderSuite) TestStatefulSetInitUpgrade(c *gc.C) {
	var (
		appName      = "testinitss"
		oldImagePath = fmt.Sprintf("%s/%s:9.9.8", podcfg.JujudOCINamespace, podcfg.JujudOCIName)
		newImagePath = fmt.Sprintf("%s/%s:9.9.9", podcfg.JujudOCINamespace, podcfg.JujudOCIName)
	)

	_, err := o.broker.Client().AppsV1().StatefulSets(o.broker.Namespace()).Create(context.TODO(),
		&apps.StatefulSet{
			ObjectMeta: meta.ObjectMeta{
				Name: o.broker.DeploymentName(appName, true),
			},
			Spec: apps.StatefulSetSpec{
				Selector: &meta.LabelSelector{
					MatchLabels: map[string]string{
						"match-label": "true",
					},
				},
				Template: core.PodTemplateSpec{
					Spec: core.PodSpec{
						InitContainers: []core.Container{
							{
								Name:  caas.InitContainerName,
								Image: oldImagePath,
							},
						},
					},
				},
			},
		}, meta.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	podChecker, err := operatorInitUpgrade(appName, newImagePath, o.broker)
	c.Assert(err, jc.ErrorIsNil)

	ss, err := o.broker.Client().AppsV1().StatefulSets(o.broker.Namespace()).
		Get(context.TODO(), o.broker.DeploymentName(appName, true), meta.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ss.Spec.Template.Spec.InitContainers[0].Image, gc.Equals, newImagePath)

	_, err = o.broker.Client().CoreV1().Pods(o.broker.Namespace()).Create(context.TODO(), &core.Pod{
		ObjectMeta: meta.ObjectMeta{
			Name: "pod1",
			Labels: map[string]string{
				"match-label": "true",
			},
		},
		Spec: core.PodSpec{
			InitContainers: []core.Container{
				{
					Name:  caas.InitContainerName,
					Image: newImagePath,
				},
			},
		},
		Status: core.PodStatus{
			Phase: core.PodPending,
		},
	}, meta.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	ready, err := podChecker()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ready, jc.IsFalse)

	_, err = o.broker.Client().CoreV1().Pods(o.broker.Namespace()).Update(context.TODO(), &core.Pod{
		ObjectMeta: meta.ObjectMeta{
			Name: "pod1",
			Labels: map[string]string{
				"match-label": "true",
			},
		},
		Spec: core.PodSpec{
			InitContainers: []core.Container{
				{
					Name:  caas.InitContainerName,
					Image: newImagePath,
				},
			},
		},
		Status: core.PodStatus{
			Phase: core.PodRunning,
		},
	}, meta.UpdateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	ready, err = podChecker()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ready, jc.IsTrue)
}

func (o *OperatorUpgraderSuite) TestStatefulSetInitUpgradePodNotReadyYet(c *gc.C) {
	var (
		appName      = "testinitss"
		oldImagePath = fmt.Sprintf("%s/%s:9.9.8", podcfg.JujudOCINamespace, podcfg.JujudOCIName)
		newImagePath = fmt.Sprintf("%s/%s:9.9.9", podcfg.JujudOCINamespace, podcfg.JujudOCIName)
	)

	_, err := o.broker.Client().AppsV1().StatefulSets(o.broker.Namespace()).Create(
		context.TODO(),
		&apps.StatefulSet{
			ObjectMeta: meta.ObjectMeta{
				Name: o.broker.DeploymentName(appName, true),
			},
			Spec: apps.StatefulSetSpec{
				Selector: &meta.LabelSelector{
					MatchLabels: map[string]string{
						"match-label": "true",
					},
				},
				Template: core.PodTemplateSpec{
					Spec: core.PodSpec{
						InitContainers: []core.Container{
							{
								Name:  caas.InitContainerName,
								Image: oldImagePath,
							},
						},
					},
				},
			},
		}, meta.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	podChecker, err := operatorInitUpgrade(appName, newImagePath, o.broker)
	c.Assert(err, jc.ErrorIsNil)

	ss, err := o.broker.Client().AppsV1().StatefulSets(o.broker.Namespace()).
		Get(context.TODO(), o.broker.DeploymentName(appName, true), meta.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ss.Spec.Template.Spec.InitContainers[0].Image, gc.Equals, newImagePath)

	ready, err := podChecker()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ready, jc.IsFalse)

	_, err = o.broker.Client().CoreV1().Pods(o.broker.Namespace()).Create(
		context.TODO(),
		&core.Pod{
			ObjectMeta: meta.ObjectMeta{
				Name: "pod1",
				Labels: map[string]string{
					"match-label": "true",
				},
			},
			Spec: core.PodSpec{
				InitContainers: []core.Container{
					{
						Name:  caas.InitContainerName,
						Image: newImagePath,
					},
				},
			},
			Status: core.PodStatus{
				Phase: core.PodRunning,
			},
		}, meta.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	ready, err = podChecker()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ready, jc.IsTrue)
}

func (o *OperatorUpgraderSuite) TestDaemonSetInitUpgrade(c *gc.C) {
	var (
		appName      = "testinitds"
		oldImagePath = fmt.Sprintf("%s/%s:9.9.8", podcfg.JujudOCINamespace, podcfg.JujudOCIName)
		newImagePath = fmt.Sprintf("%s/%s:9.9.9", podcfg.JujudOCINamespace, podcfg.JujudOCIName)
	)

	_, err := o.broker.Client().AppsV1().DaemonSets(o.broker.Namespace()).Create(context.TODO(),
		&apps.DaemonSet{
			ObjectMeta: meta.ObjectMeta{
				Name: o.broker.DeploymentName(appName, true),
			},
			Spec: apps.DaemonSetSpec{
				Selector: &meta.LabelSelector{
					MatchLabels: map[string]string{
						"match-label": "true",
					},
				},
				Template: core.PodTemplateSpec{
					Spec: core.PodSpec{
						InitContainers: []core.Container{
							{
								Name:  caas.InitContainerName,
								Image: oldImagePath,
							},
						},
					},
				},
			},
		}, meta.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	podChecker, err := operatorInitUpgrade(appName, newImagePath, o.broker)
	c.Assert(err, jc.ErrorIsNil)

	ds, err := o.broker.Client().AppsV1().DaemonSets(o.broker.Namespace()).
		Get(context.TODO(), o.broker.DeploymentName(appName, true), meta.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ds.Spec.Template.Spec.InitContainers[0].Image, gc.Equals, newImagePath)

	_, err = o.broker.Client().CoreV1().Pods(o.broker.Namespace()).Create(context.TODO(), &core.Pod{
		ObjectMeta: meta.ObjectMeta{
			Name: "pod1",
			Labels: map[string]string{
				"match-label": "true",
			},
		},
		Spec: core.PodSpec{
			InitContainers: []core.Container{
				{
					Name:  caas.InitContainerName,
					Image: newImagePath,
				},
			},
		},
		Status: core.PodStatus{
			Phase: core.PodPending,
		},
	}, meta.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	ready, err := podChecker()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ready, jc.IsFalse)

	_, err = o.broker.Client().CoreV1().Pods(o.broker.Namespace()).Update(context.TODO(), &core.Pod{
		ObjectMeta: meta.ObjectMeta{
			Name: "pod1",
			Labels: map[string]string{
				"match-label": "true",
			},
		},
		Spec: core.PodSpec{
			InitContainers: []core.Container{
				{
					Name:  caas.InitContainerName,
					Image: newImagePath,
				},
			},
		},
		Status: core.PodStatus{
			Phase: core.PodRunning,
		},
	}, meta.UpdateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	ready, err = podChecker()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ready, jc.IsTrue)
}

func (o *OperatorUpgraderSuite) TestDeploymentInitUpgrade(c *gc.C) {
	var (
		appName      = "testinitds"
		oldImagePath = fmt.Sprintf("%s/%s:9.9.8", podcfg.JujudOCINamespace, podcfg.JujudOCIName)
		newImagePath = fmt.Sprintf("%s/%s:9.9.9", podcfg.JujudOCINamespace, podcfg.JujudOCIName)
	)

	_, err := o.broker.Client().AppsV1().Deployments(o.broker.Namespace()).Create(context.TODO(),
		&apps.Deployment{
			ObjectMeta: meta.ObjectMeta{
				Name: o.broker.DeploymentName(appName, true),
			},
			Spec: apps.DeploymentSpec{
				Selector: &meta.LabelSelector{
					MatchLabels: map[string]string{
						"match-label": "true",
					},
				},
				Template: core.PodTemplateSpec{
					Spec: core.PodSpec{
						InitContainers: []core.Container{
							{
								Name:  caas.InitContainerName,
								Image: oldImagePath,
							},
						},
					},
				},
			},
		}, meta.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	podChecker, err := operatorInitUpgrade(appName, newImagePath, o.broker)
	c.Assert(err, jc.ErrorIsNil)

	de, err := o.broker.Client().AppsV1().Deployments(o.broker.Namespace()).
		Get(context.TODO(), o.broker.DeploymentName(appName, true), meta.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(de.Spec.Template.Spec.InitContainers[0].Image, gc.Equals, newImagePath)

	_, err = o.broker.Client().CoreV1().Pods(o.broker.Namespace()).Create(context.TODO(), &core.Pod{
		ObjectMeta: meta.ObjectMeta{
			Name: "pod1",
			Labels: map[string]string{
				"match-label": "true",
			},
		},
		Spec: core.PodSpec{
			InitContainers: []core.Container{
				{
					Name:  caas.InitContainerName,
					Image: newImagePath,
				},
			},
		},
		Status: core.PodStatus{
			Phase: core.PodPending,
		},
	}, meta.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	ready, err := podChecker()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ready, jc.IsFalse)

	_, err = o.broker.Client().CoreV1().Pods(o.broker.Namespace()).Update(context.TODO(), &core.Pod{
		ObjectMeta: meta.ObjectMeta{
			Name: "pod1",
			Labels: map[string]string{
				"match-label": "true",
			},
		},
		Spec: core.PodSpec{
			InitContainers: []core.Container{
				{
					Name:  caas.InitContainerName,
					Image: newImagePath,
				},
			},
		},
		Status: core.PodStatus{
			Phase: core.PodRunning,
		},
	}, meta.UpdateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	ready, err = podChecker()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ready, jc.IsTrue)
}
