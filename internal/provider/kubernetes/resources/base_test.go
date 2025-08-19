// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"github.com/juju/tc"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apiextensionsfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

type resourceSuite struct {
	client         kubernetes.Interface
	extendedClient clientset.Interface
}

func (s *resourceSuite) SetUpTest(c *tc.C) {
	s.client = fake.NewSimpleClientset()
	s.extendedClient = apiextensionsfake.NewSimpleClientset()
}

func (s *resourceSuite) TearDownTest(c *tc.C) {
	s.client = nil
	s.extendedClient = nil
}
