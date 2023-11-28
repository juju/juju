// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"time"

	"github.com/juju/errors"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"

	"github.com/juju/juju/caas/kubernetes/pod"
)

const (
	// ForwardPortTimeout is the duration for waiting for a pod to be ready.
	ForwardPortTimeout time.Duration = time.Minute * 10
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

// Close disconnects a tunnel connection
func (t *Tunnel) Close() {
	select {
	case <-t.stopChan:
	default:
		close(t.stopChan)
	}
}

// findSuitablePodForService when tunneling to a kubernetes service we need to
// introspection.
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
		return nil, errors.NotFoundf("pods for service %s", t.Target)
	} else if podCount == 1 {
		return &pods.Items[0], nil
	}

	return &pods.Items[rand.Intn(podCount-1)], nil
}

func (t *Tunnel) ForwardPort() error {
	if !t.IsValidTunnelKind() {
		return fmt.Errorf("invalid tunnel kind %s", t.Kind)
	}

	ctx, cancelFunc := context.WithTimeout(context.Background(), ForwardPortTimeout)
	defer cancelFunc()

	podName := t.Target

	if t.Kind == TunnelKindServices {
		pod, err := t.findSuitablePodForService()
		if err != nil {
			return errors.Trace(err)
		}
		podName = pod.Name
	}

	err := t.waitForPodReady(ctx, podName)
	if err != nil {
		return errors.Annotatef(err, "waiting for pod %s to become ready for tunnel", podName)
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

	errChan := make(chan error, 1)
	go func() {
		errChan <- pf.ForwardPorts()
	}()

	select {
	case <-ctx.Done():
		close(t.stopChan)
		return ctx.Err()
	case err = <-errChan:
		close(t.stopChan)
		return fmt.Errorf("forwarding ports: %v", err)
	case <-pf.Ready:
		return nil
	}
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

// IsValidTunnelKind tests that the tunnel kind supplied to this tunnel is valid
func (t *Tunnel) IsValidTunnelKind() bool {
	switch t.Kind {
	case TunnelKindPods,
		TunnelKindServices:
		return true
	}
	return false
}

// NewTunnelForConfig constructs a new tunnel from the provided rest config
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

// NewTunnel constructs a new kubernetes tunnel for the provided information
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
		Out:        io.Discard,
		readyChan:  make(chan struct{}, 1),
		RemotePort: remotePort,
		stopChan:   make(chan struct{}, 1),
		Target:     target,
	}
}

// waitForPodReady waits for the provided pod name relative to this tunnels
// namespace to become fully ready in the pod conditions. This func will block
// until the pod is ready of the context dead line has fired.
func (t *Tunnel) waitForPodReady(ctx context.Context, podName string) error {
	clientSet := kubernetes.New(t.client)
	factory := informers.NewSharedInformerFactoryWithOptions(
		clientSet,
		time.Minute,
		informers.WithNamespace(t.Namespace),
	)
	informer := factory.Core().V1().Pods().Informer()

	stopChan := make(chan struct{})
	eventChan := make(chan error)
	defer close(stopChan)
	defer close(eventChan)

	_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			objPod, valid := obj.(*corev1.Pod)
			if !valid {
				eventChan <- errors.New("expected valid pod for informer")
				return
			}

			if objPod.Name == podName && pod.IsPodRunning(objPod) {
				eventChan <- nil
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			objPod, valid := newObj.(*corev1.Pod)
			if !valid {
				eventChan <- errors.New("expected valid pod for informer")
				return
			}

			if objPod.Name == podName && pod.IsPodRunning(objPod) {
				eventChan <- nil
			}
		},
		DeleteFunc: func(obj interface{}) {
			pod, valid := obj.(*corev1.Pod)
			if !valid {
				eventChan <- errors.New("expected valid pod for informer")
				return
			}

			if pod.Name == podName {
				eventChan <- errors.Errorf("tunnel pod %s is being deleted", podName)
			}
		},
	})
	if err != nil {
		return errors.Trace(err)
	}

	go informer.Run(stopChan)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-eventChan:
		return err
	}
}
