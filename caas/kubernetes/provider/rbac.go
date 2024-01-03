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
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/retry"
	authenticationv1 "k8s.io/api/authentication/v1"
	core "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"

	"github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/caas/kubernetes/provider/resources"
	"github.com/juju/juju/caas/kubernetes/provider/utils"
	environsbootstrap "github.com/juju/juju/environs/bootstrap"
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
func RBACLabels(appName, model string, global, legacy bool) map[string]string {
	labels := utils.LabelsForApp(appName, legacy)
	if global {
		labels = utils.LabelsMerge(labels, utils.LabelsForModel(model, legacy))
	}
	return labels
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
	if !errors.Is(err, errors.AlreadyExists) {
		return nil, cleanups, errors.Trace(err)
	}
	_, err = k.listServiceAccount(sa.GetLabels())
	if err != nil {
		if errors.Is(err, errors.NotFound) {
			// sa.Name is already used for an existing service account.
			return nil, cleanups, errors.AlreadyExistsf("service account %q", sa.GetName())
		}
		return nil, cleanups, errors.Trace(err)
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

func (k *kubernetesClient) listServiceAccount(labels map[string]string) ([]core.ServiceAccount, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	listOps := v1.ListOptions{
		LabelSelector: utils.LabelsToSelector(labels).String(),
	}
	saList, err := k.client().CoreV1().ServiceAccounts(k.namespace).List(context.TODO(), listOps)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(saList.Items) == 0 {
		return nil, errors.NotFoundf("service account with labels %v", labels)
	}
	return saList.Items, nil
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
	if !errors.Is(err, errors.AlreadyExists) {
		return nil, cleanups, errors.Trace(err)
	}
	_, err = k.listRoles(utils.LabelsToSelector(role.GetLabels()))
	if err != nil {
		if errors.Is(err, errors.NotFound) {
			// role.Name is already used for an existing role.
			return nil, cleanups, errors.AlreadyExistsf("role %q", role.GetName())
		}
		return nil, cleanups, errors.Trace(err)
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

func (k *kubernetesClient) listRoles(selector k8slabels.Selector) ([]rbacv1.Role, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	listOps := v1.ListOptions{
		LabelSelector: selector.String(),
	}
	rList, err := k.client().RbacV1().Roles(k.namespace).List(context.TODO(), listOps)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(rList.Items) == 0 {
		return nil, errors.NotFoundf("role with selector %q", selector)
	}
	return rList.Items, nil
}

func (k *kubernetesClient) createClusterRole(clusterrole *rbacv1.ClusterRole) (*rbacv1.ClusterRole, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	utils.PurifyResource(clusterrole)
	out, err := k.client().RbacV1().ClusterRoles().Create(context.TODO(), clusterrole, v1.CreateOptions{})
	if k8serrors.IsAlreadyExists(err) {
		return nil, errors.AlreadyExistsf("clusterrole %q", clusterrole.GetName())
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) updateClusterRole(clusterrole *rbacv1.ClusterRole) (*rbacv1.ClusterRole, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	out, err := k.client().RbacV1().ClusterRoles().Update(context.TODO(), clusterrole, v1.UpdateOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("clusterrole %q", clusterrole.GetName())
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) getClusterRole(name string) (*rbacv1.ClusterRole, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	out, err := k.client().RbacV1().ClusterRoles().Get(context.TODO(), name, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("clusterrole %q", name)
	}
	return out, errors.Trace(err)
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
		if errors.Is(err, errors.NotFound) {
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

// TODO: make this configurable.
var expiresInSeconds = int64(60 * 10)

// EnsureSecretAccessToken ensures the RBAC resources created and updated for the provided resource name.
func (k *kubernetesClient) EnsureSecretAccessToken(tag names.Tag, owned, read, removed []string) (string, error) {
	appName := tag.Id()
	if tag.Kind() == names.UnitTagKind {
		appName, _ = names.UnitApplication(tag.Id())
	}
	labels := utils.LabelsForApp(appName, k.IsLegacyLabels())

	objMeta := v1.ObjectMeta{
		Name:      tag.String(),
		Labels:    labels,
		Namespace: k.namespace,
	}

	sa := &core.ServiceAccount{
		ObjectMeta:                   objMeta,
		AutomountServiceAccountToken: boolPtr(true),
	}
	_, _, err := k.ensureServiceAccount(sa)
	if err != nil {
		return "", errors.Annotatef(err, "cannot ensure service account %q", sa.GetName())
	}

	if err := k.ensureBindingForSecretAccessToken(sa, objMeta, owned, read, removed); err != nil {
		return "", errors.Trace(err)
	}

	treq := &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{
			ExpirationSeconds: &expiresInSeconds,
		},
	}
	tr, err := k.client().CoreV1().ServiceAccounts(k.namespace).CreateToken(context.TODO(), sa.Name, treq, v1.CreateOptions{})
	if err != nil {
		return "", errors.Annotatef(err, "cannot request a token for %q", sa.Name)
	}
	return tr.Status.Token, nil
}

func (k *kubernetesClient) ensureClusterBindingForSecretAccessToken(sa *core.ServiceAccount, objMeta v1.ObjectMeta, owned, read, removed []string) error {
	objMeta.Name = fmt.Sprintf("%s-%s", k.namespace, objMeta.Name)
	clusterRole, err := k.getClusterRole(objMeta.Name)
	if err != nil && !errors.Is(err, errors.NotFound) {
		return errors.Trace(err)
	}
	if errors.Is(err, errors.NotFound) {
		clusterRole, err = k.createClusterRole(
			&rbacv1.ClusterRole{
				ObjectMeta: objMeta,
				Rules:      rulesForSecretAccess(k.namespace, true, nil, owned, read, removed),
			},
		)
	} else {
		clusterRole.Rules = rulesForSecretAccess(k.namespace, true, clusterRole.Rules, owned, read, removed)
		clusterRole, err = k.updateClusterRole(clusterRole)
	}
	if err != nil {
		return errors.Trace(err)
	}
	bindingSpec := &rbacv1.ClusterRoleBinding{
		ObjectMeta: objMeta,
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     clusterRole.Name,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      sa.Name,
				Namespace: sa.Namespace,
			},
		},
	}
	clusterRoleBinding := resources.NewClusterRoleBinding(bindingSpec.Name, bindingSpec)
	_, err = clusterRoleBinding.Ensure(context.TODO(), k.client(), resources.ClaimJujuOwnership)
	if err != nil {
		return errors.Trace(err)
	}

	// Ensure role binding exists before we return to avoid a race where a client
	// attempts to perform an operation before the role is allowed.
	return errors.Trace(retry.Call(retry.CallArgs{
		Func: func() error {
			api := k.client().RbacV1().ClusterRoleBindings()
			_, err := api.Get(context.TODO(), clusterRoleBinding.Name, v1.GetOptions{ResourceVersion: clusterRoleBinding.ResourceVersion})
			if k8serrors.IsNotFound(err) {
				return errors.NewNotFound(err, "k8s")
			}
			return errors.Trace(err)
		},
		IsFatalError: func(err error) bool {
			return !errors.Is(err, errors.NotFound)
		},
		Clock:    jujuclock.WallClock,
		Attempts: 5,
		Delay:    time.Second,
	}))
}

func (k *kubernetesClient) ensureBindingForSecretAccessToken(sa *core.ServiceAccount, objMeta v1.ObjectMeta, owned, read, removed []string) error {
	if k.Config().Name() == environsbootstrap.ControllerModelName {
		return k.ensureClusterBindingForSecretAccessToken(sa, objMeta, owned, read, removed)
	}

	role, err := k.getRole(objMeta.Name)
	if err != nil && !errors.Is(err, errors.NotFound) {
		return errors.Trace(err)
	}
	if errors.Is(err, errors.NotFound) {
		role, err = k.createRole(
			&rbacv1.Role{
				ObjectMeta: objMeta,
				Rules:      rulesForSecretAccess(k.namespace, false, nil, owned, read, removed),
			},
		)
	} else {
		role.Rules = rulesForSecretAccess(k.namespace, false, role.Rules, owned, read, removed)
		role, err = k.updateRole(role)
	}
	if err != nil {
		return errors.Trace(err)
	}
	bindingSpec := &rbacv1.RoleBinding{
		ObjectMeta: objMeta,
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     role.Name,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      sa.Name,
				Namespace: sa.Namespace,
			},
		},
	}
	roleBinding := resources.NewRoleBinding(bindingSpec.Name, bindingSpec.Namespace, bindingSpec)
	err = roleBinding.Apply(context.TODO(), k.client())
	if err != nil {
		return errors.Trace(err)
	}

	// Ensure role binding exists before we return to avoid a race where a client
	// attempts to perform an operation before the role is allowed.
	return errors.Trace(retry.Call(retry.CallArgs{
		Func: func() error {
			api := k.client().RbacV1().RoleBindings(k.namespace)
			_, err := api.Get(context.TODO(), roleBinding.Name, v1.GetOptions{ResourceVersion: roleBinding.ResourceVersion})
			if k8serrors.IsNotFound(err) {
				return errors.NewNotFound(err, "k8s")
			}
			return errors.Trace(err)
		},
		IsFatalError: func(err error) bool {
			return !errors.Is(err, errors.NotFound)
		},
		Clock:    jujuclock.WallClock,
		Attempts: 5,
		Delay:    time.Second,
	}))
}

func cleanRules(existing []rbacv1.PolicyRule, shouldRemove func(string) bool) []rbacv1.PolicyRule {
	if len(existing) == 0 {
		return nil
	}

	i := 0
	for _, r := range existing {
		if len(r.ResourceNames) == 1 && shouldRemove(r.ResourceNames[0]) {
			continue
		}
		existing[i] = r
		i++
	}
	return existing[:i]
}

func rulesForSecretAccess(
	namespace string, isControllerModel bool,
	existing []rbacv1.PolicyRule, owned, read, removed []string,
) []rbacv1.PolicyRule {
	if len(existing) == 0 {
		existing = []rbacv1.PolicyRule{
			{
				APIGroups: []string{rbacv1.APIGroupAll},
				Resources: []string{"secrets"},
				Verbs: []string{
					"create",
					"patch", // TODO: we really should only allow "create" but not patch  but currently we uses .Apply() which requres patch!!!
				},
			},
		}
		if isControllerModel {
			// We need to be able to list/get all namespaces for units in controller model.
			existing = append(existing, rbacv1.PolicyRule{
				APIGroups: []string{rbacv1.APIGroupAll},
				Resources: []string{"namespaces"},
				Verbs:     []string{"get", "list"},
			})
		} else {
			// We just need to be able to list/get our own namespace for units in other models.
			existing = append(existing, rbacv1.PolicyRule{
				APIGroups:     []string{rbacv1.APIGroupAll},
				Resources:     []string{"namespaces"},
				Verbs:         []string{"get", "list"},
				ResourceNames: []string{namespace},
			})
		}
	}

	ownedIDs := set.NewStrings(owned...)
	readIDs := set.NewStrings(read...)
	removedIDs := set.NewStrings(removed...)

	existing = cleanRules(existing,
		func(s string) bool {
			return ownedIDs.Contains(s) || readIDs.Contains(s) || removedIDs.Contains(s)
		},
	)

	for _, rName := range owned {
		if removedIDs.Contains(rName) {
			continue
		}
		existing = append(existing, rbacv1.PolicyRule{
			APIGroups:     []string{rbacv1.APIGroupAll},
			Resources:     []string{"secrets"},
			Verbs:         []string{rbacv1.VerbAll},
			ResourceNames: []string{rName},
		})
	}
	for _, rName := range read {
		if removedIDs.Contains(rName) {
			continue
		}
		existing = append(existing, rbacv1.PolicyRule{
			APIGroups:     []string{rbacv1.APIGroupAll},
			Resources:     []string{"secrets"},
			Verbs:         []string{"get"},
			ResourceNames: []string{rName},
		})
	}
	return existing
}

// RemoveSecretAccessToken removes the RBAC resources for the provided resource name.
func (k *kubernetesClient) RemoveSecretAccessToken(unit names.Tag) error {
	name := unit.String()
	if err := k.deleteRoleBinding(name, ""); err != nil {
		logger.Warningf("cannot delete service account %q", name)
	}
	if err := k.deleteRole(name, ""); err != nil {
		logger.Warningf("cannot delete service account %q", name)
	}
	if err := k.deleteServiceAccount(name, ""); err != nil {
		logger.Warningf("cannot delete service account %q", name)
	}
	return nil
}
