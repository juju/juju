// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"context"
	"sort"
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
		ev.ObjectMeta.Name = strconv.Itoa(i)
		_, err := s.client.CoreV1().Events("test").Create(context.TODO(), &ev, metav1.CreateOptions{})
		c.Assert(err, jc.ErrorIsNil)
		res = append(res, ev)
	}
	events, err := resources.ListEventsForObject(context.TODO(), s.client.CoreV1().Events("test"), "test", "Pod")
	c.Assert(err, jc.ErrorIsNil)

	toInt := func(s string) int {
		i, err := strconv.Atoi(s)
		c.Assert(err, jc.ErrorIsNil)
		return i
	}
	sort.Slice(events[:], func(i, j int) bool {
		return toInt(events[i].Name) < toInt(events[j].Name)
	})
	c.Assert(events, jc.DeepEquals, res)
}
