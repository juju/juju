// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"encoding/json"
	"time"

	"github.com/juju/errors"
	"github.com/juju/version"
	apps "k8s.io/api/apps/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"

	"github.com/juju/juju/cloudconfig/podcfg"
	k8sannotations "github.com/juju/juju/core/annotations"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
)

const applicationUpgradeTimeoutSeconds = 30

// Upgrade sets the OCI image for the app's operator to the specified version.
func (k *kubernetesClient) Upgrade(appName string, vers version.Number) error {

	isController := appName == JujuControllerStackName
	if isController {
		// Upgrade the controller pod.
		_, err := k.upgradeControllerOrOperator(appName, vers)
		return errors.Trace(err)
	}

	// To upgrade an application, we upgrade the operator pod first then Juju init container in the workload pod.
	operatorImagePath, err := k.upgradeControllerOrOperator(k.operatorName(appName), vers)
	if err != nil {
		return errors.Trace(err)
	}
	if len(operatorImagePath) == 0 {
		// This should never happen.
		return errors.NotValidf("no resource is upgradable for application %q", appName)
	}

	var opWatcher watcher.NotifyWatcher
	opWatcher, err = k.WatchOperator(appName)
	if err != nil {
		return errors.Trace(err)
	}
	defer opWatcher.Kill()

	timeout := k.clock.After(applicationUpgradeTimeoutSeconds * time.Second)
	for {
		select {
		case <-timeout:
			return errors.Timeoutf("timeout while waiting for the upgraded operator of %q ready", appName)
		case _, ok := <-opWatcher.Changes():
			if !ok {
				opWatcher, err = k.WatchOperator(appName)
				if err != nil {
					return errors.Trace(err)
				}
			}
			operator, err := k.Operator(appName)
			if err != nil {
				return errors.Trace(err)
			}
			if operator.Status.Status == status.Running && operator.Config.OperatorImagePath == operatorImagePath {
				logger.Infof("operator has been upgraded to %q, now the init container for %q is starting to upgrade", operatorImagePath, appName)
				// Operator has been upgraded to target version and is stabilised.
				return errors.Trace(k.upgradeJujuInitContainer(appName, operatorImagePath))
			}
		}
	}
}

func (k *kubernetesClient) upgradeControllerOrOperator(name string, vers version.Number) (operatorImagePath string, err error) {
	logger.Debugf("Upgrading %q", name)

	api := k.client().AppsV1().StatefulSets(k.namespace)
	existingStatefulSet, err := api.Get(name, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		// We always expect that the controller or operator statefulset resource exist.
		// So this should never happen unless it was deleted accidentally.
		return operatorImagePath, errors.Annotatef(err, "getting the existing statefulset %q to upgrade", name)
	}
	if err != nil {
		return operatorImagePath, errors.Trace(err)
	}

	for i, c := range existingStatefulSet.Spec.Template.Spec.Containers {
		if !podcfg.IsJujuOCIImage(c.Image) {
			continue
		}
		operatorImagePath = podcfg.RebuildOldOperatorImagePath(c.Image, vers)
		if operatorImagePath != c.Image {
			logger.Infof("upgrading from %q to %q", c.Image, operatorImagePath)
		}
		c.Image = operatorImagePath
		existingStatefulSet.Spec.Template.Spec.Containers[i] = c
	}

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
	return operatorImagePath, errors.Trace(err)
}

func (k *kubernetesClient) upgradeJujuInitContainer(appName, operatorImagePath string) error {
	deploymentName := k.deploymentName(appName, true)

	var data []byte
	sResource, err := k.getStatefulSet(deploymentName)
	if err != nil && !errors.IsNotFound(err) {
		return errors.Trace(err)
	} else if err == nil {
		if err := ensureJujuInitContainer(&sResource.Spec.Template.Spec, operatorImagePath); err != nil {
			return errors.Trace(err)
		}
		if data, err = json.Marshal(apps.StatefulSet{Spec: sResource.Spec}); err != nil {
			return errors.Trace(err)
		}
		_, err = k.client().AppsV1().StatefulSets(k.namespace).Patch(sResource.GetName(), k8stypes.StrategicMergePatchType, data)
		return errors.Trace(err)
	}

	deResource, err := k.getDeployment(deploymentName)
	if err != nil && !errors.IsNotFound(err) {
		return errors.Trace(err)
	} else if err == nil {
		if err := ensureJujuInitContainer(&deResource.Spec.Template.Spec, operatorImagePath); err != nil {
			return errors.Trace(err)
		}
		if data, err = json.Marshal(apps.Deployment{Spec: deResource.Spec}); err != nil {
			return errors.Trace(err)
		}
		_, err = k.client().AppsV1().Deployments(k.namespace).Patch(deResource.GetName(), k8stypes.StrategicMergePatchType, data)
		return errors.Trace(err)
	}

	daResource, err := k.getDaemonSet(deploymentName)
	if err != nil && !errors.IsNotFound(err) {
		return errors.Trace(err)
	} else if err == nil {
		if err := ensureJujuInitContainer(&daResource.Spec.Template.Spec, operatorImagePath); err != nil {
			return errors.Trace(err)
		}
		if data, err = json.Marshal(apps.DaemonSet{Spec: daResource.Spec}); err != nil {
			return errors.Trace(err)
		}
		_, err = k.client().AppsV1().DaemonSets(k.namespace).Patch(daResource.GetName(), k8stypes.StrategicMergePatchType, data)
		return errors.Trace(err)
	}
	// TODO(caas): check here should error or not(operator charm does not have any workload. So it's probably ok if there is no init containers to upgrade).
	return nil
}
