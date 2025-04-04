// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/cloudconfig/podcfg"
)

type DummyUpgradeCAASOperator struct {
	client *fake.Clientset
	clock  clock.Clock
}

type OperatorUpgraderSuite struct {
	broker *DummyUpgradeCAASOperator
}

var _ = gc.Suite(&OperatorUpgraderSuite{})

func (d *DummyUpgradeCAASOperator) Client() kubernetes.Interface {
	return d.client
}

func (d *DummyUpgradeCAASOperator) Clock() clock.Clock {
	return d.clock
}

func (d *DummyUpgradeCAASOperator) DeploymentName(n string, _ bool) string {
	return n
}

func (d *DummyUpgradeCAASOperator) LabelVersion() constants.LabelVersion {
	return constants.LabelVersion1
}

func (d *DummyUpgradeCAASOperator) Namespace() string {
	return "test"
}

func (d *DummyUpgradeCAASOperator) Operator(appName string) (*caas.Operator, error) {
	return operator(d.client, d.Namespace(), d.OperatorName(appName), appName, d.LabelVersion(), d.Clock().Now())
}

func (d *DummyUpgradeCAASOperator) OperatorName(n string) string {
	return n + "-operator"
}

func (o *OperatorUpgraderSuite) SetUpTest(c *gc.C) {
	o.broker = &DummyUpgradeCAASOperator{
		client: fake.NewSimpleClientset(),
	}
}

func (o *OperatorUpgraderSuite) TestOperatorUpgradeToBaseCharm(c *gc.C) {
	var (
		appName        = "testinitss"
		oldImagePath   = fmt.Sprintf("%s/%s:2.9.33", podcfg.JujudOCINamespace, podcfg.JujudOCIName)
		newImagePath   = fmt.Sprintf("%s/%s:3.0.0", podcfg.JujudOCINamespace, podcfg.JujudOCIName)
		focalCharmBase = fmt.Sprintf("%s/%s:ubuntu-20.04", podcfg.JujudOCINamespace, podcfg.CharmBaseName)
		newVersion     = version.Number{Major: 3}
	)

	_, err := o.broker.Client().AppsV1().StatefulSets(o.broker.Namespace()).Create(context.TODO(),
		&apps.StatefulSet{
			ObjectMeta: meta.ObjectMeta{
				Name: o.broker.OperatorName(appName),
			},
			Spec: apps.StatefulSetSpec{
				Selector: &meta.LabelSelector{
					MatchLabels: map[string]string{
						"operator-label": "true",
					},
				},
				Template: core.PodTemplateSpec{
					Spec: core.PodSpec{
						Containers: []core.Container{
							{
								Name:  operatorContainerName,
								Image: oldImagePath,
							},
						},
					},
				},
			},
		}, meta.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	_, err = o.broker.Client().CoreV1().Pods(o.broker.Namespace()).Create(context.TODO(), &core.Pod{
		ObjectMeta: meta.ObjectMeta{
			Name: o.broker.OperatorName(appName) + "-0",
			Labels: map[string]string{
				constants.LabelJujuOperatorName:   "testinitss",
				constants.LabelJujuOperatorTarget: "application",
			},
		},
		Spec: core.PodSpec{
			Containers: []core.Container{
				{
					Name:  operatorContainerName,
					Image: oldImagePath,
				},
			},
		},
		Status: core.PodStatus{
			Phase: core.PodRunning,
		},
	}, meta.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	_, err = o.broker.Client().AppsV1().StatefulSets(o.broker.Namespace()).Create(context.TODO(),
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
			Phase: core.PodRunning,
		},
	}, meta.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	o.broker.clock = testclock.NewDilatedWallClock(10 * time.Millisecond)
	err = operatorUpgrade(appName, newVersion, o.broker)
	c.Assert(err, jc.ErrorIsNil)

	appSS, err := o.broker.Client().AppsV1().StatefulSets(o.broker.Namespace()).
		Get(context.TODO(), o.broker.DeploymentName(appName, true), meta.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(appSS.Spec.Template.Spec.InitContainers[0].Image, gc.Equals, newImagePath)

	operatorSS, err := o.broker.Client().AppsV1().StatefulSets(o.broker.Namespace()).
		Get(context.TODO(), o.broker.OperatorName(appName), meta.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(operatorSS.Spec.Template.Spec.InitContainers[0].Image, gc.Equals, newImagePath)
	c.Assert(operatorSS.Spec.Template.Spec.InitContainers[0].VolumeMounts, gc.DeepEquals, []v1.VolumeMount{{
		Name:      "juju-bins",
		MountPath: "/opt/juju",
	}})
	c.Assert(operatorSS.Spec.Template.Spec.Containers[0].Image, gc.Equals, focalCharmBase)
	c.Assert(operatorSS.Spec.Template.Spec.Containers[0].Args, gc.DeepEquals, []string{"-c", "export JUJU_DATA_DIR=/var/lib/juju\nexport JUJU_TOOLS_DIR=$JUJU_DATA_DIR/tools\n\nmkdir -p $JUJU_TOOLS_DIR\ncp /opt/juju/jujud $JUJU_TOOLS_DIR/jujud\n\nexec $JUJU_TOOLS_DIR/jujud caasoperator --application-name=testinitss --debug\n"})
	c.Assert(operatorSS.Spec.Template.Spec.Containers[0].VolumeMounts, gc.DeepEquals, []v1.VolumeMount{{
		Name:      "juju-bins",
		MountPath: "/opt/juju",
	}})
	c.Assert(operatorSS.Spec.Template.Spec.Volumes, gc.DeepEquals, []v1.Volume{{
		Name: "juju-bins",
		VolumeSource: v1.VolumeSource{
			EmptyDir: &v1.EmptyDirVolumeSource{},
		}},
	})
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

	podChecker, err := workloadInitUpgrade(appName, newImagePath, o.broker)
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

	podChecker, err := workloadInitUpgrade(appName, newImagePath, o.broker)
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

	podChecker, err := workloadInitUpgrade(appName, newImagePath, o.broker)
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

	podChecker, err := workloadInitUpgrade(appName, newImagePath, o.broker)
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
