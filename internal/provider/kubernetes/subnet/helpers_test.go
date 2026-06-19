// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnet

import (
	"testing"

	"github.com/juju/tc"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type helpersSuite struct{}

func TestHelpers(t *testing.T) {
	tc.Run(t, &helpersSuite{})
}

func (s *helpersSuite) TestManagedCloudDetected(c *tc.C) {
	clients := newClusterBuilder().addCore(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{
			Name:   "a",
			Labels: map[string]string{"eks.amazonaws.com/nodegroup": "ng-1"},
		}},
	).clients()

	c.Check(detectManagedCloud(c.Context(), clients), tc.Equals, "ec2")
}

func (s *helpersSuite) TestManagedCloudExcludesMicroK8s(c *tc.C) {
	clients := newClusterBuilder().addCore(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{
			Name:   "a",
			Labels: map[string]string{"microk8s.io/cluster": "true"},
		}},
	).clients()

	c.Check(detectManagedCloud(c.Context(), clients), tc.Equals, "")
}
