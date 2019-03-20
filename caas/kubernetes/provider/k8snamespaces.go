// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"github.com/juju/errors"
	core "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"

	jujuannotations "github.com/juju/juju/caas/kubernetes/provider/annotations"
	"github.com/juju/juju/core/watcher"
)

var requireAnnotationsForNameSpace = []string{
	annotationControllerUUIDKey, annotationModelUUIDKey,
}

func checkNamespaceOwnedByJuju(ns *core.Namespace, annotationMap map[string]string) error {
	if ns == nil {
		return nil
	}
	if jujuannotations.New(ns.GetAnnotations()).ExistAll(annotationMap) {
		return nil
	}
	return errors.NotValidf(
		"namespace %q is not owned by Juju, it has annotation %v, but required %v",
		ns.GetName(), ns.GetAnnotations(), annotationMap,
	)
}

// Namespaces returns names of the namespaces on the cluster.
func (k *kubernetesClient) Namespaces() ([]string, error) {
	namespaces := k.CoreV1().Namespaces()
	ns, err := namespaces.List(v1.ListOptions{IncludeUninitialized: true})
	if err != nil {
		return nil, errors.Annotate(err, "listing namespaces")
	}
	result := make([]string, len(ns.Items))
	for i, n := range ns.Items {
		if err := checkNamespaceOwnedByJuju(&n, k.annotations.ToMap()); err != nil {
			continue
		}
		result[i] = n.Name
	}
	return result, nil
}

// GetNamespace returns the namespace for the specified name.
func (k *kubernetesClient) GetNamespace(name string) (*core.Namespace, error) {
	notFoundErr := errors.NotFoundf("namespace %q", name)
	ns, err := k.CoreV1().Namespaces().Get(name, v1.GetOptions{IncludeUninitialized: true})
	if k8serrors.IsNotFound(err) {
		return nil, notFoundErr
	}
	if err != nil {
		return nil, errors.Annotatef(err, "getting namespace %q", name)
	}
	if err := checkNamespaceOwnedByJuju(ns, k.annotations.ToMap()); err != nil {
		return nil, notFoundErr
	}
	return ns, nil
}

func (k *kubernetesClient) getOneNamespaceByAnnotations(annotations jujuannotations.Annotation) (*core.Namespace, error) {
	namespaces, err := k.CoreV1().Namespaces().List(v1.ListOptions{IncludeUninitialized: true})
	if err != nil {
		return nil, errors.Trace(err)
	}
	var matchedNS []core.Namespace
	annotationMap := annotations.ToMap()
	logger.Criticalf("finding namespace for annotations %v", annotationMap)
	for _, ns := range namespaces.Items {
		logger.Criticalf("getOneNamespaceByAnnotations k.namespace -> %q, ns.Namespace -> %q", k.namespace, ns.GetName())
		if err := checkNamespaceOwnedByJuju(&ns, annotationMap); err != nil {
			continue
		}
		if k.namespace != "" && k.namespace == ns.GetName() {
			return &ns, nil
		}
		matchedNS = append(matchedNS, ns)
	}
	if len(matchedNS) > 0 {
		doLog := logger.Criticalf
		if len(matchedNS) > 1 {
			// this should never happen before we enable multi controller in single cluster.
			doLog = logger.Errorf
		}
		doLog("found %d matched namespaces with annotations %v", len(matchedNS), annotationMap)
		return &matchedNS[0], nil
	}
	return nil, errors.NotFoundf("namespace %v", k.annotations)
}

// GetCurrentNamespace returns current namespace name.
func (k *kubernetesClient) GetCurrentNamespace() string {
	return k.namespace
}

// EnsureNamespace ensures this broker's namespace is created.
func (k *kubernetesClient) EnsureNamespace() error {
	ns := &core.Namespace{ObjectMeta: v1.ObjectMeta{Name: k.namespace}}
	if err := k.ensureNamespaceAnnotations(ns); err != nil {
		return errors.Trace(err)
	}
	namespaces := k.CoreV1().Namespaces()
	_, err := namespaces.Update(ns)
	if k8serrors.IsNotFound(err) {
		_, err = namespaces.Create(ns)
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) ensureNamespaceAnnotations(ns *core.Namespace) error {
	annotations := jujuannotations.New(ns.GetAnnotations()).Merge(k.annotations)
	logger.Criticalf("ensureNamespaceAnnotations ------> %v", annotations)
	// check required keys are set: annotationControllerUUIDKey, annotationModelUUIDKey.
	if err := annotations.CheckKeysNonEmpty(requireAnnotationsForNameSpace...); err != nil {
		return errors.Trace(err)
	}
	ns.SetAnnotations(annotations.ToMap())
	return nil
}

// createNamespace creates a named namespace.
func (k *kubernetesClient) createNamespace(name string) error {
	ns := &core.Namespace{ObjectMeta: v1.ObjectMeta{Name: name}}
	if err := k.ensureNamespaceAnnotations(ns); err != nil {
		return errors.Trace(err)
	}
	_, err := k.CoreV1().Namespaces().Create(ns)
	if k8serrors.IsAlreadyExists(err) {
		return errors.AlreadyExistsf("namespace %q already exists", name)
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) deleteNamespace() error {
	// deleteNamespace is used as a means to implement Destroy().
	// All model resources are provisioned in the namespace;
	// deleting the namespace will also delete those resources.
	ns, err := k.GetNamespace(k.namespace)
	if k8serrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return errors.Trace(err)
	}

	if err := checkNamespaceOwnedByJuju(ns, k.annotations.ToMap()); err != nil {
		return errors.Trace(err)
	}

	err = k.CoreV1().Namespaces().Delete(k.namespace, &v1.DeleteOptions{
		PropagationPolicy: &defaultPropagationPolicy,
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

// WatchNamespace returns a watcher which notifies when there
// are changes to current namespace.
func (k *kubernetesClient) WatchNamespace() (watcher.NotifyWatcher, error) {
	w, err := k.CoreV1().Namespaces().Watch(
		v1.ListOptions{
			FieldSelector:        fields.OneTermEqualSelector("metadata.name", k.namespace).String(),
			IncludeUninitialized: true,
		},
	)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return k.newWatcher(w, k.namespace, k.clock)
}
