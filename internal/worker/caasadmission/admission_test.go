// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasadmission_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	admission "k8s.io/api/admissionregistration/v1"

	"github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/internal/worker/caasadmission"
	pkitest "github.com/juju/juju/pki/test"
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
		authority, "testns", "testmodel", "deadbeef", "badf00d", constants.LabelVersion1,
		func(obj *admission.MutatingWebhookConfiguration) (func(), error) {
			ensureWebhookCalled = true

			c.Assert(obj.Namespace, gc.Equals, namespace)
			c.Assert(len(obj.Webhooks), gc.Equals, 1)
			webhook := obj.Webhooks[0]
			c.Assert(webhook.AdmissionReviewVersions, gc.DeepEquals, []string{"v1beta1"})
			c.Assert(webhook.SideEffects, gc.NotNil)
			c.Assert(*webhook.SideEffects, gc.Equals, admission.SideEffectClassNone)
			svc := webhook.ClientConfig.Service
			c.Assert(svc.Name, gc.Equals, svcName)
			c.Assert(svc.Namespace, gc.Equals, namespace)
			c.Assert(*svc.Path, gc.Equals, path)
			c.Assert(*svc.Port, gc.Equals, port)

			return func() { ensureWebhookCleanupCalled = true }, nil
		}, serviceRef)

	c.Assert(err, jc.ErrorIsNil)

	cleanup, err := admissionCreator.EnsureMutatingWebhookConfiguration()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ensureWebhookCalled, jc.IsTrue)

	cleanup()
	c.Assert(ensureWebhookCleanupCalled, jc.IsTrue)
}
