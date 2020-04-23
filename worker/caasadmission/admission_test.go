// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasadmission_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	admission "k8s.io/api/admissionregistration/v1beta1"

	pkitest "github.com/juju/juju/pki/test"
	"github.com/juju/juju/worker/caasadmission"
)

type AdmissionSuite struct {
}

type dummyAdmissionCreator struct {
	EnsureMutatingWebhookConfigurationFunc func() (func(), error)
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

func strPtr(s string) *string {
	return &s
}

func (a *AdmissionSuite) TestAdmissionCreatorObject(c *gc.C) {
	var (
		ensureWebhookCalled              = false
		ensureWebhookCleanupCalled       = false
		namespace                        = "testns"
		path                             = "/test"
		port                       int32 = 1111
		svcName                          = "testsvc"
	)

	authority, err := pkitest.NewTestAuthority()
	c.Assert(err, jc.ErrorIsNil)

	serviceRef := &admission.ServiceReference{
		Namespace: namespace,
		Name:      svcName,
		Path:      strPtr(path),
		Port:      int32Ptr(port),
	}

	admissionCreator, err := caasadmission.NewAdmissionCreator(
		authority, "testns", "testmodel",
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
	c.Assert(ensureWebhookCalled, jc.IsTrue)

	cleanup()
	c.Assert(ensureWebhookCleanupCalled, jc.IsTrue)
}
