// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/version/v2"
	core "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	appstyped "k8s.io/client-go/kubernetes/typed/apps/v1"

	"github.com/juju/juju/caas/kubernetes/provider/utils"
	"github.com/juju/juju/cloudconfig/podcfg"
	k8sannotations "github.com/juju/juju/core/annotations"
)

func (k *kubernetesClient) Upgrade(agentTag string, vers version.Number) error {
	tag, err := names.ParseTag(agentTag)
	if err != nil {
		return errors.Annotate(err, "parsing agent tag to upgrade")
	}

	logger.Infof("handling upgrade request for tag %q", tag)

	switch tag.Kind() {
	case names.MachineTagKind:
	case names.ControllerAgentTagKind:
		return k.upgradeController(vers)
	case names.ApplicationTagKind:
		return k.upgradeOperator(tag, vers)
	case names.ModelTagKind:
		return k.upgradeModelOperator(tag, vers)
	}
	return errors.NotImplementedf("k8s upgrade for agent tag %q", agentTag)
}

func upgradeDeployment(
	name,
	imagePath string,
	vers version.Number,
	legacyLabels bool,
	broker appstyped.DeploymentInterface,
) error {
	de, err := broker.Get(context.TODO(), name, meta.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return errors.NotFoundf(
			"deployment %q", name)
	} else if err != nil {
		return errors.Annotatef(err,
			"getting deployment to upgrade for name %q", name)
	}

	updatedTemplateSpec, err := upgradePodTemplateSpec(&de.Spec.Template, imagePath, vers)
	if err != nil {
		return errors.Annotatef(err, "deployment %q", name)
	}
	de.Spec.Template = *updatedTemplateSpec

	// update juju-version annotation.
	// TODO(caas): consider how to upgrade to current annotations format safely.
	// just ensure juju-version to current version for now.
	de.SetAnnotations(
		k8sannotations.New(de.GetAnnotations()).
			Merge(utils.AnnotationsForVersion(vers.String(), legacyLabels)).ToMap(),
	)
	de.Spec.Template.SetAnnotations(
		k8sannotations.New(de.Spec.Template.GetAnnotations()).
			Merge(utils.AnnotationsForVersion(vers.String(), legacyLabels)).ToMap(),
	)

	if _, err := broker.Update(context.TODO(), de, meta.UpdateOptions{}); err != nil {
		return errors.Annotatef(err, "updating deployment %q to %s",
			name, vers)
	}
	return nil
}

func upgradeStatefulSet(
	name,
	imagePath string,
	vers version.Number,
	legacyLabels bool,
	broker appstyped.StatefulSetInterface,
) error {
	ss, err := broker.Get(context.TODO(), name, meta.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return errors.NotFoundf(
			"statefulset %q", name)
	} else if err != nil {
		return errors.Annotatef(err,
			"getting statefulset to upgrade for name %q", name)
	}

	updatedTemplateSpec, err := upgradePodTemplateSpec(&ss.Spec.Template, imagePath, vers)
	if err != nil {
		return errors.Annotatef(err, "statefulset %q", name)
	}
	ss.Spec.Template = *updatedTemplateSpec

	// update juju-version annotation.
	// TODO(caas): consider how to upgrade to current annotations format safely.
	// just ensure juju-version to current version for now.
	ss.SetAnnotations(
		k8sannotations.New(ss.GetAnnotations()).
			Merge(utils.AnnotationsForVersion(vers.String(), legacyLabels)).ToMap(),
	)
	ss.Spec.Template.SetAnnotations(
		k8sannotations.New(ss.Spec.Template.GetAnnotations()).
			Merge(utils.AnnotationsForVersion(vers.String(), legacyLabels)).ToMap(),
	)

	if _, err := broker.Update(context.TODO(), ss, meta.UpdateOptions{}); err != nil {
		return errors.Annotatef(err, "updating statefulset %q to %s",
			name, vers)
	}
	return nil
}

func upgradePodTemplateSpec(
	podTemplate *core.PodTemplateSpec,
	imagePath string,
	vers version.Number,
) (*core.PodTemplateSpec, error) {
	jujudContainerIdx, found := findJujudContainer(podTemplate.Spec.Containers)
	if !found {
		return nil, errors.NotFoundf("jujud container in pod spec")
	}

	if imagePath == "" {
		imagePath = podcfg.RebuildOldOperatorImagePath(
			podTemplate.Spec.Containers[jujudContainerIdx].Image, vers)
	}

	upgradedTemp := podTemplate.DeepCopy()

	if upgradedTemp.Spec.Containers[jujudContainerIdx].Image != imagePath {
		upgradedTemp.Spec.Containers[jujudContainerIdx].Image = imagePath
	}
	return upgradedTemp, nil
}

func findJujudContainer(containers []core.Container) (int, bool) {
	for i, c := range containers {
		if podcfg.IsJujuOCIImage(c.Image) {
			return i, true
		}
	}
	return -1, false
}
