// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"

	"github.com/juju/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
)

const maxEventsToPage = 100

// ListEventsForObject returns all the events for the specified object.
func ListEventsForObject(
	ctx context.Context, coreClient kubernetes.Interface, namespace, name, kind string,
) ([]corev1.Event, error) {
	selector := fields.AndSelectors(
		fields.OneTermEqualSelector("involvedObject.name", name),
		fields.OneTermEqualSelector("involvedObject.kind", kind),
	).String()
	opts := metav1.ListOptions{
		FieldSelector: selector,
	}
	var items []corev1.Event
	for len(items) < maxEventsToPage {
		res, err := coreClient.CoreV1().Events(namespace).List(ctx, opts)
		if err != nil {
			return nil, errors.Trace(err)
		}
		items = append(items, res.Items...)
		if res.RemainingItemCount == nil || *res.RemainingItemCount == 0 {
			break
		}
		opts.Continue = res.Continue
	}
	return items, nil
}
