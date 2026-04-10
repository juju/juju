// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	coresecrets "github.com/juju/juju/core/secrets"
	coretesting "github.com/juju/juju/testing"
)

type backendSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&backendSuite{})

func (s *backendSuite) newBackend(client *k8sfake.Clientset) *k8sBackend {
	return &k8sBackend{
		namespace: "test-ns",
		modelName: "test-model",
		modelUUID: coretesting.ModelTag.Id(),
		client:    client,
		clock:     testclock.NewDilatedWallClock(time.Millisecond),
	}
}

func (s *backendSuite) TestIsFatalError(c *gc.C) {
	gr := schema.GroupResource{Resource: "secrets"}
	tests := []struct {
		description string
		err         error
		expectFatal bool
	}{
		{
			description: "ServiceUnavailable is retryable",
			err:         k8serrors.NewServiceUnavailable("etcd unavailable"),
			expectFatal: false,
		},
		{
			description: "InternalError is retryable",
			err:         k8serrors.NewInternalError(fmt.Errorf("internal server error")),
			expectFatal: false,
		},
		{
			description: "Forbidden is retryable",
			err:         k8serrors.NewForbidden(gr, "my-secret", fmt.Errorf("forbidden")),
			expectFatal: false,
		},
		{
			description: "Conflict is retryable",
			err:         k8serrors.NewConflict(gr, "my-secret", fmt.Errorf("resource version mismatch")),
			expectFatal: false,
		},
		{
			description: "TooManyRequests is retryable",
			err:         k8serrors.NewTooManyRequests("rate limit exceeded", 1),
			expectFatal: false,
		},
		{
			description: "ServerTimeout is retryable",
			err:         k8serrors.NewServerTimeout(gr, "get", 1),
			expectFatal: false,
		},
		{
			description: "NotFound is fatal",
			err:         k8serrors.NewNotFound(gr, "my-secret"),
			expectFatal: true,
		},
		{
			description: "Unauthorized is fatal",
			err:         k8serrors.NewUnauthorized("not authorised"),
			expectFatal: true,
		},
		{
			description: "BadRequest is fatal",
			err:         k8serrors.NewBadRequest("malformed request"),
			expectFatal: true,
		},
		{
			description: "AlreadyExists is fatal",
			err:         k8serrors.NewAlreadyExists(gr, "my-secret"),
			expectFatal: true,
		},
	}
	for _, t := range tests {
		c.Logf("testing: %s", t.description)
		c.Check(isFatalError(t.err), gc.Equals, t.expectFatal, gc.Commentf(t.description))
	}
}

// TestGetContentRetries checks that GetContent retries the backend get when
// transient errors are encountered.
func (s *backendSuite) TestGetContentRetries(c *gc.C) {
	ctx := context.Background()
	fakeClient := k8sfake.NewSimpleClientset()

	revisionID := coresecrets.NewURI().Name(1)
	_, err := fakeClient.CoreV1().Secrets("test-ns").Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      revisionID,
			Namespace: "test-ns",
		},
		Data: map[string][]byte{
			"foo": []byte("bar"),
		},
	}, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	var callCount atomic.Int32
	fakeClient.PrependReactor("get", "secrets", func(
		k8stesting.Action,
	) (bool, k8sruntime.Object, error) {
		if callCount.Add(1) <= 2 {
			return true, nil, k8serrors.NewServiceUnavailable(
				"etcd temporarily unavailable")
		}
		return false, nil, nil
	})

	b := s.newBackend(fakeClient)
	content, err := b.GetContent(ctx, revisionID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(content.EncodedValues(), jc.DeepEquals, map[string]string{
		"foo": "YmFy", // base64("bar")
	})
	// Two failures and a success.
	c.Check(callCount.Load(), gc.Equals, int32(3))
}

// TestGetContentNotFound checks that GetContent does not retry when
// the secret is NotFound.
func (s *backendSuite) TestGetContentNotFound(c *gc.C) {
	ctx := context.Background()
	fakeClient := k8sfake.NewSimpleClientset()

	var callCount atomic.Int32
	fakeClient.PrependReactor("get", "secrets", func(
		k8stesting.Action,
	) (bool, k8sruntime.Object, error) {
		callCount.Add(1)
		return false, nil, nil
	})

	b := s.newBackend(fakeClient)
	_, err := b.GetContent(ctx, "nonexistent-secret")
	c.Assert(err, gc.NotNil)
	c.Check(errors.IsNotFound(err), jc.IsTrue)
	c.Check(callCount.Load(), gc.Equals, int32(1))
}

// TestSaveContentRetries checks SaveContent retries when the backend returns
// a transient error.
func (s *backendSuite) TestSaveContentRetries(c *gc.C) {
	ctx := context.Background()
	fakeClient := k8sfake.NewSimpleClientset()

	var patchCount atomic.Int32
	fakeClient.PrependReactor("patch", "secrets", func(
		k8stesting.Action,
	) (bool, k8sruntime.Object, error) {
		if patchCount.Add(1) <= 2 {
			return true, nil, k8serrors.NewServiceUnavailable(
				"etcd temporarily unavailable")
		}
		return false, nil, nil
	})

	b := s.newBackend(fakeClient)
	uri := coresecrets.NewURI()
	sv := coresecrets.NewSecretValue(map[string]string{"foo": "YmFy"}) // base64("bar")
	name, err := b.SaveContent(ctx, uri, 1, sv)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(name, gc.Equals, uri.Name(1))
	// Two failures and a success.
	c.Check(patchCount.Load(), gc.Equals, int32(3))
}

// TestSaveContentError checks that SaveContent does not retry on unknown
// errors.
func (s *backendSuite) TestSaveContentError(c *gc.C) {
	ctx := context.Background()
	fakeClient := k8sfake.NewSimpleClientset()

	var patchCount atomic.Int32
	fakeClient.PrependReactor("patch", "secrets", func(
		k8stesting.Action,
	) (bool, k8sruntime.Object, error) {
		patchCount.Add(1)
		return true, nil, k8serrors.NewBadRequest("unsupported patch operation")
	})

	b := s.newBackend(fakeClient)
	uri := coresecrets.NewURI()
	sv := coresecrets.NewSecretValue(map[string]string{"foo": "YmFy"})
	_, err := b.SaveContent(ctx, uri, 1, sv)
	c.Assert(err, gc.NotNil)
	c.Check(k8serrors.IsBadRequest(err), jc.IsTrue)
	c.Check(patchCount.Load(), gc.Equals, int32(1))
}

// TestDeleteContentRetries checks that DeleteContent retries when the backend
// returns a trainsient error.
func (s *backendSuite) TestDeleteContentRetries(c *gc.C) {
	ctx := context.Background()
	fakeClient := k8sfake.NewSimpleClientset()

	revisionID := coresecrets.NewURI().Name(1)
	_, err := fakeClient.CoreV1().Secrets("test-ns").Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      revisionID,
			Namespace: "test-ns",
		},
	}, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	var deleteCount atomic.Int32
	fakeClient.PrependReactor("delete", "secrets", func(
		k8stesting.Action,
	) (bool, k8sruntime.Object, error) {
		if deleteCount.Add(1) <= 2 {
			return true, nil, k8serrors.NewServiceUnavailable(
				"etcd temporarily unavailable")
		}
		return false, nil, nil
	})

	b := s.newBackend(fakeClient)
	err = b.DeleteContent(ctx, revisionID)
	c.Assert(err, jc.ErrorIsNil)
	// Two failures and a success.
	c.Check(deleteCount.Load(), gc.Equals, int32(3))

	// Confirm the secret revision was actually deleted.
	list, err := fakeClient.CoreV1().Secrets("test-ns").List(ctx, metav1.ListOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(list.Items, gc.HasLen, 0)
}

// TestDeleteContentError checks that DeleteContent does not retry on unknown
// errors.
func (s *backendSuite) TestDeleteContentError(c *gc.C) {
	ctx := context.Background()
	fakeClient := k8sfake.NewSimpleClientset()

	revisionID := coresecrets.NewURI().Name(1)
	_, err := fakeClient.CoreV1().Secrets("test-ns").Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      revisionID,
			Namespace: "test-ns",
		},
	}, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	var deleteCount atomic.Int32
	fakeClient.PrependReactor("delete", "secrets", func(
		k8stesting.Action,
	) (bool, k8sruntime.Object, error) {
		deleteCount.Add(1)
		return true, nil, k8serrors.NewBadRequest("delete not permitted")
	})

	b := s.newBackend(fakeClient)
	err = b.DeleteContent(ctx, revisionID)
	c.Assert(k8serrors.IsBadRequest(err), jc.IsTrue)
	c.Check(deleteCount.Load(), gc.Equals, int32(1))
}
