// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"github.com/juju/errors"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/httpstream"
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

type portForwarder interface {
	ForwardPorts() error
	GetPorts() ([]portforward.ForwardedPort, error)
}

var newPortForwarder = func(
	dialer httpstream.Dialer,
	addresses []string,
	ports []string,
	stopChan <-chan struct{},
	readyChan chan struct{},
	out, errOut io.Writer,
) (portForwarder, error) {
	return portforward.NewOnAddresses(dialer, addresses, ports, stopChan, readyChan, out, errOut)
}

// Tunnel represents an ssh like tunnel to a Kubernetes Pod or Service
type Tunnel struct {
	client     rest.Interface
	config     *rest.Config
	Kind       TunnelKind
	errChan    chan error
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

// ForwardError returns a port-forwarding error observed after the tunnel was
// reported as ready.
func (t *Tunnel) ForwardError() error {
	if t.errChan == nil {
		return nil
	}
	select {
	case err := <-t.errChan:
		return err
	default:
		return nil
	}
}

// findSuitablePodForService when tunneling to a kubernetes service we need to
// introspection.
func (t *Tunnel) findSuitablePodForService(ctx context.Context) (*corev1.Pod, error) {
	clientSet := kubernetes.New(t.client)
	service, err := clientSet.CoreV1().Services(t.Namespace).
		Get(ctx, t.Target, meta.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NewNotFound(err, "can't find service "+t.Target)
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	pods, err := clientSet.CoreV1().Pods(t.Namespace).
		List(ctx, meta.ListOptions{
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

func (t *Tunnel) ForwardPort(ctx context.Context) error {
	if !t.IsValidTunnelKind() {
		return fmt.Errorf("invalid tunnel kind %s", t.Kind)
	}

	ctx, cancelFunc := context.WithTimeout(ctx, ForwardPortTimeout)
	defer cancelFunc()

	podName := t.Target

	if t.Kind == TunnelKindServices {
		pod, err := t.findSuitablePodForService(ctx)
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

	return t.forwardPort(ctx, dialer)
}

func (t *Tunnel) forwardPort(ctx context.Context, dialer httpstream.Dialer) error {
	ports := []string{fmt.Sprintf("0:%s", t.RemotePort)}
	pf, err := newPortForwarder(
		dialer,
		[]string{"127.0.0.1"},
		ports,
		t.stopChan,
		t.readyChan,
		t.Out,
		t.Out,
	)
	if err != nil {
		return err
	}

	errChan := make(chan error, 1)
	t.errChan = errChan
	go func() {
		errChan <- pf.ForwardPorts()
	}()

	select {
	case <-ctx.Done():
		t.Close()
		return ctx.Err()
	case err = <-errChan:
		t.Close()
		return fmt.Errorf("forwarding ports: %v", err)
	case <-t.readyChan:
		forwardedPorts, err := pf.GetPorts()
		if err != nil {
			t.Close()
			return fmt.Errorf("getting forwarded ports: %w", err)
		}
		if len(forwardedPorts) == 0 {
			t.Close()
			return errors.New("no forwarded ports")
		}
		t.LocalPort = strconv.Itoa(int(forwardedPorts[0].Local))
		return nil
	}
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

	eventChan := make(chan error)
	waitCtx, cancel := context.WithCancel(ctx)
	defer func() {
		cancel()
		close(eventChan)
	}()

	reg, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			objPod, valid := obj.(*corev1.Pod)
			if !valid {
				select {
				case <-waitCtx.Done():
				case eventChan <- errors.New("expected valid pod for informer"):
				}
				return
			}

			if objPod.Name == podName && pod.IsPodRunning(objPod) {
				select {
				case <-waitCtx.Done():
				case eventChan <- nil:
				}
			}
		},
		UpdateFunc: func(oldObj, newObj any) {
			objPod, valid := newObj.(*corev1.Pod)
			if !valid {
				select {
				case <-waitCtx.Done():
				case eventChan <- errors.New("expected valid pod for informer"):
				}
				return
			}

			if objPod.Name == podName && pod.IsPodRunning(objPod) {
				select {
				case <-waitCtx.Done():
				case eventChan <- nil:
				}
			}
		},
		DeleteFunc: func(obj any) {
			pod, valid := obj.(*corev1.Pod)
			if !valid {
				select {
				case <-waitCtx.Done():
				case eventChan <- errors.New("expected valid pod for informer"):
				}
				return
			}

			if pod.Name == podName {
				select {
				case <-waitCtx.Done():
				case eventChan <- errors.Errorf("tunnel pod %s is being deleted", podName):
				}
			}
		},
	})
	if err != nil {
		return errors.Trace(err)
	}
	defer func() {
		_ = informer.RemoveEventHandler(reg)
	}()

	err = informer.SetWatchErrorHandler(func(r *cache.Reflector, err error) {
		if !errors.Is(err, context.Canceled) {
			logger.Errorf(ctx, "error watching pod %s: %v", podName, err)
		}
	})
	if err != nil {
		return errors.Trace(err)
	}

	go informer.RunWithContext(waitCtx)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-eventChan:
		return err
	}
}
