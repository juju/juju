// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasadmission_test

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	admission "k8s.io/api/admissionregistration/v1beta1"

	"github.com/juju/juju/worker/caasadmission"
)

type AdmissionSuite struct {
}

type dummyAdmissionCreator struct {
	EnsureMutatingWebhookConfigurationFunc func() (func(), error)
}

type dummyCertBroker struct {
	RegisterDomainLeasesFunc func(...string) (func(), error)
	RootCertificatesFunc     func() ([]byte, error)
}

var _ = gc.Suite(&AdmissionSuite{})

func (d *dummyAdmissionCreator) EnsureMutatingWebhookConfiguration() (func(), error) {
	if d.EnsureMutatingWebhookConfigurationFunc == nil {
		return func() {}, nil
	}
	return d.EnsureMutatingWebhookConfigurationFunc()
}

func int32Ptr(i int32) *int32 {
	return &i
}

func (d *dummyCertBroker) RegisterDomainLeases(s ...string) (func(), error) {
	if d.RegisterDomainLeasesFunc != nil {
		return d.RegisterDomainLeasesFunc(s...)
	}
	return func() {}, nil
}

func (d *dummyCertBroker) RootCertificates() ([]byte, error) {
	if d.RootCertificatesFunc != nil {
		return d.RootCertificatesFunc()
	}
	return []byte{}, nil
}

func strPtr(s string) *string {
	return &s
}

func (a *AdmissionSuite) TestAdmissionCreatorObject(c *gc.C) {
	var (
		certBrokerDeregisterCalled       = false
		certBrokerRegisterCalled         = false
		certBrokerRootsCalled            = false
		ensureWebhookCalled              = false
		ensureWebhookCleanupCalled       = false
		namespace                        = "testns"
		path                             = "/test"
		port                       int32 = 1111
		svcName                          = "testsvc"
	)

	certBroker := &dummyCertBroker{
		RegisterDomainLeasesFunc: func(d ...string) (func(), error) {
			certBrokerRegisterCalled = true
			c.Assert(d, jc.DeepEquals, []string{
				fmt.Sprintf("%s.%s.svc", svcName, namespace),
			})
			return func() { certBrokerDeregisterCalled = true }, nil
		},
		RootCertificatesFunc: func() ([]byte, error) {
			certBrokerRootsCalled = true
			return []byte{}, nil
		},
	}

	serviceRef := &admission.ServiceReference{
		Namespace: namespace,
		Name:      svcName,
		Path:      strPtr(path),
		Port:      int32Ptr(port),
	}

	admissionCreator, err := caasadmission.NewAdmissionCreator(
		certBroker, "testns", "testmodel",
		func(obj *admission.MutatingWebhookConfiguration) (func(), error) {
			ensureWebhookCalled = true

			c.Assert(obj.Namespace, gc.Equals, namespace)
			c.Assert(len(obj.Webhooks), gc.Equals, 1)
			c.Assert(obj.Webhooks[0].ClientConfig.Service.Name, gc.Equals, svcName)
			c.Assert(obj.Webhooks[0].ClientConfig.Service.Namespace, gc.Equals, namespace)
			c.Assert(*obj.Webhooks[0].ClientConfig.Service.Path, gc.Equals, path)
			c.Assert(*obj.Webhooks[0].ClientConfig.Service.Port, gc.Equals, port)

			return func() { ensureWebhookCleanupCalled = true }, nil
		}, serviceRef)

	c.Assert(err, jc.ErrorIsNil)

	cleanup, err := admissionCreator.EnsureMutatingWebhookConfiguration()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(certBrokerRegisterCalled, jc.IsTrue)
	c.Assert(certBrokerRootsCalled, jc.IsTrue)
	c.Assert(ensureWebhookCalled, jc.IsTrue)

	cleanup()
	c.Assert(certBrokerDeregisterCalled, jc.IsTrue)
	c.Assert(ensureWebhookCleanupCalled, jc.IsTrue)
}
