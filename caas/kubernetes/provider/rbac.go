// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"time"

	jujuclock "github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/retry"
	core "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"

	"github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/caas/kubernetes/provider/resources"
	k8sspecs "github.com/juju/juju/caas/kubernetes/provider/specs"
	"github.com/juju/juju/caas/kubernetes/provider/utils"
)

// AppNameForServiceAccount returns the juju application name associated with a
// given ServiceAccount. If app name cannot be obtained from the service
// account, errors.NotFound is returned.
func AppNameForServiceAccount(sa *core.ServiceAccount) (string, error) {
	if appName, ok := sa.Labels[constants.LabelKubernetesAppName]; ok {
		return appName, nil
	} else if appName, ok := sa.Labels[constants.LegacyLabelKubernetesAppName]; ok {
		return appName, nil
	}
	return "", errors.NotFoundf("application labels for service account %s", sa.Name)
}

// RBACLabels returns a set of labels that should be present for RBAC objects.
func RBACLabels(appName, modelName, modelUUID, controllerUUID string, global bool, labelVersion constants.LabelVersion) map[string]string {
	labels := utils.LabelsForApp(appName, labelVersion)
	if global {
		labels = utils.LabelsMerge(labels, utils.LabelsForModel(modelName, modelUUID, controllerUUID, labelVersion))
	}
	return labels
}

func (k *kubernetesClient) ensureServiceAccountForApp(
	appName string, annotations map[string]string, rbacDefinition k8sspecs.K8sRBACSpecConverter,
) (cleanups []func(), err error) {
	ctx := context.TODO()

	prefixNameSpace := func(name string) string {
		return fmt.Sprintf("%s-%s", k.namespace, name)
	}
	getBindingName := func(sa, cR k8sspecs.NameGetter) string {
		if sa.GetName() == cR.GetName() {
			return sa.GetName()
		}
		return fmt.Sprintf("%s-%s", sa.GetName(), cR.GetName())
	}
	getSAMeta := func(name string) v1.ObjectMeta {
		return v1.ObjectMeta{
			Name:        name,
			Namespace:   k.namespace,
			Labels:      RBACLabels(appName, k.ModelName(), k.ModelUUID(), k.ControllerUUID(), false, k.LabelVersion()),
			Annotations: annotations,
		}
	}
	getRoleClusterRoleName := func(roleName, serviceAccountName string, index int, global bool) (out string) {
		defer func() {
			if global {
				out = prefixNameSpace(out)
			}
		}()
		if roleName != "" {
			return roleName
		}
		out = serviceAccountName
		if index == 0 {
			return out
		}
		return fmt.Sprintf("%s%d", out, index)
	}
	getRoleMeta := func(roleName, serviceAccountName string, index int) v1.ObjectMeta {
		return v1.ObjectMeta{
			Name:        getRoleClusterRoleName(roleName, serviceAccountName, index, false),
			Namespace:   k.namespace,
			Labels:      RBACLabels(appName, k.ModelName(), k.ModelUUID(), k.ControllerUUID(), false, k.LabelVersion()),
			Annotations: annotations,
		}
	}
	getClusterRoleMeta := func(roleName, serviceAccountName string, index int) v1.ObjectMeta {
		return v1.ObjectMeta{
			Name:        getRoleClusterRoleName(roleName, serviceAccountName, index, true),
			Namespace:   k.namespace,
			Labels:      RBACLabels(appName, k.ModelName(), k.ModelUUID(), k.ControllerUUID(), true, k.LabelVersion()),
			Annotations: annotations,
		}
	}
	getBindingMeta := func(sa, role k8sspecs.NameGetter) v1.ObjectMeta {
		return v1.ObjectMeta{
			Name:        getBindingName(sa, role),
			Namespace:   k.namespace,
			Labels:      RBACLabels(appName, k.ModelName(), k.ModelUUID(), k.ControllerUUID(), false, k.LabelVersion()),
			Annotations: annotations,
		}
	}
	getClusterBindingMeta := func(sa, clusterRole k8sspecs.NameGetter) v1.ObjectMeta {
		return v1.ObjectMeta{
			Name:        getBindingName(sa, clusterRole),
			Namespace:   k.namespace,
			Labels:      RBACLabels(appName, k.ModelName(), k.ModelUUID(), k.ControllerUUID(), true, k.LabelVersion()),
			Annotations: annotations,
		}
	}

	serviceAccounts, roles, clusterroles, roleBindings, clusterRoleBindings := rbacDefinition.ToK8s(
		getSAMeta,
		getRoleMeta,
		getClusterRoleMeta,
		getBindingMeta,
		getClusterBindingMeta,
	)

	for _, spec := range serviceAccounts {
		_, sacleanups, err := k.ensureServiceAccount(&spec)
		cleanups = append(cleanups, sacleanups...)
		if err != nil {
			return cleanups, errors.Trace(err)
		}
	}

	for _, spec := range roles {
		_, rCleanups, err := k.ensureRole(&spec)
		cleanups = append(cleanups, rCleanups...)
		if err != nil {
			return cleanups, errors.Trace(err)
		}
	}
	for _, spec := range roleBindings {
		_, rbCleanups, err := k.ensureRoleBinding(&spec)
		cleanups = append(cleanups, rbCleanups...)
		if err != nil {
			return cleanups, errors.Trace(err)
		}
	}

	for _, spec := range clusterroles {
		cr := resources.ClusterRole{spec}
		cRCleanups, err := cr.Ensure(
			ctx,
			k.client(),
			resources.ClaimJujuOwnership,
		)
		cleanups = append(cleanups, cRCleanups...)
		if err != nil {
			return cleanups, errors.Trace(err)
		}
	}
	for _, spec := range clusterRoleBindings {
		clusterRoleBinding := resources.NewClusterRoleBinding(spec.Name, &spec)
		crbCleanups, err := clusterRoleBinding.Ensure(
			ctx,
			k.client(),
			resources.ClaimJujuOwnership,
		)
		cleanups = append(cleanups, crbCleanups...)
		if err != nil {
			return cleanups, errors.Trace(err)
		}
	}

	return cleanups, nil
}

func (k *kubernetesClient) deleteAllServiceAccountResources(appName string) error {
	selectorNamespaced := utils.LabelsToSelector(
		RBACLabels(appName, k.ModelName(), k.ModelUUID(), k.ControllerUUID(), false, k.LabelVersion()))
	selectorGlobal := utils.LabelsToSelector(
		RBACLabels(appName, k.ModelName(), k.ModelUUID(), k.ControllerUUID(), true, k.LabelVersion()))
	if err := k.deleteRoleBindings(selectorNamespaced); err != nil {
		return errors.Trace(err)
	}
	if err := k.deleteClusterRoleBindings(selectorGlobal); err != nil {
		return errors.Trace(err)
	}
	if err := k.deleteRoles(selectorNamespaced); err != nil {
		return errors.Trace(err)
	}
	if err := k.deleteClusterRoles(selectorGlobal); err != nil {
		return errors.Trace(err)
	}
	if err := k.deleteServiceAccounts(selectorNamespaced); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (k *kubernetesClient) createServiceAccount(sa *core.ServiceAccount) (*core.ServiceAccount, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	utils.PurifyResource(sa)
	out, err := k.client().CoreV1().ServiceAccounts(k.namespace).Create(context.TODO(), sa, v1.CreateOptions{})
	if k8serrors.IsAlreadyExists(err) {
		return nil, errors.AlreadyExistsf("service account %q", sa.GetName())
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) updateServiceAccount(sa *core.ServiceAccount) (*core.ServiceAccount, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	out, err := k.client().CoreV1().ServiceAccounts(k.namespace).Update(context.TODO(), sa, v1.UpdateOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("service account %q", sa.GetName())
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) ensureServiceAccount(sa *core.ServiceAccount) (out *core.ServiceAccount, cleanups []func(), err error) {
	out, err = k.createServiceAccount(sa)
	if err == nil {
		logger.Debugf("service account %q created", out.GetName())
		cleanups = append(cleanups, func() { _ = k.deleteServiceAccount(out.GetName(), out.GetUID()) })
		return out, cleanups, nil
	}
	if !errors.IsAlreadyExists(err) {
		return nil, cleanups, errors.Trace(err)
	}

	existing, err := k.getServiceAccount(sa.GetName())
	if err != nil {
		return nil, cleanups, errors.Trace(err)
	}

	var existingLabelVersion constants.LabelVersion
	switch name := sa.GetName(); name {
	case ExecRBACResourceName, modelOperatorName:
		existingLabelVersion, err = utils.MatchOperatorMetaLabelVersion(existing.ObjectMeta, modelOperatorName, OperatorModelTarget)
	default:
		if isOperatorName(name) {
			existingLabelVersion, err = utils.MatchOperatorMetaLabelVersion(existing.ObjectMeta, appNameFromOperator(name), OperatorAppTarget)
		} else {
			existingLabelVersion, err = utils.MatchApplicationMetaLabelVersion(existing.ObjectMeta, name)
		}
	}
	if err != nil {
		return nil, cleanups, errors.Annotatef(err, "ensuring ServiceAccount %q with labels %v ", sa.GetName(), existing.Labels)
	}
	if existingLabelVersion < k.labelVersion {
		logger.Warningf("updating label version for existing ServiceAccount %q from %d to %d ", sa.GetName(), existingLabelVersion, k.labelVersion)
	}

	out, err = k.updateServiceAccount(sa)
	logger.Debugf("updating service account %q", sa.GetName())
	return out, cleanups, errors.Trace(err)
}

func (k *kubernetesClient) getServiceAccount(name string) (*core.ServiceAccount, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	out, err := k.client().CoreV1().ServiceAccounts(k.namespace).Get(context.TODO(), name, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("service account %q", name)
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) deleteServiceAccount(name string, uid types.UID) error {
	if k.namespace == "" {
		return errNoNamespace
	}
	err := k.client().CoreV1().ServiceAccounts(k.namespace).Delete(context.TODO(), name, utils.NewPreconditionDeleteOptions(uid))
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) deleteServiceAccounts(selectors ...k8slabels.Selector) error {
	for _, selector := range selectors {
		err := k.client().CoreV1().ServiceAccounts(k.namespace).DeleteCollection(
			context.TODO(),
			v1.DeleteOptions{
				PropagationPolicy: constants.DefaultPropagationPolicy(),
			}, v1.ListOptions{
				LabelSelector: selector.String(),
			})
		if !k8serrors.IsNotFound(err) {
			return errors.Trace(err)
		}
	}
	return nil
}

func (k *kubernetesClient) createRole(role *rbacv1.Role) (*rbacv1.Role, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	utils.PurifyResource(role)
	out, err := k.client().RbacV1().Roles(k.namespace).Create(context.TODO(), role, v1.CreateOptions{})
	if k8serrors.IsAlreadyExists(err) {
		return nil, errors.AlreadyExistsf("role %q", role.GetName())
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) updateRole(role *rbacv1.Role) (*rbacv1.Role, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	out, err := k.client().RbacV1().Roles(k.namespace).Update(context.TODO(), role, v1.UpdateOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("role %q", role.GetName())
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) ensureRole(role *rbacv1.Role) (out *rbacv1.Role, cleanups []func(), err error) {
	out, err = k.createRole(role)
	if err == nil {
		logger.Debugf("role %q created", out.GetName())
		cleanups = append(cleanups, func() { _ = k.deleteRole(out.GetName(), out.GetUID()) })
		return out, cleanups, nil
	}
	if !errors.IsAlreadyExists(err) {
		return nil, cleanups, errors.Trace(err)
	}

	existing, err := k.getRole(role.GetName())
	if err != nil {
		return nil, cleanups, errors.Trace(err)
	}

	var existingLabelVersion constants.LabelVersion
	switch name := role.GetName(); name {
	case ExecRBACResourceName, modelOperatorName:
		existingLabelVersion, err = utils.MatchOperatorMetaLabelVersion(existing.ObjectMeta, modelOperatorName, OperatorModelTarget)
	default:
		if isOperatorName(name) {
			existingLabelVersion, err = utils.MatchOperatorMetaLabelVersion(existing.ObjectMeta, appNameFromOperator(name), OperatorAppTarget)
		} else {
			existingLabelVersion, err = utils.MatchApplicationMetaLabelVersion(existing.ObjectMeta, name)
		}
	}
	if err != nil {
		return nil, cleanups, errors.Annotatef(err, "ensuring Role %q with labels %v ", role.GetName(), existing.Labels)
	}
	if existingLabelVersion < k.labelVersion {
		logger.Warningf("updating label version for existing Role %q from %d to %d ", role.GetName(), existingLabelVersion, k.labelVersion)
	}

	out, err = k.updateRole(role)
	logger.Debugf("updating role %q", role.GetName())
	return out, cleanups, errors.Trace(err)
}

func (k *kubernetesClient) getRole(name string) (*rbacv1.Role, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	out, err := k.client().RbacV1().Roles(k.namespace).Get(context.TODO(), name, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("role %q", name)
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) deleteRole(name string, uid types.UID) error {
	if k.namespace == "" {
		return errNoNamespace
	}
	err := k.client().RbacV1().Roles(k.namespace).Delete(context.TODO(), name, utils.NewPreconditionDeleteOptions(uid))
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) deleteRoles(selectors ...k8slabels.Selector) error {
	if k.namespace == "" {
		return errNoNamespace
	}
	for _, selector := range selectors {
		err := k.client().RbacV1().Roles(k.namespace).DeleteCollection(
			context.TODO(),
			v1.DeleteOptions{
				PropagationPolicy: constants.DefaultPropagationPolicy(),
			}, v1.ListOptions{
				LabelSelector: selector.String(),
			})
		if !k8serrors.IsNotFound(err) {
			return errors.Trace(err)
		}
	}
	return nil
}

func (k *kubernetesClient) deleteClusterRoles(selector k8slabels.Selector) error {
	err := k.client().RbacV1().ClusterRoles().DeleteCollection(context.TODO(), v1.DeleteOptions{
		PropagationPolicy: constants.DefaultPropagationPolicy(),
	}, v1.ListOptions{
		LabelSelector: selector.String(),
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) listClusterRoles(selector k8slabels.Selector) ([]rbacv1.ClusterRole, error) {
	listOps := v1.ListOptions{
		LabelSelector: selector.String(),
	}
	cRList, err := k.client().RbacV1().ClusterRoles().List(context.TODO(), listOps)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(cRList.Items) == 0 {
		return nil, errors.NotFoundf("cluster role with selector %q", selector)
	}
	return cRList.Items, nil
}

func (k *kubernetesClient) createRoleBinding(rb *rbacv1.RoleBinding) (*rbacv1.RoleBinding, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	utils.PurifyResource(rb)
	out, err := k.client().RbacV1().RoleBindings(k.namespace).Create(context.TODO(), rb, v1.CreateOptions{})
	if k8serrors.IsAlreadyExists(err) {
		return nil, errors.AlreadyExistsf("role binding %q", rb.GetName())
	}
	return out, errors.Trace(err)
}

func ensureResourceDeleted(clock jujuclock.Clock, getResource func() error) error {
	notReadyYetErr := errors.New("resource is still being deleted")
	deletionChecker := func() error {
		err := getResource()
		if errors.IsNotFound(err) {
			return nil
		}
		if err == nil {
			return notReadyYetErr
		}
		return errors.Trace(err)
	}

	err := retry.Call(retry.CallArgs{
		Attempts: 10,
		Delay:    2 * time.Second,
		Clock:    clock,
		Func:     deletionChecker,
		IsFatalError: func(err error) bool {
			return err != nil && err != notReadyYetErr
		},
		NotifyFunc: func(error, int) {
			logger.Debugf("waiting for resource to be deleted")
		},
	})
	return errors.Trace(err)
}

func isRoleBindingEqual(a, b rbacv1.RoleBinding) bool {
	sortSubjects := func(s []rbacv1.Subject) {
		sort.Slice(s, func(i, j int) bool {
			return s[i].Name+s[i].Namespace+s[i].Kind > s[j].Name+s[j].Namespace+s[j].Kind
		})
	}
	sortSubjects(a.Subjects)
	sortSubjects(b.Subjects)

	// We don't compare labels.
	return reflect.DeepEqual(a.RoleRef, b.RoleRef) &&
		reflect.DeepEqual(a.Subjects, b.Subjects) &&
		reflect.DeepEqual(a.ObjectMeta.Annotations, b.ObjectMeta.Annotations)
}

func (k *kubernetesClient) ensureRoleBinding(rb *rbacv1.RoleBinding) (out *rbacv1.RoleBinding, cleanups []func(), err error) {
	isFirstDeploy := false
	// RoleRef is immutable, so delete first then re-create.
	existing, err := k.getRoleBinding(rb.GetName())
	if errors.Is(err, errors.NotFound) {
		isFirstDeploy = true
	} else if err != nil {
		return nil, cleanups, errors.Trace(err)
	}
	if existing != nil {
		if isRoleBindingEqual(*existing, *rb) {
			return existing, cleanups, nil
		}
		name := existing.GetName()
		UID := existing.GetUID()
		if err := k.deleteRoleBinding(name, UID); err != nil {
			return nil, cleanups, errors.Trace(err)
		}

		if err := ensureResourceDeleted(
			k.clock,
			func() error {
				_, err := k.getRoleBinding(name)
				return errors.Trace(err)
			},
		); err != nil {
			return nil, cleanups, errors.Trace(err)
		}
	}
	out, err = k.createRoleBinding(rb)
	if err != nil {
		return nil, cleanups, errors.Trace(err)
	}
	if isFirstDeploy {
		// only do cleanup for the first time, don't do this for existing deployments.
		cleanups = append(cleanups, func() { _ = k.deleteRoleBinding(out.GetName(), out.GetUID()) })
	}
	logger.Debugf("role binding %q created", rb.GetName())
	return out, cleanups, nil
}

func (k *kubernetesClient) getRoleBinding(name string) (*rbacv1.RoleBinding, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	out, err := k.client().RbacV1().RoleBindings(k.namespace).Get(context.TODO(), name, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("role binding %q", name)
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) deleteRoleBinding(name string, uid types.UID) error {
	if k.namespace == "" {
		return errNoNamespace
	}
	err := k.client().RbacV1().RoleBindings(k.namespace).Delete(context.TODO(), name, utils.NewPreconditionDeleteOptions(uid))
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) deleteRoleBindings(selectors ...k8slabels.Selector) error {
	if k.namespace == "" {
		return errNoNamespace
	}
	for _, selector := range selectors {
		err := k.client().RbacV1().RoleBindings(k.namespace).DeleteCollection(
			context.TODO(),
			v1.DeleteOptions{
				PropagationPolicy: constants.DefaultPropagationPolicy(),
			}, v1.ListOptions{
				LabelSelector: selector.String(),
			})
		if !k8serrors.IsNotFound(err) {
			return errors.Trace(err)
		}
	}
	return nil
}

func (k *kubernetesClient) deleteClusterRoleBindings(selector k8slabels.Selector) error {
	err := k.client().RbacV1().ClusterRoleBindings().DeleteCollection(context.TODO(), v1.DeleteOptions{
		PropagationPolicy: constants.DefaultPropagationPolicy(),
	}, v1.ListOptions{
		LabelSelector: selector.String(),
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) listClusterRoleBindings(selector k8slabels.Selector) ([]rbacv1.ClusterRoleBinding, error) {
	listOps := v1.ListOptions{
		LabelSelector: selector.String(),
	}
	cRBList, err := k.client().RbacV1().ClusterRoleBindings().List(context.TODO(), listOps)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(cRBList.Items) == 0 {
		return nil, errors.NotFoundf("cluster role binding with selector %q", selector)
	}
	return cRBList.Items, nil
}
