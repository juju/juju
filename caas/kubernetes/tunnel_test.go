// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes_test

import (
	"testing"

	"k8s.io/client-go/rest/fake"

	"github.com/juju/juju/caas/kubernetes"
)

func TestTunnelURL(t *testing.T) {
	client := &fake.RESTClient{}
	tun := kubernetes.NewTunnel(
		client,
		nil,
		kubernetes.TunnelKindPods,
		"test",
		"test-pod",
		"wef")

	if err := tun.ForwardPort(); err != nil {
		t.Fatal(err)
	}

	t.Fatal("because I can")
}
