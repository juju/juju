// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/juju/errors"
	"github.com/juju/featureflag"
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/caas/kubernetes/provider/resources"
	"github.com/juju/juju/caas/kubernetes/provider/storage"
	"github.com/juju/juju/caas/kubernetes/provider/utils"
	"github.com/juju/juju/cloudconfig/podcfg"
	k8sannotations "github.com/juju/juju/core/annotations"
	"github.com/juju/juju/core/paths"
	coreresources "github.com/juju/juju/core/resources"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/juju/osenv"
)

const (
	// OperatorAppTarget is the constant used to describe the operator's target
	// in kubernetes. This allows us to differentiate between different
	// operators that would possibly have the same labels otherwise.
	OperatorAppTarget = "application"
)

// GetOperatorPodName returns operator pod name for an application.
func GetOperatorPodName(
	podAPI typedcorev1.PodInterface,
	nsAPI typedcorev1.NamespaceInterface,
	appName,
	namespace,
	modelName,
	modelUUID,
	controllerUUID string,
) (string, error) {
	labelVersion, err := utils.MatchModelLabelVersion(namespace, modelName, modelUUID, controllerUUID, nsAPI)
	if err != nil {
		return "", errors.Annotatef(err, "determining legacy label status for model %s", modelName)
	}

	podsList, err := podAPI.List(context.TODO(), v1.ListOptions{
		LabelSelector: operatorSelector(appName, labelVersion),
	})
	if err != nil {
		return "", errors.Trace(err)
	}
	if len(podsList.Items) == 0 {
		return "", errors.NotFoundf("operator pod for application %q", appName)
	}
	return podsList.Items[0].GetName(), nil
}

func (k *kubernetesClient) deleteOperatorRBACResources(operatorName string) error {
	selector := utils.LabelsToSelector(
		utils.LabelsForOperator(operatorName, OperatorAppTarget, k.LabelVersion()),
	)

	if err := k.deleteRoleBindings(selector); err != nil {
		return errors.Trace(err)
	}
	if err := k.deleteRoles(selector); err != nil {
		return errors.Trace(err)
	}
	if err := k.deleteServiceAccounts(selector); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (k *kubernetesClient) ensureOperatorRBACResources(
	operatorName string,
	labels,
	annotations map[string]string,
) (sa *core.ServiceAccount, cleanUps []func(), err error) {
	defer func() {
		// ensure cleanup in reversed order.
		i := 0
		j := len(cleanUps) - 1
		for i < j {
			cleanUps[i], cleanUps[j] = cleanUps[j], cleanUps[i]
			i++
			j--
		}
	}()

	mountToken := true
	// ensure service account.
	saSpec := &core.ServiceAccount{
		ObjectMeta: v1.ObjectMeta{
			Name:        operatorName,
			Namespace:   k.namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		AutomountServiceAccountToken: &mountToken,
	}
	sa, saCleanups, err := k.ensureServiceAccount(saSpec)
	cleanUps = append(cleanUps, saCleanups...)
	if err != nil {
		return nil, cleanUps, errors.Trace(err)
	}
	// ensure role.
	r, rCleanups, err := k.ensureRole(&rbac.Role{
		ObjectMeta: v1.ObjectMeta{
			Name:        operatorName,
			Namespace:   k.namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Rules: []rbac.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods", "services"},
				Verbs: []string{
					"get",
					"list",
					"patch",
				},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"pods/exec"},
				Verbs: []string{
					"create",
				},
			},
		},
	})
	cleanUps = append(cleanUps, rCleanups...)
	if err != nil {
		return nil, cleanUps, errors.Trace(err)
	}
	// ensure rolebinding.
	_, rBCleanups, err := k.ensureRoleBinding(&rbac.RoleBinding{
		ObjectMeta: v1.ObjectMeta{
			Name:        operatorName,
			Namespace:   k.namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		RoleRef: rbac.RoleRef{
			Name: r.GetName(),
			Kind: "Role",
		},
		Subjects: []rbac.Subject{
			{
				Kind:      rbac.ServiceAccountKind,
				Name:      sa.GetName(),
				Namespace: sa.GetNamespace(),
			},
		},
	})
	cleanUps = append(cleanUps, rBCleanups...)
	if err != nil {
		return nil, cleanUps, errors.Trace(err)
	}
	return sa, cleanUps, nil
}

// EnsureOperator creates or updates an operator pod with the given application
// name, agent path, and operator config.
func (k *kubernetesClient) EnsureOperator(appName, agentPath string, config *caas.OperatorConfig) (err error) {
	if k.namespace == "" {
		return errNoNamespace
	}
	logger.Infof("creating/updating %s operator", appName)

	operatorName := k.operatorName(appName)

	selectorLabels := utils.LabelsForOperator(appName, OperatorAppTarget, k.LabelVersion())
	labels := selectorLabels

	if k.LabelVersion() != constants.LegacyLabelVersion {
		labels = utils.LabelsMerge(selectorLabels, utils.LabelsJuju)
	}

	annotations := utils.ResourceTagsToAnnotations(config.ResourceTags, k.LabelVersion()).
		Merge(utils.AnnotationsForVersion(config.Version.String(), k.LabelVersion()))

	var cleanups []func()
	defer func() {
		if err == nil {
			return
		}
		for _, f := range cleanups {
			f()
		}
	}()

	service := &core.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:        operatorName,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: core.ServiceSpec{
			Selector: selectorLabels,
			Type:     core.ServiceTypeClusterIP,
			Ports: []core.ServicePort{
				{
					Protocol:   core.ProtocolTCP,
					Port:       constants.JujuExecServerSocketPort,
					TargetPort: intstr.FromInt(constants.JujuExecServerSocketPort),
				},
			},
		},
	}
	if _, err := k.ensureK8sService(service); err != nil {
		return errors.Annotatef(err, "creating or updating service for %v operator", appName)
	}
	cleanups = append(cleanups, func() { _ = k.deleteService(operatorName) })
	services := k.client().CoreV1().Services(k.namespace)
	svc, err := services.Get(context.TODO(), operatorName, v1.GetOptions{})
	if err != nil {
		return errors.Trace(err)
	}

	sa, rbacCleanUps, err := k.ensureOperatorRBACResources(operatorName, labels, annotations)
	cleanups = append(cleanups, rbacCleanUps...)
	if err != nil {
		return errors.Trace(err)
	}

	cmName := operatorConfigMapName(operatorName)
	// TODO(caas) use secrets for storing agent password?
	if config.AgentConf == nil && config.OperatorInfo == nil {
		// We expect that the config map already exists,
		// so make sure it does.
		if _, err := k.getConfigMap(cmName); err != nil {
			return errors.Annotatef(err, "config map for %q should already exist", appName)
		}
	} else {
		configMapLabels := labels
		if k.LabelVersion() == constants.LegacyLabelVersion {
			configMapLabels = k.getConfigMapLabels(appName)
		}
		cmCleanUp, err := k.ensureConfigMapLegacy(
			operatorConfigMap(appName, cmName, configMapLabels, annotations, config))
		cleanups = append(cleanups, cmCleanUp)
		if err != nil {
			return errors.Annotate(err, "creating or updating ConfigMap")
		}
	}

	// Set up the parameters for creating charm storage (if required).
	pod, err := operatorPod(
		operatorName,
		appName,
		svc.Spec.ClusterIP,
		agentPath,
		config.ImageDetails,
		config.BaseImageDetails,
		selectorLabels,
		annotations.Copy(),
		sa.GetName(),
	)
	if err != nil {
		return errors.Annotate(err, "generating operator podspec")
	}
	// Take a copy for use with statefulset.
	podWithoutStorage := pod

	numPods := int32(1)
	operatorPvc, err := k.operatorVolumeClaim(appName, operatorName, config.CharmStorage)
	if err != nil {
		return errors.Trace(err)
	}
	statefulset := &apps.StatefulSet{
		ObjectMeta: v1.ObjectMeta{
			Name:        operatorName,
			Labels:      labels,
			Annotations: annotations.ToMap()},
		Spec: apps.StatefulSetSpec{
			Replicas: &numPods,
			Selector: &v1.LabelSelector{
				MatchLabels: selectorLabels,
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels:      selectorLabels,
					Annotations: pod.Annotations,
				},
			},
			PodManagementPolicy: apps.ParallelPodManagement,
		},
	}
	if operatorPvc != nil {
		logger.Debugf("using persistent volume claim for operator %s: %+v", appName, operatorPvc)
		statefulset.Spec.VolumeClaimTemplates = []core.PersistentVolumeClaim{*operatorPvc}
		pod.Spec.Containers[0].VolumeMounts = append(pod.Spec.Containers[0].VolumeMounts, core.VolumeMount{
			Name:      operatorPvc.Name,
			MountPath: agent.BaseDir(agentPath),
		})
	}
	statefulset.Spec.Template.Spec = pod.Spec
	err = k.ensureStatefulSet(statefulset, podWithoutStorage.Spec)
	return errors.Annotatef(err, "creating or updating %v operator StatefulSet", appName)
}

func operatorSelector(appName string, labelVersion constants.LabelVersion) string {
	return utils.LabelsToSelector(
		utils.LabelsForOperator(appName, OperatorAppTarget, labelVersion)).
		String()
}

func (k *kubernetesClient) operatorVolumeClaim(
	appName,
	operatorName string,
	storageParams *caas.CharmStorageParams,
) (*core.PersistentVolumeClaim, error) {
	// We may no longer need storage for charms, but if the charm has previously been deployed
	// with storage, we need to retain that.
	operatorVolumeClaim := "charm"
	if isLegacyName(operatorName) {
		operatorVolumeClaim = fmt.Sprintf("%v-operator-volume", appName)
	}
	if storageParams == nil {
		existingClaim, err := k.getPVC(operatorVolumeClaim)
		if errors.IsNotFound(err) {
			logger.Debugf("no existing volume claim for operator %s", operatorName)
			return nil, nil
		} else if err != nil {
			return nil, errors.Annotatef(err, "getting operator volume claim")
		}
		return existingClaim, nil
	}
	if storageParams.Provider != constants.StorageProviderType {
		return nil, errors.Errorf("expected charm storage provider %q, got %q", constants.StorageProviderType, storageParams.Provider)
	}

	// Charm needs storage so set it up.
	fsSize, err := resource.ParseQuantity(fmt.Sprintf("%dMi", storageParams.Size))
	if err != nil {
		return nil, errors.Annotatef(err, "invalid volume size %v", storageParams.Size)
	}

	params, err := storage.ParseVolumeParams(operatorVolumeClaim, fsSize, storageParams.Attributes)
	if err != nil {
		return nil, errors.Annotatef(err, "invalid storage configuration for %q operator", appName)
	}
	// We want operator storage to be deleted when the operator goes away.
	params.StorageConfig.ReclaimPolicy = core.PersistentVolumeReclaimDelete
	logger.Debugf("operator storage config %#v", *params.StorageConfig)

	// Attempt to get a persistent volume to store charm state etc.
	pvcSpec, err := k.maybeGetVolumeClaimSpec(*params)
	if err != nil {
		return nil, errors.Annotate(err, "finding operator volume claim")
	}

	return &core.PersistentVolumeClaim{
		ObjectMeta: v1.ObjectMeta{
			Name:        params.Name,
			Annotations: utils.ResourceTagsToAnnotations(storageParams.ResourceTags, k.LabelVersion()).ToMap()},
		Spec: *pvcSpec,
	}, nil
}

func (k *kubernetesClient) validateOperatorStorage() (string, error) {
	storageClass, _ := k.Config().AllAttrs()[constants.OperatorStorageKey].(string)
	if storageClass == "" {
		return "", errors.NewNotValid(nil, "config without operator-storage value not valid.\nRun juju add-k8s to reimport your k8s cluster.")
	}
	_, err := k.getStorageClass(storageClass)
	return storageClass, errors.Trace(err)
}

// OperatorExists indicates if the operator for the specified
// application exists, and whether the operator is terminating.
func (k *kubernetesClient) OperatorExists(appName string) (caas.DeploymentState, error) {
	operatorName := k.operatorName(appName)
	exists, terminating, err := k.operatorStatefulSetExists(operatorName)
	if err != nil {
		return caas.DeploymentState{}, errors.Trace(err)
	}
	if exists || terminating {
		if terminating {
			logger.Tracef("operator %q exists and is terminating", operatorName)
		} else {
			logger.Tracef("operator %q exists", operatorName)
		}
		return caas.DeploymentState{Exists: exists, Terminating: terminating}, nil
	}
	checks := []struct {
		label string
		check func(operatorName string) (bool, bool, error)
	}{
		{"rbac", k.operatorRBACResourcesRemaining},
		{"config map", k.operatorConfigMapExists},
		{"configurations config map", func(on string) (bool, bool, error) { return k.operatorConfigurationsConfigMapExists(appName, on) }},
		{"service", k.operatorServiceExists},
		{"secret", func(on string) (bool, bool, error) { return k.operatorSecretExists(appName, on) }},
		{"deployment", k.operatorDeploymentExists},
		{"pods", func(on string) (bool, bool, error) { return k.operatorPodExists(appName) }},
	}
	for _, c := range checks {
		exists, _, err := c.check(operatorName)
		if err != nil {
			return caas.DeploymentState{}, errors.Annotatef(err, "%s resource check", c.label)
		}
		if exists {
			// Terminating is always set to true regardless of whether the resource is failed as terminating
			// since it's the overall state that is reported back.
			logger.Debugf("operator %q exists and is terminating due to dangling %s resource(s)", operatorName, c.label)
			return caas.DeploymentState{Exists: true, Terminating: true}, nil
		}
	}
	return caas.DeploymentState{}, nil
}

func (k *kubernetesClient) operatorStatefulSetExists(operatorName string) (exists bool, terminating bool, err error) {
	if k.namespace == "" {
		return false, false, errNoNamespace
	}
	statefulSets := k.client().AppsV1().StatefulSets(k.namespace)
	operator, err := statefulSets.Get(context.TODO(), operatorName, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return false, false, nil
	}
	if err != nil {
		return false, false, errors.Trace(err)
	}
	return true, operator.DeletionTimestamp != nil, nil
}

func (k *kubernetesClient) operatorRBACResourcesRemaining(operatorName string) (exists bool, terminating bool, err error) {
	sa, err := k.getServiceAccount(operatorName)
	if errors.IsNotFound(err) {
		// continue
	} else if err != nil {
		return false, false, errors.Trace(err)
	} else {
		return true, sa.DeletionTimestamp != nil, nil
	}
	r, err := k.getRole(operatorName)
	if errors.IsNotFound(err) {
		// continue
	} else if err != nil {
		return false, false, errors.Trace(err)
	} else {
		return true, r.DeletionTimestamp != nil, nil
	}
	rb, err := k.getRoleBinding(operatorName)
	if errors.IsNotFound(err) {
		// continue
	} else if err != nil {
		return false, false, errors.Trace(err)
	} else {
		return true, rb.DeletionTimestamp != nil, nil
	}
	return false, false, nil
}

func (k *kubernetesClient) operatorConfigMapExists(operatorName string) (exists bool, terminating bool, err error) {
	if k.namespace == "" {
		return false, false, errNoNamespace
	}
	configMaps := k.client().CoreV1().ConfigMaps(k.namespace)
	configMapName := operatorConfigMapName(operatorName)
	cm, err := configMaps.Get(context.TODO(), configMapName, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return false, false, nil
	} else if err != nil {
		return false, false, errors.Trace(err)
	}
	return true, cm.DeletionTimestamp != nil, nil
}

func (k *kubernetesClient) operatorConfigurationsConfigMapExists(appName string, operatorName string) (exists bool, terminating bool, err error) {
	if k.namespace == "" {
		return false, false, errNoNamespace
	}
	legacy := isLegacyName(operatorName)
	configMaps := k.client().CoreV1().ConfigMaps(k.namespace)
	configMapName := appName + "-configurations-config"
	if legacy {
		configMapName = "juju-" + configMapName
	}
	cm, err := configMaps.Get(context.TODO(), configMapName, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return false, false, nil
	} else if err != nil {
		return false, false, errors.Trace(err)
	}
	return true, cm.DeletionTimestamp != nil, nil
}

func (k *kubernetesClient) operatorServiceExists(operatorName string) (exists bool, terminating bool, err error) {
	if k.namespace == "" {
		return false, false, errNoNamespace
	}
	services := k.client().CoreV1().Services(k.namespace)
	s, err := services.Get(context.TODO(), operatorName, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return false, false, nil
	} else if err != nil {
		return false, false, errors.Trace(err)
	}
	return true, s.DeletionTimestamp != nil, nil
}

func (k *kubernetesClient) operatorSecretExists(appName string, operatorName string) (exists bool, terminating bool, err error) {
	if k.namespace == "" {
		return false, false, errNoNamespace
	}
	legacy := isLegacyName(operatorName)
	deploymentName := appName
	if legacy {
		deploymentName = "juju-" + appName
	}
	secretName := appSecretName(deploymentName, operatorContainerName)
	s, err := k.getSecret(secretName)
	if errors.IsNotFound(err) {
		return false, false, nil
	} else if err != nil {
		return false, false, errors.Trace(err)
	}
	return true, s.DeletionTimestamp != nil, nil
}

func (k *kubernetesClient) operatorDeploymentExists(operatorName string) (exists bool, terminating bool, err error) {
	if k.namespace == "" {
		return false, false, errNoNamespace
	}
	deployments := k.client().AppsV1().Deployments(k.namespace)
	operator, err := deployments.Get(context.TODO(), operatorName, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return false, false, nil
	} else if err != nil {
		return false, false, errors.Trace(err)
	}
	return true, operator.DeletionTimestamp != nil, nil
}

func (k *kubernetesClient) operatorPodExists(appName string) (exists bool, terminating bool, err error) {
	if k.namespace == "" {
		return false, false, errNoNamespace
	}
	pods := k.client().CoreV1().Pods(k.namespace)
	podList, err := pods.List(context.TODO(), v1.ListOptions{
		LabelSelector: operatorSelector(appName, k.LabelVersion()),
	})
	if err != nil {
		return false, false, errors.Trace(err)
	}
	return len(podList.Items) != 0, false, nil
}

// DeleteOperator deletes the specified operator.
func (k *kubernetesClient) DeleteOperator(appName string) (err error) {
	if k.namespace == "" {
		return errNoNamespace
	}
	logger.Debugf("deleting %s operator", appName)

	operatorName := k.operatorName(appName)
	legacy := isLegacyName(operatorName)

	// First delete RBAC resources.
	if err = k.deleteOperatorRBACResources(appName); err != nil {
		return errors.Trace(err)
	}

	// Delete the config map(s).
	configMaps := k.client().CoreV1().ConfigMaps(k.namespace)
	configMapName := operatorConfigMapName(operatorName)
	err = configMaps.Delete(context.TODO(), configMapName, v1.DeleteOptions{
		PropagationPolicy: constants.DefaultPropagationPolicy(),
	})
	if err != nil && !k8serrors.IsNotFound(err) {
		return errors.Trace(err)
	}

	// Delete artefacts created by k8s itself.
	configMapName = appName + "-configurations-config"
	if legacy {
		configMapName = "juju-" + configMapName
	}
	err = configMaps.Delete(context.TODO(), configMapName, v1.DeleteOptions{
		PropagationPolicy: constants.DefaultPropagationPolicy(),
	})
	if err != nil && !k8serrors.IsNotFound(err) {
		return errors.Trace(err)
	}

	// Finally the operator itself.
	if err := k.deleteService(operatorName); err != nil {
		return errors.Trace(err)
	}
	if err := k.deleteStatefulSet(operatorName); err != nil {
		return errors.Trace(err)
	}
	pods := k.client().CoreV1().Pods(k.namespace)
	podsList, err := pods.List(context.TODO(), v1.ListOptions{
		LabelSelector: operatorSelector(appName, k.LabelVersion()),
	})
	if err != nil {
		return errors.Trace(err)
	}

	deploymentName := appName
	if legacy {
		deploymentName = "juju-" + appName
	}
	pvs := k.client().CoreV1().PersistentVolumes()
	for _, p := range podsList.Items {
		// Delete secrets.
		for _, c := range p.Spec.Containers {
			secretName := appSecretName(deploymentName, c.Name)
			if err := k.deleteSecret(secretName, ""); err != nil {
				return errors.Annotatef(err, "deleting %s secret for container %s", appName, c.Name)
			}
		}
		// Delete operator storage volumes.
		volumeNames, err := k.deleteVolumeClaims(appName, &p)
		if err != nil {
			return errors.Trace(err)
		}
		// Just in case the volume reclaim policy is retain, we force deletion
		// for operators as the volume is an inseparable part of the operator.
		for _, volName := range volumeNames {
			logger.Infof("deleting operator PV %s for application %s due to call to kubernetesClient.DeleteOperator", volName, appName)
			err = pvs.Delete(context.TODO(), volName, v1.DeleteOptions{
				PropagationPolicy: constants.DefaultPropagationPolicy(),
			})
			if err != nil && !k8serrors.IsNotFound(err) {
				return errors.Annotatef(err, "deleting operator persistent volume %v for %v",
					volName, appName)
			}
		}
	}
	return errors.Trace(k.deleteDeployment(operatorName))
}

// WatchOperator returns a watcher which notifies when there
// are changes to the operator of the specified application.
func (k *kubernetesClient) WatchOperator(appName string) (watcher.NotifyWatcher, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	factory := informers.NewSharedInformerFactoryWithOptions(k.client(), 0,
		informers.WithNamespace(k.namespace),
		informers.WithTweakListOptions(func(o *v1.ListOptions) {
			o.LabelSelector = operatorSelector(appName, k.LabelVersion())
		}),
	)
	return k.newWatcher(factory.Core().V1().Pods().Informer(), appName, k.clock)
}

// Operator returns an Operator with current status and life details.
func (k *kubernetesClient) Operator(appName string) (*caas.Operator, error) {
	return operator(k.client(),
		k.namespace,
		k.operatorName(appName),
		appName,
		k.LabelVersion(),
		k.clock.Now())
}

func operator(client kubernetes.Interface,
	namespace string,
	operatorName string,
	appName string,
	labelVersion constants.LabelVersion,
	now time.Time) (*caas.Operator, error) {
	if namespace == "" {
		return nil, errNoNamespace
	}
	statefulSets := client.AppsV1().StatefulSets(namespace)
	_, err := statefulSets.Get(context.TODO(), operatorName, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("operator %s", appName)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}

	pods := client.CoreV1().Pods(namespace)
	podsList, err := pods.List(context.TODO(), v1.ListOptions{
		LabelSelector: operatorSelector(appName, labelVersion),
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(podsList.Items) == 0 {
		return nil, errors.NotFoundf("operator pod for application %q", appName)
	}

	opPod := podsList.Items[0]

	eventGetter := func() ([]core.Event, error) {
		return resources.ListEventsForObject(context.TODO(), client.CoreV1().Events(namespace), opPod.Name, "Pod")
	}

	terminated := opPod.DeletionTimestamp != nil
	statusMessage, opStatus, since, err := resources.PodToJujuStatus(
		opPod, now, eventGetter)
	if err != nil {
		return nil, errors.Trace(err)
	}

	cfg := caas.OperatorConfig{}
	if ver, ok := opPod.Annotations[utils.AnnotationVersionKey(labelVersion)]; ok {
		cfg.Version, err = version.Parse(ver)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	for _, container := range opPod.Spec.InitContainers {
		if container.Name == operatorInitContainerName {
			cfg.ImageDetails = coreresources.DockerImageDetails{
				RegistryPath: container.Image,
			}
			break
		}
	}
	for _, container := range opPod.Spec.Containers {
		if container.Name == operatorContainerName {
			if podcfg.IsJujuOCIImage(container.Image) {
				// Old pod spec operators use the operator/controller image rather than a focal
				// charm-base image.
				cfg.ImageDetails = coreresources.DockerImageDetails{
					RegistryPath: container.Image,
				}
				break
			} else if podcfg.IsCharmBaseImage(container.Image) {
				cfg.BaseImageDetails = coreresources.DockerImageDetails{
					RegistryPath: container.Image,
				}
				break
			}
			return nil, errors.Errorf("unrecognized operator image path %q", container.Image)
		}
	}
	configMaps := client.CoreV1().ConfigMaps(namespace)
	configMap, err := configMaps.Get(context.TODO(), operatorConfigMapName(operatorName), v1.GetOptions{})
	if err != nil && !k8serrors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}
	if configMap != nil {
		cfg.ConfigMapGeneration = configMap.Generation
		if agentConf, ok := configMap.Data[operatorConfigMapAgentConfKey(appName)]; ok {
			cfg.AgentConf = []byte(agentConf)
		}
		if operatorInfo, ok := configMap.Data[caas.OperatorInfoFile]; ok {
			cfg.OperatorInfo = []byte(operatorInfo)
		}
	}

	return &caas.Operator{
		Id:    string(opPod.UID),
		Dying: terminated,
		Status: status.StatusInfo{
			Status:  opStatus,
			Message: statusMessage,
			Since:   &since,
		},
		Config: &cfg,
	}, nil
}

// operatorPod returns a *core.Pod for the operator pod
// of the specified application.
func operatorPod(
	podName,
	appName,
	operatorServiceIP,
	agentPath string,
	operatorImageDetails coreresources.DockerImageDetails,
	baseImageDetails coreresources.DockerImageDetails,
	selectorLabels map[string]string,
	annotations k8sannotations.Annotation,
	serviceAccountName string,
) (*core.Pod, error) {
	configMapName := operatorConfigMapName(podName)
	configVolName := configMapName

	if isLegacyName(podName) {
		configVolName += "-volume"
	}

	appTag := names.NewApplicationTag(appName)
	jujudCmd := fmt.Sprintf("exec $JUJU_TOOLS_DIR/jujud caasoperator --application-name=%s --debug", appName)
	jujuDataDir := paths.DataDir(paths.OSUnixLike)
	mountToken := true
	env := []core.EnvVar{
		{Name: "JUJU_APPLICATION", Value: appName},
		{Name: constants.OperatorServiceIPEnvName, Value: operatorServiceIP},
		{
			Name: constants.OperatorPodIPEnvName,
			ValueFrom: &core.EnvVarSource{
				FieldRef: &core.ObjectFieldSelector{
					FieldPath: "status.podIP",
				},
			},
		},
		{
			Name: constants.OperatorNamespaceEnvName,
			ValueFrom: &core.EnvVarSource{
				FieldRef: &core.ObjectFieldSelector{
					FieldPath: "metadata.namespace",
				},
			},
		},
	}
	if features := featureflag.AsEnvironmentValue(); features != "" {
		env = append(env, core.EnvVar{
			Name:  osenv.JujuFeatureFlagEnvKey,
			Value: features,
		})
	}
	pod := &core.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:        podName,
			Annotations: podAnnotations(annotations.Copy()).ToMap(),
			Labels:      selectorLabels,
		},
		Spec: core.PodSpec{
			ServiceAccountName:           serviceAccountName,
			AutomountServiceAccountToken: &mountToken,
			InitContainers: []core.Container{{
				Name:            operatorInitContainerName,
				ImagePullPolicy: core.PullIfNotPresent,
				Image:           operatorImageDetails.RegistryPath,
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
			}},
			Containers: []core.Container{{
				Name:            operatorContainerName,
				ImagePullPolicy: core.PullIfNotPresent,
				Image:           baseImageDetails.RegistryPath,
				WorkingDir:      jujuDataDir,
				Command: []string{
					"/bin/sh",
				},
				Args: []string{
					"-c",
					fmt.Sprintf(
						caas.JujudStartUpAltSh,
						jujuDataDir,
						"tools",
						"/opt/juju",
						jujudCmd,
					),
				},
				Env: env,
				VolumeMounts: []core.VolumeMount{{
					Name:      configVolName,
					MountPath: filepath.Join(agent.Dir(agentPath, appTag), constants.TemplateFileNameAgentConf),
					SubPath:   constants.TemplateFileNameAgentConf,
				}, {
					Name:      configVolName,
					MountPath: filepath.Join(agent.Dir(agentPath, appTag), caas.OperatorInfoFile),
					SubPath:   caas.OperatorInfoFile,
				}, {
					Name:      "juju-bins",
					MountPath: "/opt/juju",
				}},
			}},
			Volumes: []core.Volume{{
				Name: configVolName,
				VolumeSource: core.VolumeSource{
					ConfigMap: &core.ConfigMapVolumeSource{
						LocalObjectReference: core.LocalObjectReference{
							Name: configMapName,
						},
						Items: []core.KeyToPath{{
							Key:  operatorConfigMapAgentConfKey(appName),
							Path: constants.TemplateFileNameAgentConf,
						}, {
							Key:  caas.OperatorInfoFile,
							Path: caas.OperatorInfoFile,
						}},
					},
				},
			}, {
				Name: "juju-bins",
				VolumeSource: core.VolumeSource{
					EmptyDir: &core.EmptyDirVolumeSource{},
				},
			}},
		},
	}
	if operatorImageDetails.IsPrivate() {
		pod.Spec.ImagePullSecrets = []core.LocalObjectReference{
			{Name: constants.CAASImageRepoSecretName},
		}
	}
	return pod, nil
}

func operatorConfigMapAgentConfKey(appName string) string {
	return appName + "-agent.conf"
}

// operatorConfigMap returns a *core.ConfigMap for the operator pod
// of the specified application, with the specified configuration.
func operatorConfigMap(appName, name string, labels, annotations map[string]string, config *caas.OperatorConfig) *core.ConfigMap {
	return &core.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name:        name,
			Labels:      labels,
			Annotations: annotations,
			Generation:  config.ConfigMapGeneration,
		},
		Data: map[string]string{
			operatorConfigMapAgentConfKey(appName): string(config.AgentConf),
			caas.OperatorInfoFile:                  string(config.OperatorInfo),
		},
	}
}
