// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/version"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8slabels "k8s.io/apimachinery/pkg/labels"
	k8stypes "k8s.io/apimachinery/pkg/types"

	"github.com/juju/juju/cloudconfig/podcfg"
	k8sannotations "github.com/juju/juju/core/annotations"
	"github.com/juju/juju/core/status"
)

const applicationUpgradeTimeoutSeconds = 30

// Upgrade sets the OCI image for the app's operator to the specified version.
func (k *kubernetesClient) Upgrade(appName string, vers version.Number) error {

	isController := appName == JujuControllerStackName
	if isController {
		// Upgrade the controller pod.
		return errors.Trace(k.upgradeControllerOrOperator(appName, vers, ""))
	}

	operator, err := k.Operator(appName)
	if err != nil {
		return errors.Trace(err)
	}
	operatorImagePath := podcfg.RebuildOldOperatorImagePath(operator.Config.OperatorImagePath, vers)
	if len(operatorImagePath) == 0 {
		// This should never happen.
		return errors.NotValidf("no resource is upgradable for application %q", appName)
	}

	// To upgrade an application, we upgrade the Juju init container in the workload pod first then operator pod.
	podChecker, err := k.upgradeJujuInitContainer(appName, operatorImagePath)
	if err != nil {
		return errors.Trace(err)
	}

	timeout := k.clock.After(applicationUpgradeTimeoutSeconds * time.Second)
	for {
		select {
		case <-timeout:
			return errors.Timeoutf("timeout while waiting for the upgraded operator of %q ready", appName)
		case <-k.clock.After(1 * time.Second):
			// TODO(caas): change to use k8s watcher to trigger the polling.
			ready, err := podChecker()
			if err != nil {
				return errors.Trace(err)
			}
			if ready {
				logger.Infof("init container has been upgraded to %q, now the operator for %q starts to upgrade", operatorImagePath, appName)
				return errors.Trace(k.upgradeControllerOrOperator(k.operatorName(appName), vers, operatorImagePath))
			}
		}
	}
}

func upgradeContainer(existingContainers []core.Container, imagePath string) {
	index := findJujudContainer(existingContainers)
	if index >= 0 {
		c := existingContainers[index]
		if imagePath != c.Image {
			logger.Infof("upgrading from %q to %q", c.Image, imagePath)
			c.Image = imagePath
			existingContainers[index] = c
		}
	}
}

func findJujudContainer(containers []core.Container) (index int) {
	index = -1
	for i, c := range containers {
		if podcfg.IsJujuOCIImage(c.Image) {
			return i
		}
	}
	return index
}

func (k *kubernetesClient) upgradeControllerOrOperator(name string, vers version.Number, imagePath string) error {
	logger.Debugf("Upgrading %q", name)

	api := k.client().AppsV1().StatefulSets(k.namespace)
	existingStatefulSet, err := api.Get(name, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		// We always expect that the controller or operator statefulset resource exist.
		// So this should never happen unless it was deleted accidentally.
		return errors.Annotatef(err, "getting the existing statefulset %q to upgrade", name)
	}
	if err != nil {
		return errors.Trace(err)
	}

	if len(imagePath) == 0 {
		jujudContainerIdx := findJujudContainer(existingStatefulSet.Spec.Template.Spec.Containers)
		if jujudContainerIdx < 0 {
			return errors.NotFoundf("jujud container in statefulset %q", existingStatefulSet.GetName())
		}
		imagePath = existingStatefulSet.Spec.Template.Spec.Containers[jujudContainerIdx].Image
	}

	upgradeContainer(existingStatefulSet.Spec.Template.Spec.Containers, imagePath)

	// update juju-version annotation.
	// TODO(caas): consider how to upgrade to current annotations format safely.
	// just ensure juju-version to current version for now.
	existingStatefulSet.SetAnnotations(
		k8sannotations.New(existingStatefulSet.GetAnnotations()).
			Add(labelVersion, vers.String()).ToMap(),
	)
	existingStatefulSet.Spec.Template.SetAnnotations(
		k8sannotations.New(existingStatefulSet.Spec.Template.GetAnnotations()).
			Add(labelVersion, vers.String()).ToMap(),
	)

	_, err = api.Update(existingStatefulSet)
	return errors.Trace(err)
}

func (k *kubernetesClient) selectPod(labelSet k8slabels.Set) (*core.Pod, error) {
	labelSelector := labelSetToSelector(labelSet).String()
	podList, err := k.client().CoreV1().Pods(k.namespace).List(v1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, errors.Annotatef(err, "selecting pod with label %q", labelSelector)
	}
	if len(podList.Items) == 0 {
		return nil, errors.NotFoundf("pod for selector %q", labelSet)
	}
	return &podList.Items[0], nil
}

func (k *kubernetesClient) upgradeJujuInitContainer(appName string, imagePath string) (podChecker func() (bool, error), err error) {
	deploymentName := k.deploymentName(appName, true)

	var data []byte
	var selector k8slabels.Set
	defer func() {
		podChecker = func() (done bool, err error) {
			pod, err := k.selectPod(selector)
			if errors.IsNotFound(err) {
				// Not found means not ready.
				logger.Tracef("listing pod %q, not found yet", selector.String())
				return false, nil
			} else if err != nil {
				return false, errors.Trace(err)
			}
			_, opStatus, _, err := k.getPODStatus(*pod, k.clock.Now())
			if err != nil {
				return false, errors.Trace(err)
			}
			index := findJujudContainer(pod.Spec.InitContainers)
			msg := fmt.Sprintf("init container of %q is still upgrading, current status -> %q", appName, opStatus)
			if index >= 0 {
				msg += " | version -> " + pod.Spec.InitContainers[index].Image
			}
			defer func() {
				if !done {
					logger.Debugf(msg)
				}
			}()
			return opStatus == status.Running && index >= 0 && pod.Spec.InitContainers[index].Image == imagePath, nil
		}
	}()
	sResource, err := k.getStatefulSet(deploymentName)
	if err != nil && !errors.IsNotFound(err) {
		return podChecker, errors.Trace(err)
	} else if err == nil {
		selector = sResource.Spec.Selector.MatchLabels
		if err := ensureJujuInitContainer(&sResource.Spec.Template.Spec, imagePath); err != nil {
			return podChecker, errors.Trace(err)
		}
		if data, err = json.Marshal(apps.StatefulSet{Spec: sResource.Spec}); err != nil {
			return podChecker, errors.Trace(err)
		}
		_, err = k.client().AppsV1().StatefulSets(k.namespace).Patch(sResource.GetName(), k8stypes.StrategicMergePatchType, data)
		return podChecker, errors.Trace(err)
	}

	deResource, err := k.getDeployment(deploymentName)
	if err != nil && !errors.IsNotFound(err) {
		return podChecker, errors.Trace(err)
	} else if err == nil {
		selector = deResource.Spec.Selector.MatchLabels
		if err := ensureJujuInitContainer(&deResource.Spec.Template.Spec, imagePath); err != nil {
			return podChecker, errors.Trace(err)
		}
		if data, err = json.Marshal(apps.Deployment{Spec: deResource.Spec}); err != nil {
			return podChecker, errors.Trace(err)
		}
		_, err = k.client().AppsV1().Deployments(k.namespace).Patch(deResource.GetName(), k8stypes.StrategicMergePatchType, data)
		return podChecker, errors.Trace(err)
	}

	daResource, err := k.getDaemonSet(deploymentName)
	if err != nil && !errors.IsNotFound(err) {
		return podChecker, errors.Trace(err)
	} else if err == nil {
		selector = daResource.Spec.Selector.MatchLabels
		if err := ensureJujuInitContainer(&daResource.Spec.Template.Spec, imagePath); err != nil {
			return podChecker, errors.Trace(err)
		}
		if data, err = json.Marshal(apps.DaemonSet{Spec: daResource.Spec}); err != nil {
			return podChecker, errors.Trace(err)
		}
		_, err = k.client().AppsV1().DaemonSets(k.namespace).Patch(daResource.GetName(), k8stypes.StrategicMergePatchType, data)
		return podChecker, errors.Trace(err)
	}
	// TODO(caas): check here should error or not(operator charm does not have any workload. So it's probably ok if there is no init containers to upgrade).
	return podChecker, nil
}
