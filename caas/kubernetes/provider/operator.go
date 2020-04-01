// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/juju/errors"
	"github.com/juju/version"
	"gopkg.in/juju/names.v3"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/informers"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/caas"
	k8sannotations "github.com/juju/juju/core/annotations"
	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
)

func operatorLabels(appName string) map[string]string {
	return map[string]string{labelOperator: appName}
}

func (k *kubernetesClient) deleteOperatorRBACResources(appName string) error {
	selector := labelSetToSelector(operatorLabels(appName))
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

func (k *kubernetesClient) ensureOperatorRBACResources(operatorName string, labels, annotations map[string]string) (sa *core.ServiceAccount, cleanUps []func(), err error) {
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
	r, rCleanups, err := k.ensureRole(&rbacv1.Role{
		ObjectMeta: v1.ObjectMeta{
			Name:        operatorName,
			Namespace:   k.namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Rules: []rbacv1.PolicyRule{
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
	_, rBCleanups, err := k.ensureRoleBinding(&rbacv1.RoleBinding{
		ObjectMeta: v1.ObjectMeta{
			Name:        operatorName,
			Namespace:   k.namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		RoleRef: rbacv1.RoleRef{
			Name: r.GetName(),
			Kind: "Role",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
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
	logger.Debugf("creating/updating %s operator", appName)

	operatorName := k.operatorName(appName)
	labels := operatorLabels(appName)
	annotations := resourceTagsToAnnotations(config.ResourceTags).
		Add(labelVersion, config.Version.String())

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
			Selector: map[string]string{labelOperator: appName},
			Type:     core.ServiceTypeClusterIP,
			Ports: []core.ServicePort{
				{Protocol: core.ProtocolTCP, Port: JujuRunServerSocketPort, TargetPort: intstr.FromInt(JujuRunServerSocketPort)}},
		},
	}
	if err := k.ensureK8sService(service); err != nil {
		return errors.Annotatef(err, "creating or updating service for %v operator", appName)
	}
	cleanups = append(cleanups, func() { _ = k.deleteService(operatorName) })
	services := k.client().CoreV1().Services(k.namespace)
	svc, err := services.Get(operatorName, v1.GetOptions{})
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
		cmCleanUp, err := k.ensureConfigMapLegacy(
			operatorConfigMap(appName, cmName, k.getConfigMapLabels(appName), annotations, config))
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
		config.OperatorImagePath,
		config.Version.String(),
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
				MatchLabels: labels,
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels:      labels,
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

func (k *kubernetesClient) operatorVolumeClaim(appName, operatorName string, storageParams *caas.CharmStorageParams) (*core.PersistentVolumeClaim, error) {
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
			Annotations: resourceTagsToAnnotations(storageParams.ResourceTags).ToMap()},
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
func (k *kubernetesClient) OperatorExists(appName string) (caas.OperatorState, error) {
	operatorName := k.operatorName(appName)
	exists, terminating, err := k.operatorStatefulSetExists(appName, operatorName)
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
		check func(appName string, operatorName string) (bool, bool, error)
	}{
		{"rbac", k.operatorRBACResourcesRemaining},
		{"config map", k.operatorConfigMapExists},
		{"configurations config map", k.operatorConfigurationsConfigMapExists},
		{"service", k.operatorServiceExists},
		{"secret", k.operatorSecretExists},
		{"deployment", k.operatorDeploymentExists},
		{"pods", k.operatorPodExists},
	}
	for _, c := range checks {
		exists, _, err := c.check(appName, operatorName)
		if err != nil {
			return caas.OperatorState{}, errors.Annotatef(err, "%s resource check", c.label)
		}
		if exists {
			// Terminating is always set to true regardless of whether the resource is failed as terminating
			// since it's the overall state that is reported back.
			logger.Debugf("operator %q exists and is terminating due to dangling %s resource(s)", c.label)
			return caas.OperatorState{Exists: true, Terminating: true}, nil
		}
	}
	return caas.OperatorState{}, nil
}

func (k *kubernetesClient) operatorStatefulSetExists(appName string, operatorName string) (exists bool, terminating bool, err error) {
	statefulSets := k.client().AppsV1().StatefulSets(k.namespace)
	operator, err := statefulSets.Get(operatorName, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return false, false, nil
	}
	if err != nil {
		return false, false, errors.Trace(err)
	}
	return true, operator.DeletionTimestamp != nil, nil
}

func (k *kubernetesClient) operatorRBACResourcesRemaining(appName string, operatorName string) (exists bool, terminating bool, err error) {
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

func (k *kubernetesClient) operatorConfigMapExists(appName string, operatorName string) (exists bool, terminating bool, err error) {
	configMaps := k.client().CoreV1().ConfigMaps(k.namespace)
	configMapName := operatorConfigMapName(operatorName)
	cm, err := configMaps.Get(configMapName, v1.GetOptions{})
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
	cm, err := configMaps.Get(configMapName, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return false, false, nil
	} else if err != nil {
		return false, false, errors.Trace(err)
	}
	return true, cm.DeletionTimestamp != nil, nil
}

func (k *kubernetesClient) operatorServiceExists(appName string, operatorName string) (exists bool, terminating bool, err error) {
	services := k.client().CoreV1().Services(k.namespace)
	s, err := services.Get(operatorName, v1.GetOptions{})
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

func (k *kubernetesClient) operatorDeploymentExists(appName string, operatorName string) (exists bool, terminating bool, err error) {
	deployments := k.client().AppsV1().Deployments(k.namespace)
	operator, err := deployments.Get(operatorName, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return false, false, nil
	} else if err != nil {
		return false, false, errors.Trace(err)
	}
	return true, operator.DeletionTimestamp != nil, nil
}

func (k *kubernetesClient) operatorPodExists(appName string, operatorName string) (exists bool, terminating bool, err error) {
	pods := k.client().CoreV1().Pods(k.namespace)
	podList, err := pods.List(v1.ListOptions{
		LabelSelector: operatorSelector(appName),
	})
	if err != nil {
		return false, false, errors.Trace(err)
	}
	return len(podList.Items) != 0, false, nil
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
	err = configMaps.Delete(configMapName, &v1.DeleteOptions{
		PropagationPolicy: &defaultPropagationPolicy,
	})
	if err != nil && !k8serrors.IsNotFound(err) {
		return errors.Trace(err)
	}

	// Delete artefacts created by k8s itself.
	configMapName = appName + "-configurations-config"
	if legacy {
		configMapName = "juju-" + configMapName
	}
	err = configMaps.Delete(configMapName, &v1.DeleteOptions{
		PropagationPolicy: &defaultPropagationPolicy,
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
	podsList, err := pods.List(v1.ListOptions{
		LabelSelector: operatorSelector(appName),
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
			err = pvs.Delete(volName, &v1.DeleteOptions{
				PropagationPolicy: &defaultPropagationPolicy,
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
			o.LabelSelector = operatorSelector(appName)
		}),
	)
	return k.newWatcher(factory.Core().V1().Pods().Informer(), appName, k.clock)
}

// Operator returns an Operator with current status and life details.
func (k *kubernetesClient) Operator(appName string) (*caas.Operator, error) {
	operatorName := k.operatorName(appName)
	statefulSets := k.client().AppsV1().StatefulSets(k.namespace)
	operator, err := statefulSets.Get(operatorName, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("operator %s", appName)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}

	pods := k.client().CoreV1().Pods(k.namespace)
	podsList, err := pods.List(v1.ListOptions{
		LabelSelector: operatorSelector(appName),
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(podsList.Items) == 0 {
		return nil, errors.NotFoundf("operator pod for application %q", appName)
	}

	opPod := podsList.Items[0]
	terminated := opPod.DeletionTimestamp != nil
	now := time.Now()
	statusMessage, opStatus, since, err := k.getPODStatus(opPod, now)
	if err != nil {
		return nil, errors.Trace(err)
	}

	cfg := caas.OperatorConfig{}
	if ver, ok := operator.Annotations[labelVersion]; ok {
		cfg.Version, err = version.Parse(ver)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	for _, container := range operator.Spec.Template.Spec.Containers {
		if container.Name == operatorContainerName {
			cfg.OperatorImagePath = container.Image
			break
		}
	}
	configMaps := k.client().CoreV1().ConfigMaps(k.namespace)
	configMap, err := configMaps.Get(operatorConfigMapName(operatorName), v1.GetOptions{})
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
			Name: podName,
			Annotations: podAnnotations(annotations.Copy()).
				Add(labelVersion, version).ToMap(),
			Labels: operatorLabels(appName),
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
					{Name: OperatorServiceIPEnvName, Value: operatorServiceIP},
					{
						Name: OperatorPodIPEnvName,
						ValueFrom: &core.EnvVarSource{
							FieldRef: &core.ObjectFieldSelector{
								FieldPath: "status.podIP",
							},
						},
					},
					{
						Name: OperatorNamespaceEnvName,
						ValueFrom: &core.EnvVarSource{
							FieldRef: &core.ObjectFieldSelector{
								FieldPath: "metadata.namespace",
							},
						},
					},
				},
				VolumeMounts: []core.VolumeMount{{
					Name:      configVolName,
					MountPath: filepath.Join(agent.Dir(agentPath, appTag), TemplateFileNameAgentConf),
					SubPath:   TemplateFileNameAgentConf,
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
							Path: TemplateFileNameAgentConf,
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
