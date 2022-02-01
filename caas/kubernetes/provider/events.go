// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"

	"github.com/juju/errors"
	core "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/informers"

	"github.com/juju/juju/core/watcher"
)

// Constants below are copied from "k8s.io/kubernetes/pkg/kubelet/events"
// to avoid introducing the huge dependency.
// Remove them once k8s.io/kubernetes added as a dependency.
const (
	// Container event reason list
	CreatedContainer        = "Created"
	StartedContainer        = "Started"
	FailedToCreateContainer = "Failed"
	FailedToStartContainer  = "Failed"
	KillingContainer        = "Killing"
	PreemptContainer        = "Preempting"
	BackOffStartContainer   = "BackOff"
	ExceededGracePeriod     = "ExceededGracePeriod"

	// Pod event reason list
	FailedToKillPod                = "FailedKillPod"
	FailedToCreatePodContainer     = "FailedCreatePodContainer"
	FailedToMakePodDataDirectories = "Failed"
	NetworkNotReady                = "NetworkNotReady"

	// Image event reason list
	PullingImage            = "Pulling"
	PulledImage             = "Pulled"
	FailedToPullImage       = "Failed"
	FailedToInspectImage    = "InspectFailed"
	ErrImageNeverPullPolicy = "ErrImageNeverPull"
	BackOffPullImage        = "BackOff"
)

func (k *kubernetesClient) getEvents(objName string, objKind string) ([]core.Event, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	selector := fields.AndSelectors(
		fields.OneTermEqualSelector("involvedObject.name", objName),
		fields.OneTermEqualSelector("involvedObject.kind", objKind),
	).String()
	logger.Debugf("getting the latest event for %q", selector)
	eventList, err := k.client().CoreV1().Events(k.namespace).List(context.TODO(), v1.ListOptions{
		FieldSelector: selector,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return eventList.Items, nil
}

func (k *kubernetesClient) watchEvents(objName string, objKind string) (watcher.NotifyWatcher, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	factory := informers.NewSharedInformerFactoryWithOptions(k.client(), 0,
		informers.WithNamespace(k.namespace),
		informers.WithTweakListOptions(func(o *v1.ListOptions) {
			o.FieldSelector = fields.AndSelectors(
				fields.OneTermEqualSelector("involvedObject.name", objName),
				fields.OneTermEqualSelector("involvedObject.kind", objKind),
			).String()
		}),
	)
	return k.newWatcher(factory.Core().V1().Events().Informer(), objName, k.clock)
}
