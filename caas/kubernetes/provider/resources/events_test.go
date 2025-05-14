// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"sort"
	"strconv"

	"github.com/juju/tc"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/caas/kubernetes/provider/resources"
)

type eventsSuite struct {
	resourceSuite
}

var _ = tc.Suite(&eventsSuite{})

func (s *eventsSuite) TestList(c *tc.C) {
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
		_, err := s.client.CoreV1().Events("test").Create(c.Context(), &ev, metav1.CreateOptions{})
		c.Assert(err, tc.ErrorIsNil)
		res = append(res, ev)
	}
	events, err := resources.ListEventsForObject(c.Context(), s.client, "test", "test", "Pod")
	c.Assert(err, tc.ErrorIsNil)

	toInt := func(s string) int {
		i, err := strconv.Atoi(s)
		c.Assert(err, tc.ErrorIsNil)
		return i
	}
	sort.Slice(events[:], func(i, j int) bool {
		return toInt(events[i].Name) < toInt(events[j].Name)
	})
	c.Assert(events, tc.DeepEquals, res)
}
