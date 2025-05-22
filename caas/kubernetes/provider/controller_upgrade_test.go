// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"fmt"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/caas/kubernetes/provider/utils"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/cloudconfig/podcfg"
)

// DummyUpgradeCAASController implements UpgradeCAASControllerBroker for the
// purpose of testing.
type dummyUpgradeCAASController struct {
	client *fake.Clientset
}

type ControllerUpgraderSuite struct {
	broker *dummyUpgradeCAASController
}

func TestControllerUpgraderSuite(t *testing.T) {
	tc.Run(t, &ControllerUpgraderSuite{})
}

func (d *dummyUpgradeCAASController) Client() kubernetes.Interface {
	return d.client
}

func (d *dummyUpgradeCAASController) LabelVersion() k8sconstants.LabelVersion {
	return k8sconstants.LabelVersion2
}

func (d *dummyUpgradeCAASController) Namespace() string {
	return "test"
}

func (s *ControllerUpgraderSuite) SetUpTest(c *tc.C) {
	s.broker = &dummyUpgradeCAASController{
		client: fake.NewSimpleClientset(),
	}
}

func (s *ControllerUpgraderSuite) TestControllerUpgrade(c *tc.C) {
	var (
		appName      = k8sconstants.JujuControllerStackName
		oldImagePath = fmt.Sprintf("%s/%s:9.9.8", podcfg.JujudOCINamespace, podcfg.JujudOCIName)
		newImagePath = fmt.Sprintf("%s/%s:9.9.9", podcfg.JujudOCINamespace, podcfg.JujudOCIName)
	)
	_, err := s.broker.Client().AppsV1().StatefulSets(s.broker.Namespace()).Create(c.Context(),
		&apps.StatefulSet{
			ObjectMeta: meta.ObjectMeta{
				Name: appName,
			},
			Spec: apps.StatefulSetSpec{
				Selector: &meta.LabelSelector{
					MatchLabels: map[string]string{
						"match-label": "true",
					},
				},
				Template: core.PodTemplateSpec{
					Spec: core.PodSpec{
						Containers: []core.Container{
							{
								Name:  "jujud",
								Image: oldImagePath,
							},
						},
					},
				},
			},
		}, meta.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(controllerUpgrade(c.Context(), appName, semversion.MustParse("9.9.9"), s.broker), tc.ErrorIsNil)

	ss, err := s.broker.Client().AppsV1().StatefulSets(s.broker.Namespace()).
		Get(c.Context(), appName, meta.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ss.Spec.Template.Spec.Containers[0].Image, tc.Equals, newImagePath)

	c.Assert(ss.Annotations[utils.AnnotationVersionKey(k8sconstants.LabelVersion2)], tc.Equals, semversion.MustParse("9.9.9").String())
	c.Assert(ss.Spec.Template.Annotations[utils.AnnotationVersionKey(k8sconstants.LabelVersion2)], tc.Equals, semversion.MustParse("9.9.9").String())
}

func (s *ControllerUpgraderSuite) TestControllerDoesNotExist(c *tc.C) {
	var (
		appName = k8sconstants.JujuControllerStackName
	)
	err := controllerUpgrade(c.Context(), appName, semversion.MustParse("9.9.9"), s.broker)
	c.Assert(err, tc.NotNil)
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}
