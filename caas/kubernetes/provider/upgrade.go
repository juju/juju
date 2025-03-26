// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	core "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	appstyped "k8s.io/client-go/kubernetes/typed/apps/v1"

	"github.com/juju/juju/caas/kubernetes/provider/utils"
	k8sannotations "github.com/juju/juju/core/annotations"
	"github.com/juju/juju/internal/cloudconfig/podcfg"
	"github.com/juju/juju/internal/version"
)

func (k *kubernetesClient) Upgrade(ctx context.Context, agentTag string, vers version.Number) error {
	tag, err := names.ParseTag(agentTag)
	if err != nil {
		return errors.Annotate(err, "parsing agent tag to upgrade")
	}

	logger.Infof(context.TODO(), "handling upgrade request for tag %q to %s", tag, vers.String())

	switch tag.Kind() {
	case names.MachineTagKind:
	case names.ControllerAgentTagKind:
		return k.upgradeController(ctx, vers)
	case names.ModelTagKind:
		return k.upgradeModelOperator(ctx, vers)
	case names.UnitTagKind:
		// Sidecar charms don't have an upgrade step.
		// See PR 14974
		return nil
	}
	return errors.NotImplementedf("k8s upgrade for agent tag %q", agentTag)
}

func upgradeDeployment(
	ctx context.Context,
	name,
	imagePath string,
	vers version.Number,
	legacyLabels bool,
	broker appstyped.DeploymentInterface,
) error {
	de, err := broker.Get(ctx, name, meta.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return errors.NotFoundf(
			"deployment %q", name)
	} else if err != nil {
		return errors.Annotatef(err,
			"getting deployment to upgrade for name %q", name)
	}

	newInitContainers, err := upgradePodTemplateSpec(de.Spec.Template.Spec.InitContainers, imagePath, vers)
	if err != nil {
		return errors.Annotatef(err, "deployment %q", name)
	}
	de.Spec.Template.Spec.InitContainers = newInitContainers

	newContainers, err := upgradePodTemplateSpec(de.Spec.Template.Spec.Containers, imagePath, vers)
	if err != nil {
		return errors.Annotatef(err, "deployment %q", name)
	}
	de.Spec.Template.Spec.Containers = newContainers

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

	if _, err := broker.Update(ctx, de, meta.UpdateOptions{}); err != nil {
		return errors.Annotatef(err, "updating deployment %q to %s",
			name, vers)
	}
	return nil
}

func upgradeOperatorOrControllerStatefulSet(
	ctx context.Context,
	name string,
	imagePath string,
	vers version.Number,
	legacyLabels bool,
	broker appstyped.StatefulSetInterface,
) error {
	ss, err := broker.Get(ctx, name, meta.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return errors.NotFoundf(
			"statefulset %q", name)
	} else if err != nil {
		return errors.Annotatef(err,
			"getting statefulset to upgrade for name %q", name)
	}

	newInitContainers, err := upgradePodTemplateSpec(ss.Spec.Template.Spec.InitContainers, imagePath, vers)
	if err != nil {
		return errors.Annotatef(err, "statefulset %q", name)
	}
	ss.Spec.Template.Spec.InitContainers = newInitContainers

	newContainers, err := upgradePodTemplateSpec(ss.Spec.Template.Spec.Containers, imagePath, vers)
	if err != nil {
		return errors.Annotatef(err, "statefulset %q", name)
	}
	ss.Spec.Template.Spec.Containers = newContainers

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

	if _, err := broker.Update(ctx, ss, meta.UpdateOptions{}); err != nil {
		return errors.Annotatef(err, "updating statefulset %q to %s",
			name, vers)
	}
	return nil
}

func upgradePodTemplateSpec(
	containers []core.Container,
	imagePath string,
	vers version.Number,
) ([]core.Container, error) {
	jujudContainerIdx, found := findJujudContainer(containers)
	if !found {
		return containers, nil
	}

	if imagePath == "" {
		var err error
		imagePath, err = podcfg.RebuildOldOperatorImagePath(
			containers[jujudContainerIdx].Image, vers)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	containersCopy := append([]core.Container(nil), containers...)
	if containersCopy[jujudContainerIdx].Image != imagePath {
		containersCopy[jujudContainerIdx].Image = imagePath
	}
	return containersCopy, nil
}

func findJujudContainer(containers []core.Container) (int, bool) {
	for i, c := range containers {
		if podcfg.IsJujuOCIImage(c.Image) {
			return i, true
		}
	}
	return -1, false
}
