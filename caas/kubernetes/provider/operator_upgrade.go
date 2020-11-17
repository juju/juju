// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"encoding/json"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/version"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/cloudconfig/podcfg"
)

type upgradeCAASOperatorBridge struct {
	clientFn         func() kubernetes.Interface
	clockFn          func() clock.Clock
	deploymentNameFn func(string, bool) string
	namespaceFn      func() string
	operatorFn       func(string) (*caas.Operator, error)
	operatorNameFn   func(string) string
}

type UpgradeCAASOperatorBroker interface {
	// Clock provides the clock to use with this broker for time operations
	Clock() clock.Clock

	// Client returns a Kubernetes client associated with the current broker's
	// cluster
	Client() kubernetes.Interface

	// Returns the deployment name use for the given application name, supports
	// finding legacy deployment names if set to True.
	DeploymentName(string, bool) string

	Namespace() string

	// Operator returns an Operator with current status and life details.
	Operator(string) (*caas.Operator, error)

	// OperatorName returns the operator name used for the operator deployment
	// for the supplied application.
	OperatorName(string) string
}

const applicationUpgradeTimeoutSeconds = time.Second * 30

func (u *upgradeCAASOperatorBridge) Client() kubernetes.Interface {
	return u.clientFn()
}

func (u *upgradeCAASOperatorBridge) Clock() clock.Clock {
	return u.clockFn()
}

func (u *upgradeCAASOperatorBridge) DeploymentName(n string, l bool) string {
	return u.deploymentNameFn(n, l)
}

func (u *upgradeCAASOperatorBridge) Operator(n string) (*caas.Operator, error) {
	return u.operatorFn(n)
}

func (u *upgradeCAASOperatorBridge) OperatorName(n string) string {
	return u.operatorNameFn(n)
}

func (u *upgradeCAASOperatorBridge) Namespace() string {
	return u.namespaceFn()
}

func operatorInitUpgrade(appName, imagePath string, broker UpgradeCAASOperatorBroker) (func() (bool, error), error) {
	deploymentName := broker.DeploymentName(appName, true)

	var data []byte
	var selector labels.Set
	podChecker := func(appName string,
		labelSet labels.Set,
		broker UpgradeCAASOperatorBroker) func() (bool, error) {

		return func() (done bool, err error) {
			labelSelector := labelSetToSelector(labelSet).String()
			podList, err := broker.Client().CoreV1().Pods(broker.Namespace()).
				List(meta.ListOptions{
					LabelSelector: labelSelector,
				})
			if k8serrors.IsNotFound(err) || (err == nil && len(podList.Items) == 0) {
				// Not found means not ready.
				logger.Tracef("listing pod %q, not found yet", selector.String())
				return false, nil
			} else if err != nil {
				return false, errors.Trace(err)
			}
			pod := podList.Items[0]
			if pod.Status.Phase != core.PodRunning {
				logger.Debugf(
					"init container %q is still upgrade, current status -> %q",
					appName, pod.Status.Phase)
				return false, nil
			}

			index, found := findJujudContainer(pod.Spec.InitContainers)
			if !found {
				logger.Debugf("init container for app %q not found", appName)
				return false, nil
			}

			return pod.Spec.InitContainers[index].Image == imagePath, nil
		}
	}

	ssInterface := broker.Client().AppsV1().StatefulSets(broker.Namespace())
	sResource, err := ssInterface.Get(deploymentName, meta.GetOptions{})
	if err != nil && !k8serrors.IsNotFound(err) {
		return nil, errors.Annotatef(err, "getting statefulset %q", deploymentName)
	} else if err == nil {
		selector = sResource.Spec.Selector.MatchLabels
		if err := ensureJujuInitContainer(&sResource.Spec.Template.Spec, imagePath); err != nil {
			return nil, errors.Trace(err)
		}
		if data, err = json.Marshal(apps.StatefulSet{Spec: sResource.Spec}); err != nil {
			return nil, errors.Trace(err)
		}
		_, err = ssInterface.Patch(sResource.GetName(), types.StrategicMergePatchType, data)
		return podChecker(deploymentName, selector, broker), errors.Trace(err)
	}

	deInterface := broker.Client().AppsV1().Deployments(broker.Namespace())
	deResource, err := deInterface.Get(deploymentName, meta.GetOptions{})
	if err != nil && !k8serrors.IsNotFound(err) {
		return nil, errors.Trace(err)
	} else if err == nil {
		selector = deResource.Spec.Selector.MatchLabels
		if err := ensureJujuInitContainer(&deResource.Spec.Template.Spec, imagePath); err != nil {
			return nil, errors.Trace(err)
		}
		if data, err = json.Marshal(apps.Deployment{Spec: deResource.Spec}); err != nil {
			return nil, errors.Trace(err)
		}
		_, err = deInterface.Patch(deResource.GetName(), types.StrategicMergePatchType, data)
		return podChecker(deploymentName, selector, broker), errors.Trace(err)
	}

	dsInterface := broker.Client().AppsV1().DaemonSets(broker.Namespace())
	dsResource, err := dsInterface.Get(deploymentName, meta.GetOptions{})
	if err != nil && !k8serrors.IsNotFound(err) {
		return nil, errors.Trace(err)
	} else if err == nil {
		selector = dsResource.Spec.Selector.MatchLabels
		if err := ensureJujuInitContainer(&dsResource.Spec.Template.Spec, imagePath); err != nil {
			return nil, errors.Trace(err)
		}
		if data, err = json.Marshal(apps.DaemonSet{Spec: dsResource.Spec}); err != nil {
			return nil, errors.Trace(err)
		}
		_, err = dsInterface.Patch(dsResource.GetName(), types.StrategicMergePatchType, data)
		return podChecker(deploymentName, selector, broker), errors.Trace(err)
	}

	return nil, errors.NotFoundf("deployment %q init containers", deploymentName)
}

func operatorUpgrade(appName string, vers version.Number, broker UpgradeCAASOperatorBroker) error {
	operator, err := broker.Operator(appName)
	if err != nil {
		return errors.Trace(err)
	}

	operatorImagePath := podcfg.RebuildOldOperatorImagePath(operator.Config.OperatorImagePath, vers)
	if len(operatorImagePath) == 0 {
		// This should never happen.
		return errors.NotValidf("no resource is upgradable for application %q", appName)
	}

	podChecker, err := operatorInitUpgrade(appName, operatorImagePath, broker)
	if err != nil {
		return errors.Trace(err)
	}

	timeout := broker.Clock().After(applicationUpgradeTimeoutSeconds)
	for {
		select {
		case <-timeout:
			return errors.Timeoutf("timeout while waiting for the upgraded operator of %q ready", appName)
		case <-broker.Clock().After(time.Second):
			// TODO(caas): change to use k8s watcher to trigger the polling.
			ready, err := podChecker()
			if err != nil {
				return errors.Trace(err)
			}
			if ready {
				logger.Infof("init container has been upgraded to %q, now the operator for %q starts to upgrade", operatorImagePath, appName)
				return errors.Trace(upgradeStatefulSet(
					broker.OperatorName(appName),
					operatorImagePath,
					vers,
					broker.Client().AppsV1().StatefulSets(broker.Namespace())))
			}
		}
	}
}

func (k *kubernetesClient) upgradeOperator(agentTag names.Tag, vers version.Number) error {
	broker := &upgradeCAASOperatorBridge{
		clientFn:         k.client,
		clockFn:          func() clock.Clock { return k.clock },
		deploymentNameFn: k.deploymentName,
		namespaceFn:      k.GetCurrentNamespace,
		operatorFn:       k.Operator,
		operatorNameFn:   k.operatorName,
	}
	return operatorUpgrade(agentTag.Id(), vers, broker)
}
