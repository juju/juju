// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"

	"github.com/juju/errors"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

// Tunnel represents an ssh like tunnel to a Kubernetes Pod or Service
type Tunnel struct {
	client     rest.Interface
	config     *rest.Config
	Kind       TunnelKind
	LocalPort  string
	Namespace  string
	Out        io.Writer
	readyChan  chan struct{}
	RemotePort string
	stopChan   chan struct{}
	Target     string
}

type TunnelKind string

const (
	TunnelKindPods     = TunnelKind("pods")
	TunnelKindServices = TunnelKind("services")
)

func NewTunnelForConfig(
	c *rest.Config,
	kind TunnelKind,
	namespace,
	target,
	remotePort string) (*Tunnel, error) {

	config := *c
	gv := corev1.SchemeGroupVersion
	config.GroupVersion = &gv
	config.APIPath = "/api"
	config.NegotiatedSerializer = scheme.Codecs.WithoutConversion()

	client, err := rest.RESTClientFor(&config)
	if err != nil {
		return nil, fmt.Errorf("failed creating kubernetes rest client for tunnel: %w", err)
	}

	return NewTunnel(client, &config, kind, namespace, target, remotePort), nil
}

func NewTunnel(
	client rest.Interface,
	c *rest.Config,
	kind TunnelKind,
	namespace,
	target,
	remotePort string) *Tunnel {

	return &Tunnel{
		client:     client,
		config:     c,
		Kind:       kind,
		Namespace:  namespace,
		Out:        ioutil.Discard,
		readyChan:  make(chan struct{}, 1),
		RemotePort: remotePort,
		stopChan:   make(chan struct{}, 1),
		Target:     target,
	}
}

func (t *Tunnel) findSuitablePodForService() (*corev1.Pod, error) {
	clientSet := kubernetes.New(t.client)
	service, err := clientSet.CoreV1().Services(t.Namespace).
		Get(context.TODO(), t.Target, meta.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NewNotFound(err, "can't find service "+t.Target)
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	pods, err := clientSet.CoreV1().Pods(t.Namespace).
		List(context.TODO(), meta.ListOptions{
			LabelSelector: labels.SelectorFromSet(service.Spec.Selector).String(),
		})

	if err != nil {
		return nil, errors.Trace(err)
	}

	podCount := len(pods.Items)
	if podCount == 0 {
		return nil, errors.NotFoundf("no pods founds for service %s", t.Target)
	} else if podCount == 1 {
		return &pods.Items[0], nil
	}

	return &pods.Items[rand.Intn(podCount-1)], nil
}

func (t *Tunnel) ForwardPort() error {
	if !t.IsValidTunnelKind() {
		return fmt.Errorf("invalid tunel kind %s", t.Kind)
	}

	podName := t.Target

	if t.Kind == TunnelKindServices {
		pod, err := t.findSuitablePodForService()
		if err != nil {
			return errors.Trace(err)
		}
		podName = pod.Name
	}

	u := t.client.Post().
		Resource(string(TunnelKindPods)).
		Namespace(t.Namespace).
		Name(podName).
		SubResource("portforward").URL()

	transport, upgrader, err := spdy.RoundTripperFor(t.config)
	if err != nil {
		return err
	}
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, u)

	local, err := getAvailablePort()
	if err != nil {
		return fmt.Errorf("could not find an available port: %w", err)
	}
	t.LocalPort = local

	ports := []string{fmt.Sprintf("%s:%s", t.LocalPort, t.RemotePort)}

	pf, err := portforward.New(dialer, ports, t.stopChan, t.readyChan, t.Out, t.Out)
	if err != nil {
		return err
	}

	errChan := make(chan error)
	go func() {
		errChan <- pf.ForwardPorts()
	}()

	select {
	case err = <-errChan:
		return fmt.Errorf("forwarding ports: %v", err)
	case <-pf.Ready:
		return nil
	}
	return nil
}

func getAvailablePort() (string, error) {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		return "", err
	}
	defer l.Close()

	_, p, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		return "", err
	}
	return p, err
}

func (t *Tunnel) IsValidTunnelKind() bool {
	switch t.Kind {
	case TunnelKindPods,
		TunnelKindServices:
		return true
	}
	return false
}
