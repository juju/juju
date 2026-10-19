// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	corev1 "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/scheme"
	restfake "k8s.io/client-go/rest/fake"
	"k8s.io/client-go/tools/portforward"

	"github.com/juju/juju/internal/testhelpers"
)

type tunnelSuite struct {
	testhelpers.IsolationSuite
}

func TestTunnelSuite(t *testing.T) {
	tc.Run(t, &tunnelSuite{})
}

func (s *tunnelSuite) TestCloseIsConcurrentSafe(c *tc.C) {
	tunnel := newTestTunnel()
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			tunnel.Close()
		})
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		wg.Wait()
	}()

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for concurrent closes")
	}
	select {
	case <-tunnel.stopChan:
	default:
		c.Fatalf("stop channel was not closed")
	}
}

func (s *tunnelSuite) TestForwardPortBindsIPv4LocalhostAndUsesAssignedPort(c *tc.C) {
	var gotAddresses []string
	var gotPorts []string
	done := make(chan struct{})
	forwarder := &fakePortForwarder{
		done:        done,
		signalReady: true,
		waitForStop: true,
		forwardedPorts: []portforward.ForwardedPort{{
			Local:  33419,
			Remote: 17070,
		}},
	}
	tunnel := newTestTunnelWithForwarder(func(
		_ httpstream.Dialer,
		addresses []string,
		ports []string,
		stopChan <-chan struct{},
		readyChan chan struct{},
		_, _ io.Writer,
	) (portForwarder, error) {
		gotAddresses = append([]string(nil), addresses...)
		gotPorts = append([]string(nil), ports...)
		forwarder.stopChan = stopChan
		forwarder.readyChan = readyChan
		return forwarder, nil
	})
	err := tunnel.forwardPort(c.Context(), nil, "test-pod", "")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotAddresses, tc.DeepEquals, []string{"127.0.0.1"})
	c.Check(gotPorts, tc.DeepEquals, []string{"0:17070"})
	c.Check(tunnel.LocalPort, tc.Equals, "33419")

	tunnel.Close()
	assertDone(c, done)
}

func (s *tunnelSuite) TestForwardPortReportsFailureBeforeReady(c *tc.C) {
	done := make(chan struct{})
	forwarder := &fakePortForwarder{
		done:       done,
		forwardErr: errors.New("lost connection to pod"),
	}
	tunnel := newTestTunnelWithForwarder(func(
		_ httpstream.Dialer,
		_ []string,
		_ []string,
		stopChan <-chan struct{},
		readyChan chan struct{},
		_, _ io.Writer,
	) (portForwarder, error) {
		forwarder.stopChan = stopChan
		forwarder.readyChan = readyChan
		return forwarder, nil
	})
	err := tunnel.forwardPort(c.Context(), nil, "test-pod", "")
	c.Assert(err, tc.ErrorMatches, "forwarding ports: lost connection to pod")
	assertDone(c, done)
}

func (s *tunnelSuite) TestForwardPortStopsWhenAssignedPortCannotBeRead(c *tc.C) {
	done := make(chan struct{})
	forwarder := &fakePortForwarder{
		done:        done,
		signalReady: true,
		waitForStop: true,
		getPortsErr: errors.New("ports unavailable"),
	}
	tunnel := newTestTunnelWithForwarder(func(
		_ httpstream.Dialer,
		_ []string,
		_ []string,
		stopChan <-chan struct{},
		readyChan chan struct{},
		_, _ io.Writer,
	) (portForwarder, error) {
		forwarder.stopChan = stopChan
		forwarder.readyChan = readyChan
		return forwarder, nil
	})
	err := tunnel.forwardPort(c.Context(), nil, "test-pod", "")
	c.Assert(err, tc.ErrorMatches, "getting forwarded ports: ports unavailable")
	assertDone(c, done)
}

func (s *tunnelSuite) TestForwardPortReportsPostReadyFailure(c *tc.C) {
	done := make(chan struct{})
	release := make(chan struct{})
	forwarder := &fakePortForwarder{
		done:        done,
		release:     release,
		signalReady: true,
		forwardErr:  errors.New("lost connection to pod"),
		forwardedPorts: []portforward.ForwardedPort{{
			Local:  33419,
			Remote: 17070,
		}},
	}
	tunnel := newTestTunnelWithForwarder(func(
		_ httpstream.Dialer,
		_ []string,
		_ []string,
		stopChan <-chan struct{},
		readyChan chan struct{},
		_, _ io.Writer,
	) (portForwarder, error) {
		forwarder.stopChan = stopChan
		forwarder.readyChan = readyChan
		return forwarder, nil
	})
	err := tunnel.forwardPort(c.Context(), nil, "test-pod", "")
	c.Assert(err, tc.ErrorIsNil)

	close(release)
	assertDone(c, done)
	assertForwardError(c, tunnel, "lost connection to pod")
	assertBroken(c, tunnel)
}

func (s *tunnelSuite) TestForwardPortBrokenNotClosedBeforeForward(c *tc.C) {
	done := make(chan struct{})
	release := make(chan struct{})
	forwarder := &fakePortForwarder{
		done:        done,
		release:     release,
		signalReady: true,
		forwardedPorts: []portforward.ForwardedPort{{
			Local:  33419,
			Remote: 17070,
		}},
	}
	tunnel := newTestTunnelWithForwarder(func(
		_ httpstream.Dialer,
		_ []string,
		_ []string,
		stopChan <-chan struct{},
		readyChan chan struct{},
		_, _ io.Writer,
	) (portForwarder, error) {
		forwarder.stopChan = stopChan
		forwarder.readyChan = readyChan
		return forwarder, nil
	})
	err := tunnel.forwardPort(c.Context(), nil, "test-pod", "")
	c.Assert(err, tc.ErrorIsNil)

	select {
	case <-tunnel.Broken():
		c.Fatalf("broken closed before forwarder exited")
	default:
	}

	close(release)
	assertDone(c, done)
	assertBroken(c, tunnel)
}

func (s *tunnelSuite) TestPodHealthCheckStopsTunnelWhenPodIsMissing(c *tc.C) {
	tunnel := newTestTunnel()
	tunnel.Namespace = "test-namespace"
	tunnel.client = newPodListRESTClient(&corev1.PodList{})

	tunnel.defaultPodHealthCheck("missing-pod", "")

	assertStopped(c, tunnel)
}

func (s *tunnelSuite) TestPodHealthCheckStopsTunnelWhenPodTerminates(c *tc.C) {
	tunnel := newTestTunnel()
	tunnel.Namespace = "test-namespace"
	tunnel.client = newPodListRESTClient(&corev1.PodList{Items: []corev1.Pod{{
		ObjectMeta: meta.ObjectMeta{Name: "test-pod", Namespace: tunnel.Namespace, UID: "pod-uid"},
		Status:     corev1.PodStatus{Phase: corev1.PodFailed},
	}}})

	tunnel.defaultPodHealthCheck("test-pod", "pod-uid")

	assertStopped(c, tunnel)
}

func (s *tunnelSuite) TestPodHealthCheckStopsTunnelWhenPodIsDeleted(c *tc.C) {
	tunnel := newTestTunnel()
	tunnel.Namespace = "test-namespace"
	pod := &corev1.Pod{ObjectMeta: meta.ObjectMeta{
		Name:      "test-pod",
		Namespace: tunnel.Namespace,
		UID:       "pod-uid",
	}}
	tunnel.client = newPodWatchRESTClient(
		&corev1.PodList{Items: []corev1.Pod{*pod}},
		watch.Event{Type: watch.Deleted, Object: pod},
	)

	tunnel.defaultPodHealthCheck(pod.Name, pod.UID)

	assertStopped(c, tunnel)
}

func (s *tunnelSuite) TestPodHealthCheckStopsTunnelWhenPodIsReplaced(c *tc.C) {
	tunnel := newTestTunnel()
	tunnel.Namespace = "test-namespace"
	pod := &corev1.Pod{ObjectMeta: meta.ObjectMeta{
		Name:      "test-pod",
		Namespace: tunnel.Namespace,
		UID:       "original-uid",
	}}
	replacement := pod.DeepCopy()
	replacement.UID = "replacement-uid"
	tunnel.client = newPodWatchRESTClient(
		&corev1.PodList{Items: []corev1.Pod{*pod}},
		watch.Event{Type: watch.Modified, Object: replacement},
	)

	tunnel.defaultPodHealthCheck(pod.Name, pod.UID)

	assertStopped(c, tunnel)
}

func (s *tunnelSuite) TestPodHealthCheckDoesNotStopTunnelForNonTerminalPod(c *tc.C) {
	tunnel := newTestTunnel()
	pod := &corev1.Pod{
		ObjectMeta: meta.ObjectMeta{UID: "pod-uid"},
		Status:     corev1.PodStatus{Phase: corev1.PodPending},
	}

	tunnel.checkTunnelPod(c.Context(), "test-pod", types.UID("pod-uid"), pod)

	select {
	case <-tunnel.stopChan:
		c.Fatal("tunnel stopped for non-terminal pod")
	default:
	}
}

type fakePortForwarder struct {
	done           chan struct{}
	forwardErr     error
	forwardedPorts []portforward.ForwardedPort
	getPortsErr    error
	readyChan      chan struct{}
	release        chan struct{}
	signalReady    bool
	stopChan       <-chan struct{}
	waitForStop    bool
}

func (f *fakePortForwarder) ForwardPorts() error {
	if f.done != nil {
		defer close(f.done)
	}
	if f.signalReady {
		close(f.readyChan)
	}
	if f.release != nil {
		select {
		case <-f.release:
		case <-f.stopChan:
			return nil
		}
	}
	if f.waitForStop {
		<-f.stopChan
	}
	return f.forwardErr
}

func (f *fakePortForwarder) GetPorts() ([]portforward.ForwardedPort, error) {
	if f.getPortsErr != nil {
		return nil, f.getPortsErr
	}
	return f.forwardedPorts, nil
}

func newTestTunnel() *Tunnel {
	return newTestTunnelWithForwarder(defaultPortForwarder)
}

func newTestTunnelWithForwarder(forwarderFactory portForwarderFactory) *Tunnel {
	return &Tunnel{
		Out:              io.Discard,
		broken:           make(chan struct{}),
		newPortForwarder: forwarderFactory,
		readyChan:        make(chan struct{}),
		RemotePort:       "17070",
		stopChan:         make(chan struct{}),
	}
}

func newPodWatchRESTClient(pods *corev1.PodList, events ...watch.Event) *restfake.RESTClient {
	return &restfake.RESTClient{
		GroupVersion:         schema.GroupVersion{Version: "v1"},
		NegotiatedSerializer: scheme.Codecs.WithoutConversion(),
		VersionedAPIPath:     "/api",
		Client: restfake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
			if req.URL.Query().Get("watch") != "" {
				var data bytes.Buffer
				initialEvents := make([]watch.Event, 0, len(pods.Items)+len(events)+1)
				for i := range pods.Items {
					initialEvents = append(initialEvents, watch.Event{
						Type:   watch.Added,
						Object: &pods.Items[i],
					})
				}
				bookmark := &corev1.Pod{ObjectMeta: meta.ObjectMeta{
					Annotations:     map[string]string{meta.InitialEventsAnnotationKey: "true"},
					ResourceVersion: "1",
				}}
				initialEvents = append(initialEvents, watch.Event{Type: watch.Bookmark, Object: bookmark})
				initialEvents = append(initialEvents, events...)
				for _, event := range initialEvents {
					object, err := runtime.Encode(scheme.Codecs.LegacyCodec(corev1.SchemeGroupVersion), event.Object)
					if err != nil {
						return nil, err
					}
					if err := json.NewEncoder(&data).Encode(meta.WatchEvent{
						Type:   string(event.Type),
						Object: runtime.RawExtension{Raw: object},
					}); err != nil {
						return nil, err
					}
				}
				return newPodWatchRESTResponse(data.Bytes()), nil
			}
			data, err := runtime.Encode(scheme.Codecs.LegacyCodec(corev1.SchemeGroupVersion), pods)
			if err != nil {
				return nil, err
			}
			return newPodRESTResponse(data), nil
		}),
	}
}

func newPodListRESTClient(pods *corev1.PodList) *restfake.RESTClient {
	return &restfake.RESTClient{
		GroupVersion:         schema.GroupVersion{Version: "v1"},
		NegotiatedSerializer: scheme.Codecs.WithoutConversion(),
		VersionedAPIPath:     "/api",
		Client: restfake.CreateHTTPClient(func(*http.Request) (*http.Response, error) {
			data, err := runtime.Encode(scheme.Codecs.LegacyCodec(corev1.SchemeGroupVersion), pods)
			if err != nil {
				return nil, err
			}
			return newPodRESTResponse(data), nil
		}),
	}
}

func newPodRESTResponse(data []byte) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{runtime.ContentTypeJSON}},
		Body:       io.NopCloser(bytes.NewReader(data)),
	}
}

func newPodWatchRESTResponse(data []byte) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{runtime.ContentTypeJSON + ";stream=watch"}},
		Body:       io.NopCloser(bytes.NewReader(data)),
	}
}

func assertDone(c *tc.C, done <-chan struct{}) {
	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for forwarding goroutine")
	}
}

func assertForwardError(c *tc.C, tunnel *Tunnel, match string) {
	select {
	case err := <-tunnel.errChan:
		c.Check(err, tc.ErrorMatches, match)
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for forwarding error")
	}
}

func assertBroken(c *tc.C, tunnel *Tunnel) {
	select {
	case <-tunnel.Broken():
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for broken channel to close")
	}
}

func assertStopped(c *tc.C, tunnel *Tunnel) {
	select {
	case <-tunnel.stopChan:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for tunnel to stop")
	}
}
