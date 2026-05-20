package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"math"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// ssl_expiry is a Prometheus metric to check SSL expiration dates for all ingress hosts
var ssl_expiry = prometheus.NewDesc(
	prometheus.BuildFQName("", "", "ssl_expiry"),
	"Checking SSL Expiration Dates of all ingress hosts",
	[]string{"domain", "ingress", "common_name", "namespace"},
	nil,
)

func main() {
	kubeConfig := flag.String("kubeconfig", "", "Kubeconfig path")
	port := flag.String("port", "8080", "Port on which the server is listening, defaults to 8080")
	flag.Parse()

	clientset, err := kubeClient(kubeConfig)
	if err != nil {
		fmt.Println("Failed to create Kubernetes client: ", err)
		panic(err.Error())
	}

	exporter := NewExporter(clientset)
	prometheus.MustRegister(exporter)

	log.Print("Starting Metrics Server...")
	log.Printf("Listening on port %s", *port)
	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(":"+*port, nil))
}

// kubeClient creates a Kubernetes client using in-cluster config or the provided kubeconfig file.
func kubeClient(kubeConfig *string) (*kubernetes.Clientset, error) {
	if flag.CommandLine.Lookup("kubeconfig") == nil {
		config, err := rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to get in-cluster config: %v", err)
		}
		clientset, err := kubernetes.NewForConfig(config)
		if err != nil {
			return nil, fmt.Errorf("failed to create clientset: %v", err)
		}
		fmt.Print("Built config from Service Account")
		return clientset, nil
	}
	config, err := clientcmd.BuildConfigFromFlags("", *kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to build config from kubeconfig: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %v", err)
	}
	log.Print("Built config from kubeconfig flag...")
	return clientset, nil
}

// namespaces retrieves a list of all namespace names in the cluster.
func namespaces(client *kubernetes.Clientset) ([]string, error) {
	nsList, err := client.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces: %v", err)
	}
	var ns []string
	for _, namespace := range nsList.Items {
		ns = append(ns, namespace.Name)
	}
	return ns, nil
}

// ingressDomains returns ingress→host mappings across all provided namespaces.
func ingressDomains(client *kubernetes.Clientset, namespaces []string) ([]map[string]string, error) {
	var domains []map[string]string
	for _, ns := range namespaces {
		ingresses, err := client.NetworkingV1().Ingresses(ns).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to list ingress resources in namespace %s: %v", ns, err)
		}
		for _, ingress := range ingresses.Items {
			for _, rule := range ingress.Spec.Rules {
				domains = append(domains, map[string]string{
					"ingress":   ingress.Name,
					"domain":    rule.Host,
					"namespace": ingress.Namespace,
				})
			}
		}
	}
	return domains, nil
}

// getCertificatesExpiry dials a TLS connection to domain:443 and appends certificate
// expiry data to certs. certs and mu must be local to the enclosing Collect call to
// avoid data races across concurrent scrapes.
func getCertificatesExpiry(domain, ingress, namespace string, certs *[]map[string]string, mu *sync.Mutex, wg *sync.WaitGroup) {
	defer wg.Done()

	dialer := &net.Dialer{
		Timeout:   5 * time.Second,
		KeepAlive: 5 * time.Second,
	}
	conn, err := tls.DialWithDialer(dialer, "tcp", domain+":443", &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		log.Printf("Error dialing domain %s: %s", domain, err.Error())
		return
	}
	defer conn.Close()

	for _, certificate := range conn.ConnectionState().PeerCertificates {
		diff := certificate.NotAfter.Sub(time.Now())
		valInDays := fmt.Sprintf("%f", math.Round(diff.Hours()/24))
		mu.Lock()
		*certs = append(*certs, map[string]string{
			"domain":         domain,
			"ingress":        ingress,
			"expirationDays": valInDays,
			"commonName":     certificate.Subject.CommonName,
			"namespace":      namespace,
		})
		mu.Unlock()
	}
}

// filterDuplicateCertificates removes duplicate certificate entries.
func filterDuplicateCertificates(certificatesMap []map[string]string) []map[string]string {
	seen := make(map[string]bool)
	var filtered []map[string]string
	for _, cert := range certificatesMap {
		hash := fmt.Sprintf("%v", cert)
		if !seen[hash] {
			seen[hash] = true
			filtered = append(filtered, cert)
		}
	}
	return filtered
}

// Exporter is a Prometheus collector for SSL certificate expiration metrics.
type Exporter struct {
	clientset *kubernetes.Clientset
}

// NewExporter returns an Exporter backed by the given Kubernetes clientset.
func NewExporter(clientset *kubernetes.Clientset) *Exporter {
	return &Exporter{clientset: clientset}
}

// Describe sends the ssl_expiry metric descriptor to the channel.
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- ssl_expiry
}

// Collect fetches SSL certificates for all ingress hosts and emits expiry metrics.
// All mutable state is local to this call so concurrent Prometheus and kubelet
// scrapes cannot race on shared globals.
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	ns, err := namespaces(e.clientset)
	if err != nil {
		log.Printf("Failed to retrieve namespaces: %v", err)
		return
	}

	domainMap, err := ingressDomains(e.clientset, ns)
	if err != nil {
		log.Printf("Failed to fetch ingress domains: %v", err)
		return
	}

	// localCerts, mu, and wg are scoped to this Collect call.
	// This prevents data races when the kubelet readiness probe and Prometheus
	// scrape /metrics concurrently (e.g. probe every 20 s, Prometheus every 120 s).
	var (
		localCerts []map[string]string
		mu         sync.Mutex
		wg         sync.WaitGroup
	)

	for _, m := range domainMap {
		wg.Add(1) // Must be called before the goroutine starts to prevent WaitGroup underflow.
		go getCertificatesExpiry(m["domain"], m["ingress"], m["namespace"], &localCerts, &mu, &wg)
	}
	wg.Wait()

	for _, cert := range filterDuplicateCertificates(localCerts) {
		valInDays, err := strconv.ParseFloat(cert["expirationDays"], 64)
		if err != nil {
			log.Printf("Error parsing expirationDays for domain %s", cert["domain"])
			continue
		}
		ch <- prometheus.MustNewConstMetric(ssl_expiry, prometheus.GaugeValue, valInDays,
			cert["domain"], cert["ingress"], cert["commonName"], cert["namespace"])
	}
}
