// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasadmission_test

import (
	"context"
	"testing"

	"github.com/juju/tc"
	admission "k8s.io/api/admissionregistration/v1"

	pkitest "github.com/juju/juju/internal/pki/test"
	"github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/worker/caasadmission"
)

type AdmissionSuite struct {
}

type dummyAdmissionCreator struct {
	EnsureMutatingWebhookConfigurationFunc func(ctx context.Context) (func(), error)
}

func TestAdmissionSuite(t *testing.T) {
	tc.Run(t, &AdmissionSuite{})
}

func (d *dummyAdmissionCreator) EnsureMutatingWebhookConfiguration(ctx context.Context) (func(), error) {
	if d.EnsureMutatingWebhookConfigurationFunc == nil {
		return func() {}, nil
	}
	return d.EnsureMutatingWebhookConfigurationFunc(ctx)
}

func int32Ptr(i int32) *int32 {
	return &i
}

func strPtr(s string) *string {
	return &s
}

func (a *AdmissionSuite) TestAdmissionCreatorObject(c *tc.C) {
	var (
		ensureWebhookCalled              = false
		ensureWebhookCleanupCalled       = false
		namespace                        = "testns"
		path                             = "/test"
		port                       int32 = 1111
		svcName                          = "testsvc"
	)

	authority, err := pkitest.NewTestAuthority()
	c.Assert(err, tc.ErrorIsNil)

	serviceRef := &admission.ServiceReference{
		Namespace: namespace,
		Name:      svcName,
		Path:      strPtr(path),
		Port:      int32Ptr(port),
	}

	admissionCreator, err := caasadmission.NewAdmissionCreator(
		authority, "testns", "testmodel", "deadbeef", "badf00d", constants.LabelVersion1,
		func(_ context.Context, obj *admission.MutatingWebhookConfiguration) (func(), error) {
			ensureWebhookCalled = true

			c.Assert(obj.Namespace, tc.Equals, namespace)
			c.Assert(len(obj.Webhooks), tc.Equals, 1)
			webhook := obj.Webhooks[0]
			c.Assert(webhook.AdmissionReviewVersions, tc.DeepEquals, []string{"v1beta1"})
			c.Assert(webhook.SideEffects, tc.NotNil)
			c.Assert(*webhook.SideEffects, tc.Equals, admission.SideEffectClassNone)
			svc := webhook.ClientConfig.Service
			c.Assert(svc.Name, tc.Equals, svcName)
			c.Assert(svc.Namespace, tc.Equals, namespace)
			c.Assert(*svc.Path, tc.Equals, path)
			c.Assert(*svc.Port, tc.Equals, port)

			return func() { ensureWebhookCleanupCalled = true }, nil
		}, serviceRef)

	c.Assert(err, tc.ErrorIsNil)

	cleanup, err := admissionCreator.EnsureMutatingWebhookConfiguration(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ensureWebhookCalled, tc.IsTrue)

	cleanup()
	c.Assert(ensureWebhookCleanupCalled, tc.IsTrue)
}
