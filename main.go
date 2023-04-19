package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"math"
	"flag"
	"net/http"
	"time"
	"strconv"
	"net"
	"sync"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/rest"
)

// certificates holds the SSL certificate information for all ingress hosts
var certificates []map[string]string

// ssl_expiry is a Prometheus metric to check SSL expiration dates for all ingress hosts
var ssl_expiry = prometheus.NewDesc(
	prometheus.BuildFQName("", "", "ssl_expiry"),
	"Checking SSL Expiration Dates of all ingress hosts",
	[]string{"domain", "ingress", "common_name", "namespace"},
	nil,
)

var wg sync.WaitGroup // Wait group to ensure all SSL certificates are checked

var mut sync.Mutex // Mutex to lock certificates slice during concurrent writes

func main() {

	kubeConfig := flag.String("kubeconfig","","Kubeconfig path")
	port := flag.String("port","8080","Port on which the server is listening, defaults to 8080")
	flag.Parse()

	// Generating kubeclient
	clientset, err := kubeClient(kubeConfig)
	if err != nil {
		fmt.Println("Failed to create Kubernetes client: ", err)
		panic(err.Error())
	}

	// Runtime Loop
	exporter := NewExporter(clientset)
	prometheus.MustRegister(exporter)

	//Prometheus Http handler
	log.Print("Starting Metrics Server...")
	log.Printf("Listening on port %s",*port)
	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(":"+ *port, nil))
}

// kubeClient creates a Kubernetes client by either using the Kubernetes cluster config or the provided kubeconfig file. If no kubeconfig file is provided, it uses the in-cluster config.
func kubeClient(kubeConfig *string) (*kubernetes.Clientset, error) {
	// Check if kubeconfig flag is specified
	if flag.CommandLine.Lookup("kubeconfig") == nil {
		// Use in-cluster config if no kubeconfig flag is specified
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
	}else {
		// Use kubeconfig file for authentication if kubeconfig flag is specified
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
}

// namespaces retrieves a list of all namespaces in the cluster using the provided Kubernetes client.
func namespaces(client *kubernetes.Clientset) ([]string, error) {
	// Call the Kubernetes API to retrieve a list of all namespaces.
	namespaces, err := client.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		// If an error occurs, return nil and the error.
		return nil, fmt.Errorf("failed to list namespaces: %v", err)
	}

	// Create an empty slice to hold the namespace names.
	var ns []string

	// Loop through each namespace in the list and add its name to the slice.
	for _, namespace := range namespaces.Items {
		ns = append(ns, namespace.Name)
	}

	// Return the slice of namespace names.
	return ns, nil
}

// ingressDomains returns a list of all ingress domain mappings across the provided namespaces
func ingressDomains(client *kubernetes.Clientset, namespaces []string) ([]map[string]string, error) {
	var ingressDomains []map[string]string
	for i := 0; i < len(namespaces); i++ {
		ingresses, err := client.NetworkingV1().Ingresses(namespaces[i]).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to list the ingress resources in the namespace %s: %v", namespaces[i], err)
		}
		for _, ingress := range ingresses.Items {
			for _, ingressRule := range ingress.Spec.Rules {
				// For each ingress rule, add a mapping to the result array
				m := map[string]string{
					"ingress": ingress.Name,
					"domain": ingressRule.Host,
					"namespace": ingress.Namespace,
				}
				ingressDomains = append(ingressDomains,m)
			}
		}
	}
	// Return the array of ingress domain mappings
	return ingressDomains, nil
}

func getCertificatesExpiry(domain string, ingress string, namespace string) {
	defer wg.Done()
	dialer := &net.Dialer{
		Timeout:   5 * time.Second,
		KeepAlive: 5 * time.Second,
	}
	config := &tls.Config{
		InsecureSkipVerify: true,
	}

	// Dial with TLS to the given domain and port 443
	conn, err := tls.DialWithDialer(dialer,"tcp", domain +":443", config)
	if err != nil {
		// Print error message with domain and ingress details
		log.Printf("Error dialing domain %s: %s", domain, err.Error())
	} else {
		// If successful, iterate through the peer certificates and extract relevant information
		for _,certificate := range conn.ConnectionState().PeerCertificates {
			commonName := certificate.Subject.CommonName
			expiry := certificate.NotAfter
			date := time.Now()
			diff := expiry.Sub(date)
			valInDays := fmt.Sprintf("%f", math.Round(diff.Hours() / 24))
			// Append the certificate details to a list
			certificates = append(certificates, map[string]string{
				"domain": domain, 
				"ingress": ingress, 
				"expirationDays": valInDays, 
				"commonName": commonName, 
				"namespace": namespace,
			})
			}
			defer conn.Close()
	}
}

// filterDuplicateCertificates removes duplicate certificates from the given slice of certificate maps
func filterDuplicateCertificates(certificatesMap []map[string]string) ([]map[string]string) {
	check := make(map[string]bool)
	var filteredCertificates []map[string]string
	for _,certificate := range certificatesMap {
		// calculate a hash for the certificate map to check for duplicates
		hash := fmt.Sprintf("%v", certificate)

		// check if the certificate map has already been seen
		if !check[hash] {
			check[hash]= true
			filteredCertificates = append(filteredCertificates, certificate) 
		}
	}

	// return the new slice of unique certificates
	return filteredCertificates
}

// Prometheus exporter for SSL certificate expiration metrics.
type Exporter struct {
	clientset *kubernetes.Clientset
}

// New instance of the exporter with the given Kubernetes clientset.
func NewExporter(clientset *kubernetes.Clientset) *Exporter {
	return &Exporter{
		clientset: clientset,
	}
}

// Describe sends the description of the SSL expiry metric to the given channel.
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- ssl_expiry
}


// Collect fetches SSL certificates for all ingress hosts in all namespaces and sends the expiry metric to the given channel.
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {

	// Fetch list of namespaces
	ns, err := namespaces(e.clientset)
	if err != nil {
		fmt.Println("Failed to retrieve the namespaces:", err)
		panic(err.Error())
	}

	// Fetch all ingress hosts in all namespaces
	ingressDomainMap, err := ingressDomains(e.clientset, ns)
	if err != nil {
		fmt.Println("Failed to fetch the ingress domains")
		panic(err.Error())
	}

	// Retrieve the expiry metrics for all SSL certificates
	for _,m := range ingressDomainMap {
		mut.Lock()
		go getCertificatesExpiry(m["domain"],m["ingress"], m["namespace"])
		mut.Unlock()
		wg.Add(1)
	}
	wg.Wait()

	// Filter out any duplicate certificates
	filteredCertificates := filterDuplicateCertificates(certificates)

	// Send the SSL expiry metric to the given channel for each SSL certificate
	for _, metrics := range filteredCertificates {
		valInDays,err := strconv.ParseFloat(metrics["expirationDays"], 64) 
		if err != nil {
			log.Printf("Error typecasting int value to string")
		} else {
			ch <- prometheus.MustNewConstMetric(ssl_expiry, prometheus.GaugeValue, valInDays , metrics["domain"], metrics["ingress"], metrics["commonName"], metrics["namespace"])
		}
	}
}
