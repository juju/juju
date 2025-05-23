// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	fakeapiextensions "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	fakedynamic "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

func NoopFakeK8sClients(_ *rest.Config) (
	k8sClient kubernetes.Interface,
	apiextensionsclient apiextensionsclientset.Interface,
	dynamicClient dynamic.Interface,
	_ error,
) {
	k8sClient = fake.NewSimpleClientset()
	apiextensionsclient = fakeapiextensions.NewSimpleClientset()
	scheme := runtime.NewScheme()
	dynamicClient = fakedynamic.NewSimpleDynamicClient(scheme)
	return k8sClient, apiextensionsclient, dynamicClient, nil
}
