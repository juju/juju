// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	appstyped "k8s.io/client-go/kubernetes/typed/apps/v1"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/caas/kubernetes/provider/utils"
	"github.com/juju/juju/cloudconfig/podcfg"
	k8sannotations "github.com/juju/juju/core/annotations"
	"github.com/juju/juju/core/paths"
)

func (k *kubernetesClient) Upgrade(agentTag string, vers version.Number) error {
	tag, err := names.ParseTag(agentTag)
	if err != nil {
		return errors.Annotate(err, "parsing agent tag to upgrade")
	}

	logger.Infof("handling upgrade request for tag %q to %s", tag, vers.String())

	switch tag.Kind() {
	case names.MachineTagKind:
	case names.ControllerAgentTagKind:
		return k.upgradeController(vers)
	case names.ApplicationTagKind:
		return k.upgradeOperator(tag, vers)
	case names.ModelTagKind:
		return k.upgradeModelOperator(tag, vers)
	case names.UnitTagKind:
		// Sidecar charms don't have an upgrade step.
		// See PR 14974
		return nil
	}
	return errors.NotImplementedf("k8s upgrade for agent tag %q", agentTag)
}

func upgradeDeployment(
	name,
	imagePath string,
	vers version.Number,
	labelVersion constants.LabelVersion,
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
			Merge(utils.AnnotationsForVersion(vers.String(), labelVersion)).ToMap(),
	)
	de.Spec.Template.SetAnnotations(
		k8sannotations.New(de.Spec.Template.GetAnnotations()).
			Merge(utils.AnnotationsForVersion(vers.String(), labelVersion)).ToMap(),
	)

	if _, err := broker.Update(context.TODO(), de, meta.UpdateOptions{}); err != nil {
		return errors.Annotatef(err, "updating deployment %q to %s",
			name, vers)
	}
	return nil
}

func upgradeOperatorOrControllerStatefulSet(
	appName string,
	name string,
	isOperator bool,
	imagePath string,
	baseImagePath string,
	vers version.Number,
	labelVersion constants.LabelVersion,
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

	if isOperator {
		err := patchOperatorToCharmBase(ss, appName, imagePath, baseImagePath)
		if err != nil {
			return errors.Annotatef(err, "unable to patch operator to charm base")
		}
	}

	// update juju-version annotation.
	// TODO(caas): consider how to upgrade to current annotations format safely.
	// just ensure juju-version to current version for now.
	ss.SetAnnotations(
		k8sannotations.New(ss.GetAnnotations()).
			Merge(utils.AnnotationsForVersion(vers.String(), labelVersion)).ToMap(),
	)
	ss.Spec.Template.SetAnnotations(
		k8sannotations.New(ss.Spec.Template.GetAnnotations()).
			Merge(utils.AnnotationsForVersion(vers.String(), labelVersion)).ToMap(),
	)

	if _, err := broker.Update(context.TODO(), ss, meta.UpdateOptions{}); err != nil {
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

func patchOperatorToCharmBase(ss *apps.StatefulSet, appName string, imagePath string, baseImagePath string) error {
	for _, container := range ss.Spec.Template.Spec.InitContainers {
		if container.Name == operatorInitContainerName {
			// Already patched.
			return nil
		}
	}

	ss.Spec.Template.Spec.InitContainers = append(ss.Spec.Template.Spec.InitContainers, core.Container{
		Name:            operatorInitContainerName,
		ImagePullPolicy: core.PullIfNotPresent,
		Image:           imagePath,
		Command: []string{
			"/bin/sh",
		},
		Args: []string{
			"-c",
			fmt.Sprintf(
				caas.JujudCopySh,
				"/opt/juju",
				"",
			),
		},
		VolumeMounts: []core.VolumeMount{{
			Name:      "juju-bins",
			MountPath: "/opt/juju",
		}},
	})

	ss.Spec.Template.Spec.Volumes = append(ss.Spec.Template.Spec.Volumes, core.Volume{
		Name: "juju-bins",
		VolumeSource: core.VolumeSource{
			EmptyDir: &core.EmptyDirVolumeSource{},
		},
	})

	for i, container := range ss.Spec.Template.Spec.Containers {
		if !podcfg.IsJujuOCIImage(container.Image) {
			continue
		}

		jujudCmd := fmt.Sprintf("exec $JUJU_TOOLS_DIR/jujud caasoperator --application-name=%s --debug", appName)
		jujuDataDir := paths.DataDir(paths.OSUnixLike)
		container.Image = baseImagePath
		container.Args = []string{
			"-c",
			fmt.Sprintf(
				caas.JujudStartUpAltSh,
				jujuDataDir,
				"tools",
				"/opt/juju",
				jujudCmd,
			),
		}
		container.VolumeMounts = append(container.VolumeMounts, core.VolumeMount{
			Name:      "juju-bins",
			MountPath: "/opt/juju",
		})

		ss.Spec.Template.Spec.Containers[i] = container
		break
	}

	return nil
}
