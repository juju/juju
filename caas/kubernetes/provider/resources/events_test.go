// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"context"
	"strconv"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/caas/kubernetes/provider/resources"
)

type eventsSuite struct {
	resourceSuite
}

var _ = gc.Suite(&eventsSuite{})

func (s *eventsSuite) TestList(c *gc.C) {
	template := corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ev",
			Namespace: "test",
		},
		InvolvedObject: corev1.ObjectReference{
			Name: "test",
			Kind: "Pod",
		},
	}
	res := []corev1.Event{}
	for i := 0; i < 1000; i++ {
		ev := template
		ev.ObjectMeta.Name += strconv.Itoa(i)
		_, err := s.client.CoreV1().Events("test").Create(context.TODO(), &ev, metav1.CreateOptions{})
		c.Assert(err, jc.ErrorIsNil)
		res = append(res, ev)
	}
	events, err := resources.ListEventsForObject(context.TODO(), s.client, "test", "test", "Pod")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(events, jc.DeepEquals, res)
}
