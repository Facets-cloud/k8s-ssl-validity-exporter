package main

import (
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// TestCollectConcurrentNoRace verifies that concurrent Collect() calls do not
// panic or race on shared state.
//
// Before the fix, package-level globals for the WaitGroup and certificate slice
// were shared across calls, and wg.Add was called *after* launching each
// goroutine. Under concurrent scrapes (Prometheus every 120 s + kubelet
// readiness probe every 20 s) this caused:
//   - data races on the shared slice
//   - WaitGroup counter underflow → "sync: negative WaitGroup counter" panic
//   - pod stuck not-ready until liveness probe forced a restart
//
// Run with: go test -race ./...
func TestCollectConcurrentNoRace(t *testing.T) {
	fakeClient := fake.NewSimpleClientset(
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "default"},
		},
		&networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-ingress",
				Namespace: "default",
			},
			Spec: networkingv1.IngressSpec{
				Rules: []networkingv1.IngressRule{
					// 127.0.0.1:443 → connection refused, so the TLS dial
					// fails fast without a real cluster or certificate.
					{Host: "127.0.0.1"},
				},
			},
		},
	)

	exporter := NewExporter(fakeClient)

	const concurrency = 10
	var wg sync.WaitGroup
	wg.Add(concurrency)

	panics := make(chan any, concurrency)
	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					panics <- r
				}
			}()
			ch := make(chan prometheus.Metric, 100)
			exporter.Collect(ch)
		}()
	}

	wg.Wait()
	close(panics)
	for p := range panics {
		t.Errorf("Collect() panicked: %v", p)
	}
}
