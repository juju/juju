// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	gc "gopkg.in/check.v1"
	"k8s.io/client-go/kubernetes/fake"
)

type resourceSuite struct {
	client *fake.Clientset
}

func (s *resourceSuite) SetUpTest(c *gc.C) {
	s.client = fake.NewSimpleClientset()
}

func (s *resourceSuite) TearDownTest(c *gc.C) {
	s.client = nil
}
