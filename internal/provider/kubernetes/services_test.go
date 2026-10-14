// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/juju/juju/internal/provider/kubernetes/constants"
)

type servicesSuite struct {
	client *fake.Clientset
}

func TestServicesSuite(t *testing.T) {
	tc.Run(t, &servicesSuite{})
}

func (s *servicesSuite) SetUpTest(c *tc.C) {
	s.client = fake.NewSimpleClientset()
}

func (s *servicesSuite) TestFindServiceForApplication(c *tc.C) {
	_, err := s.client.CoreV1().Services("test").Create(
		c.Context(),
		&core.Service{
			ObjectMeta: meta.ObjectMeta{
				Name: "wallyworld",
				Labels: map[string]string{
					"app.kubernetes.io/name":       "wallyworld",
					"app.kubernetes.io/managed-by": "juju",
				},
			},
		},
		meta.CreateOptions{},
	)

	c.Assert(err, tc.ErrorIsNil)

	svc, err := findServiceForApplication(
		c.Context(),
		s.client.CoreV1().Services("test"),
		"wallyworld",
		constants.LabelVersion1,
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(svc.Name, tc.Equals, "wallyworld")
}

func (s *servicesSuite) TestFindServiceForApplicationWithEndpoints(c *tc.C) {
	_, err := s.client.CoreV1().Services("test").Create(
		c.Context(),
		&core.Service{
			ObjectMeta: meta.ObjectMeta{
				Name: "wallyworld",
				Labels: map[string]string{
					"app.kubernetes.io/name":       "wallyworld",
					"app.kubernetes.io/managed-by": "juju",
				},
			},
		},
		meta.CreateOptions{},
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.client.CoreV1().Services("test").Create(
		c.Context(),
		&core.Service{
			ObjectMeta: meta.ObjectMeta{
				Name: "wallyworld-endpoints",
				Labels: map[string]string{
					"app.kubernetes.io/name":       "wallyworld",
					"app.kubernetes.io/managed-by": "juju",
				},
			},
		},
		meta.CreateOptions{},
	)
	c.Assert(err, tc.ErrorIsNil)

	svc, err := findServiceForApplication(
		c.Context(),
		s.client.CoreV1().Services("test"),
		"wallyworld",
		constants.LabelVersion1,
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(svc.Name, tc.Equals, "wallyworld")
}

func (s *servicesSuite) TestFindServiceForApplicationWithLabelledHeadless(c *tc.C) {
	_, err := s.client.CoreV1().Services("test").Create(
		c.Context(),
		&core.Service{
			ObjectMeta: meta.ObjectMeta{
				Name: "wallyworld",
				Labels: map[string]string{
					"app.kubernetes.io/name":       "wallyworld",
					"app.kubernetes.io/managed-by": "juju",
				},
			},
		},
		meta.CreateOptions{},
	)
	c.Assert(err, tc.ErrorIsNil)

	// A headless service whose name does not end in "-endpoints"; it must be
	// filtered out via the LabelJujuServiceType label rather than the legacy
	// suffix.
	_, err = s.client.CoreV1().Services("test").Create(
		c.Context(),
		&core.Service{
			ObjectMeta: meta.ObjectMeta{
				Name: "wallyworld-dqlite",
				Labels: map[string]string{
					"app.kubernetes.io/name":       "wallyworld",
					"app.kubernetes.io/managed-by": "juju",
					constants.LabelJujuServiceType: constants.ServiceTypeEndpoints,
				},
			},
		},
		meta.CreateOptions{},
	)
	c.Assert(err, tc.ErrorIsNil)

	svc, err := findServiceForApplication(
		c.Context(),
		s.client.CoreV1().Services("test"),
		"wallyworld",
		constants.LabelVersion1,
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(svc.Name, tc.Equals, "wallyworld")
}

func (s *servicesSuite) TestFindServiceForApplicationWithMultiple(c *tc.C) {
	_, err := s.client.CoreV1().Services("test").Create(
		c.Context(),
		&core.Service{
			ObjectMeta: meta.ObjectMeta{
				Name: "wallyworld",
				Labels: map[string]string{
					"app.kubernetes.io/name":       "wallyworld",
					"app.kubernetes.io/managed-by": "juju",
				},
			},
		},
		meta.CreateOptions{},
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.client.CoreV1().Services("test").Create(
		c.Context(),
		&core.Service{
			ObjectMeta: meta.ObjectMeta{
				Name: "wallyworld-v2",
				Labels: map[string]string{
					"app.kubernetes.io/name":       "wallyworld",
					"app.kubernetes.io/managed-by": "juju",
				},
			},
		},
		meta.CreateOptions{},
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = findServiceForApplication(
		c.Context(),
		s.client.CoreV1().Services("test"),
		"wallyworld",
		constants.LabelVersion1,
	)

	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *servicesSuite) TestFindServiceForApplicationMissing(c *tc.C) {
	_, err := findServiceForApplication(
		c.Context(),
		s.client.CoreV1().Services("test"),
		"wallyworld",
		constants.LabelVersion1,
	)

	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

func (s *servicesSuite) createControllerService(c *tc.C, ports ...core.ServicePort) {
	_, err := s.client.CoreV1().Services("test").Create(
		c.Context(),
		&core.Service{
			ObjectMeta: meta.ObjectMeta{Name: constants.ControllerServiceName},
			Spec:       core.ServiceSpec{Ports: ports},
		},
		meta.CreateOptions{},
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *servicesSuite) controllerServicePorts(c *tc.C) []core.ServicePort {
	svc, err := s.client.CoreV1().Services("test").Get(c.Context(), constants.ControllerServiceName, meta.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	return svc.Spec.Ports
}

func (s *servicesSuite) TestUpdateControllerServiceSSHPort(c *tc.C) {
	s.createControllerService(c,
		core.ServicePort{Name: "api-server", Port: 17070, TargetPort: intstr.FromInt(17070)},
		core.ServicePort{Name: "ssh-server", Port: 17022, TargetPort: intstr.FromInt(17022)},
	)

	// The ssh server port moves, the api server port is left alone.
	err := updateControllerServiceSSHPort(c.Context(), s.client.CoreV1().Services("test"), 17099)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(s.controllerServicePorts(c), tc.DeepEquals, []core.ServicePort{
		{Name: "api-server", Port: 17070, TargetPort: intstr.FromInt(17070)},
		{Name: "ssh-server", Port: 17099, TargetPort: intstr.FromInt(17099)},
	})
}

func (s *servicesSuite) TestUpdateControllerServiceSSHPortUnchanged(c *tc.C) {
	s.createControllerService(c,
		core.ServicePort{Name: "ssh-server", Port: 17022, TargetPort: intstr.FromInt(17022)},
	)
	s.client.ClearActions()

	// Nothing to do, so the Service isn't updated.
	err := updateControllerServiceSSHPort(c.Context(), s.client.CoreV1().Services("test"), 17022)
	c.Assert(err, tc.ErrorIsNil)
	for _, action := range s.client.Actions() {
		c.Check(action.GetVerb(), tc.Not(tc.Equals), "update")
	}
}

func (s *servicesSuite) TestUpdateControllerServiceSSHPortMissingPort(c *tc.C) {
	s.createControllerService(c,
		core.ServicePort{Name: "api-server", Port: 17070, TargetPort: intstr.FromInt(17070)},
	)

	// A Service without the ssh server port gets it added.
	err := updateControllerServiceSSHPort(c.Context(), s.client.CoreV1().Services("test"), 17099)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(s.controllerServicePorts(c), tc.DeepEquals, []core.ServicePort{
		{Name: "api-server", Port: 17070, TargetPort: intstr.FromInt(17070)},
		{Name: "ssh-server", Port: 17099, TargetPort: intstr.FromInt(17099)},
	})
}

func (s *servicesSuite) TestUpdateControllerServiceSSHPortNoService(c *tc.C) {
	err := updateControllerServiceSSHPort(c.Context(), s.client.CoreV1().Services("test"), 17099)
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}
