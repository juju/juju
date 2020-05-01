// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	// "bytes"
	"context"
	// "crypto/rand"
	"encoding/json"
	// "fmt"
	// "io"
	// "regexp"
	// "sort"
	// "strconv"
	// "strings"
	// "sync"
	"time"

	// jujuclock "github.com/juju/clock"
	// "github.com/juju/collections/set"
	"github.com/juju/errors"
	// "github.com/juju/loggo"
	// "github.com/juju/utils/arch"
	"github.com/juju/version"
	// "github.com/kr/pretty"
	// "gopkg.in/juju/names.v3"
	apps "k8s.io/api/apps/v1"
	// core "k8s.io/api/core/v1"
	// "k8s.io/api/extensions/v1beta1"
	// k8sstorage "k8s.io/api/storage/v1"
	// apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	// "k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	// k8slabels "k8s.io/apimachinery/pkg/labels"
	k8stypes "k8s.io/apimachinery/pkg/types"
	// "k8s.io/apimachinery/pkg/util/intstr"
	// k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	// "k8s.io/client-go/dynamic"
	// "k8s.io/client-go/informers"
	// "k8s.io/client-go/kubernetes"
	// "k8s.io/client-go/rest"

	// "github.com/juju/juju/caas"
	// k8sspecs "github.com/juju/juju/caas/kubernetes/provider/specs"
	// "github.com/juju/juju/caas/specs"
	"github.com/juju/juju/cloudconfig/podcfg"
	k8sannotations "github.com/juju/juju/core/annotations"
	// "github.com/juju/juju/core/application"
	// "github.com/juju/juju/core/constraints"
	// "github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/status"
	// "github.com/juju/juju/core/network"
	// "github.com/juju/juju/core/paths"
	// "github.com/juju/juju/core/status"
	// "github.com/juju/juju/core/watcher"
	// "github.com/juju/juju/environs"
	// "github.com/juju/juju/environs/config"
	// envcontext "github.com/juju/juju/environs/context"
	// "github.com/juju/juju/environs/tags"
	// "github.com/juju/juju/storage"
)

//
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

	ctx, cancel := context.WithTimeout(context.Background(), applicationUpgradeTimeoutSeconds*time.Second)
	defer cancel()
	watcher, err := k.WatchOperator(appName)
	if err != nil {
		return errors.Trace(err)
	}

	for {
		select {
		case <-ctx.Done():
			logger.Criticalf(`UpgradeUpgradeUpgrade -> %q`, errors.Annotatef(ctx.Err(), "waiting for the upgraded operator ready for %q", appName))
			// return errors.Annotatef(ctx.Err(), "waiting for the upgraded operator ready for %q", appName)
			break
		case <-watcher.Changes():
			operator, err := k.Operator(appName)
			if err != nil {
				return errors.Trace(err)
			}
			logger.Criticalf("operator.Status.Status -> %#v, operator.Status.Status == status.Running -> %v", operator.Status.Status, operator.Status.Status == status.Running)
			if operator.Status.Status == status.Running {
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
		logger.Criticalf("sleeping 30s")
		time.Sleep(30 * time.Second)
		logger.Criticalf("wake up after 30s")
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
	return nil
}
