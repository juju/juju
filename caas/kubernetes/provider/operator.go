// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/version"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/informers"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider/constants"
	caasconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/caas/kubernetes/provider/utils"
	k8sannotations "github.com/juju/juju/core/annotations"
	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
)

const (
	// OperatorAppTarget is the constant used to describe the operator's target
	// in kubernetes. This allows us to differentiate between different
	// operators that would possible have the same labels otherwise
	OperatorAppTarget = "application"
)

// GetOperatorPodName returns operator pod name for an application.
func GetOperatorPodName(
	podAPI typedcorev1.PodInterface,
	nsAPI typedcorev1.NamespaceInterface,
	appName string,
	namespace string,
) (string, error) {
	legacyLabels, err := utils.IsLegacyModelLabels(namespace, nsAPI)
	if err != nil {
		return "", errors.Annotatef(err, "determining legacy label status for namespace %s", namespace)
	}

	podsList, err := podAPI.List(context.TODO(), v1.ListOptions{
		LabelSelector: operatorSelector(appName, legacyLabels),
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
		utils.LabelsForOperator(operatorName, OperatorAppTarget, k.IsLegacyLabels()),
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
				Resources: []string{"pods"},
				Verbs: []string{
					"get",
					"list",
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
func (k *kubernetesClient) EnsureOperator(name, agentPath string, config *caas.OperatorConfig) (err error) {
	logger.Debugf("creating/updating %s operator", name)

	operatorName := k.operatorName(name)

	selectorLabels := utils.LabelsForOperator(name, OperatorAppTarget, k.IsLegacyLabels())
	labels := selectorLabels

	if !k.IsLegacyLabels() {
		labels = utils.LabelsMerge(selectorLabels, utils.LabelsJuju)
	}

	annotations := utils.ResourceTagsToAnnotations(config.ResourceTags).
		Merge(utils.AnnotationsForVersion(config.Version.String(), k.IsLegacyLabels()))

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
					Port:       caasconstants.JujuRunServerSocketPort,
					TargetPort: intstr.FromInt(caasconstants.JujuRunServerSocketPort),
				},
			},
		},
	}
	if _, err := k.ensureK8sService(service); err != nil {
		return errors.Annotatef(err, "creating or updating service for %v operator", name)
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
			return errors.Annotatef(err, "config map for %q should already exist", name)
		}
	} else {
		configMapLabels := labels
		if k.IsLegacyLabels() {
			configMapLabels = k.getConfigMapLabels(name)
		}
		cmCleanUp, err := k.ensureConfigMapLegacy(
			operatorConfigMap(name, cmName, configMapLabels, annotations, config))
		cleanups = append(cleanups, cmCleanUp)
		if err != nil {
			return errors.Annotate(err, "creating or updating ConfigMap")
		}
	}

	// Set up the parameters for creating charm storage (if required).
	pod, err := operatorPod(
		operatorName,
		name,
		svc.Spec.ClusterIP,
		agentPath,
		config.OperatorImagePath,
		config.Version.String(),
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
	operatorPvc, err := k.operatorVolumeClaim(name, operatorName, config.CharmStorage)
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
		logger.Debugf("using persistent volume claim for operator %s: %+v", name, operatorPvc)
		statefulset.Spec.VolumeClaimTemplates = []core.PersistentVolumeClaim{*operatorPvc}
		pod.Spec.Containers[0].VolumeMounts = append(pod.Spec.Containers[0].VolumeMounts, core.VolumeMount{
			Name:      operatorPvc.Name,
			MountPath: agent.BaseDir(agentPath),
		})
	}
	statefulset.Spec.Template.Spec = pod.Spec
	err = k.ensureStatefulSet(statefulset, podWithoutStorage.Spec)
	return errors.Annotatef(err, "creating or updating %v operator StatefulSet", name)
}

func operatorSelector(appName string, legacyLabels bool) string {
	return utils.LabelSetToSelector(
		utils.LabelsForOperator(appName, OperatorAppTarget, legacyLabels)).
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
	if storageParams.Provider != K8s_ProviderType {
		return nil, errors.Errorf("expected charm storage provider %q, got %q", K8s_ProviderType, storageParams.Provider)
	}

	// Charm needs storage so set it up.
	fsSize, err := resource.ParseQuantity(fmt.Sprintf("%dMi", storageParams.Size))
	if err != nil {
		return nil, errors.Annotatef(err, "invalid volume size %v", storageParams.Size)
	}

	params, err := newVolumeParams(operatorVolumeClaim, fsSize, storageParams.Attributes)
	if err != nil {
		return nil, errors.Annotatef(err, "invalid storage configuration for %q operator", appName)
	}
	// We want operator storage to be deleted when the operator goes away.
	params.storageConfig.reclaimPolicy = core.PersistentVolumeReclaimDelete
	logger.Debugf("operator storage config %#v", *params.storageConfig)

	// Attempt to get a persistent volume to store charm state etc.
	pvcSpec, err := k.maybeGetVolumeClaimSpec(params)
	if err != nil {
		return nil, errors.Annotate(err, "finding operator volume claim")
	}

	return &core.PersistentVolumeClaim{
		ObjectMeta: v1.ObjectMeta{
			Name:        params.pvcName,
			Annotations: utils.ResourceTagsToAnnotations(storageParams.ResourceTags).ToMap()},
		Spec: *pvcSpec,
	}, nil
}

func (k *kubernetesClient) validateOperatorStorage() (string, error) {
	storageClass, _ := k.Config().AllAttrs()[OperatorStorageKey].(string)
	if storageClass == "" {
		return "", errors.NewNotValid(nil, "config without operator-storage value not valid.\nRun juju add-k8s to reimport your k8s cluster.")
	}
	_, err := k.getStorageClass(storageClass)
	return storageClass, errors.Trace(err)
}

// OperatorExists indicates if the operator for the specified
// application exists, and whether the operator is terminating.
func (k *kubernetesClient) OperatorExists(name string) (caas.OperatorState, error) {
	operatorName := k.operatorName(name)
	exists, terminating, err := k.operatorStatefulSetExists(operatorName)
	if err != nil {
		return caas.OperatorState{}, errors.Trace(err)
	}
	if exists || terminating {
		if terminating {
			logger.Tracef("operator %q exists and is terminating")
		} else {
			logger.Tracef("operator %q exists")
		}
		return caas.OperatorState{Exists: exists, Terminating: terminating}, nil
	}
	checks := []struct {
		label string
		check func(operatorName string) (bool, bool, error)
	}{
		{"rbac", k.operatorRBACResourcesRemaining},
		{"config map", k.operatorConfigMapExists},
		{"configurations config map", func(on string) (bool, bool, error) { return k.operatorConfigurationsConfigMapExists(name, on) }},
		{"service", k.operatorServiceExists},
		{"secret", func(on string) (bool, bool, error) { return k.operatorSecretExists(name, on) }},
		{"deployment", k.operatorDeploymentExists},
	}
	for _, c := range checks {
		exists, _, err := c.check(operatorName)
		if err != nil {
			return caas.OperatorState{}, errors.Annotatef(err, "%s resource check", c.label)
		}
		if exists {
			// Terminating is always set to true regardless of whether the resource is failed as terminating
			// since it's the overall state that is reported back.
			logger.Debugf("operator %q exists and is terminating due to dangling %s resource(s)", operatorName, c.label)
			return caas.OperatorState{Exists: true, Terminating: true}, nil
		}
	}
	return caas.OperatorState{}, nil
}

func (k *kubernetesClient) operatorStatefulSetExists(operatorName string) (exists bool, terminating bool, err error) {
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
	deployments := k.client().AppsV1().Deployments(k.namespace)
	operator, err := deployments.Get(context.TODO(), operatorName, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return false, false, nil
	} else if err != nil {
		return false, false, errors.Trace(err)
	}
	return true, operator.DeletionTimestamp != nil, nil
}

// DeleteOperator deletes the specified operator.
func (k *kubernetesClient) DeleteOperator(appName string) (err error) {
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
		PropagationPolicy: &constants.DefaultPropagationPolicy,
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
		PropagationPolicy: &constants.DefaultPropagationPolicy,
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
		LabelSelector: operatorSelector(appName, k.IsLegacyLabels()),
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
			err = pvs.Delete(context.TODO(), volName, v1.DeleteOptions{
				PropagationPolicy: &constants.DefaultPropagationPolicy,
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
	factory := informers.NewSharedInformerFactoryWithOptions(k.client(), 0,
		informers.WithNamespace(k.namespace),
		informers.WithTweakListOptions(func(o *v1.ListOptions) {
			o.LabelSelector = operatorSelector(appName, k.IsLegacyLabels())
		}),
	)
	return k.newWatcher(factory.Core().V1().Pods().Informer(), appName, k.clock)
}

// Operator returns an Operator with current status and life details.
func (k *kubernetesClient) Operator(appName string) (*caas.Operator, error) {
	operatorName := k.operatorName(appName)
	statefulSets := k.client().AppsV1().StatefulSets(k.namespace)
	_, err := statefulSets.Get(context.TODO(), operatorName, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("operator %s", appName)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}

	pods := k.client().CoreV1().Pods(k.namespace)
	podsList, err := pods.List(context.TODO(), v1.ListOptions{
		LabelSelector: operatorSelector(appName, k.IsLegacyLabels()),
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(podsList.Items) == 0 {
		return nil, errors.NotFoundf("operator pod for application %q", appName)
	}

	opPod := podsList.Items[0]
	terminated := opPod.DeletionTimestamp != nil
	statusMessage, opStatus, since, err := k.getPODStatus(opPod, k.clock.Now())
	if err != nil {
		return nil, errors.Trace(err)
	}

	cfg := caas.OperatorConfig{}
	if ver, ok := opPod.Annotations[utils.AnnotationVersionKey(k.IsLegacyLabels())]; ok {
		cfg.Version, err = version.Parse(ver)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	for _, container := range opPod.Spec.Containers {
		if container.Name == operatorContainerName {
			cfg.OperatorImagePath = container.Image
			break
		}
	}
	configMaps := k.client().CoreV1().ConfigMaps(k.namespace)
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
	agentPath,
	operatorImagePath,
	version string,
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
	jujudCmd := fmt.Sprintf("$JUJU_TOOLS_DIR/jujud caasoperator --application-name=%s --debug", appName)
	jujuDataDir, err := paths.DataDir("kubernetes")
	if err != nil {
		return nil, errors.Trace(err)
	}
	mountToken := true
	return &core.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:        podName,
			Annotations: podAnnotations(annotations.Copy()).ToMap(),
			Labels:      selectorLabels,
		},
		Spec: core.PodSpec{
			ServiceAccountName:           serviceAccountName,
			AutomountServiceAccountToken: &mountToken,
			Containers: []core.Container{{
				Name:            operatorContainerName,
				ImagePullPolicy: core.PullIfNotPresent,
				Image:           operatorImagePath,
				WorkingDir:      jujuDataDir,
				Command: []string{
					"/bin/sh",
				},
				Args: []string{
					"-c",
					fmt.Sprintf(
						caas.JujudStartUpSh,
						jujuDataDir,
						"tools",
						jujudCmd,
					),
				},
				Env: []core.EnvVar{
					{Name: "JUJU_APPLICATION", Value: appName},
					{Name: caasconstants.OperatorServiceIPEnvName, Value: operatorServiceIP},
					{
						Name: caasconstants.OperatorPodIPEnvName,
						ValueFrom: &core.EnvVarSource{
							FieldRef: &core.ObjectFieldSelector{
								FieldPath: "status.podIP",
							},
						},
					},
					{
						Name: caasconstants.OperatorNamespaceEnvName,
						ValueFrom: &core.EnvVarSource{
							FieldRef: &core.ObjectFieldSelector{
								FieldPath: "metadata.namespace",
							},
						},
					},
				},
				VolumeMounts: []core.VolumeMount{{
					Name:      configVolName,
					MountPath: filepath.Join(agent.Dir(agentPath, appTag), constants.TemplateFileNameAgentConf),
					SubPath:   constants.TemplateFileNameAgentConf,
				}, {
					Name:      configVolName,
					MountPath: filepath.Join(agent.Dir(agentPath, appTag), caas.OperatorInfoFile),
					SubPath:   caas.OperatorInfoFile,
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
			}},
		},
	}, nil
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
