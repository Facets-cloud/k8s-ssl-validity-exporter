package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"math"
	"net/http"
	"time"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	// "k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/rest"
)

func main() {

	// Generating kubeclient
	clientset, err := kubeClient()
	if err != nil {
		fmt.Println("Unable to create kubeclient")
		panic(err.Error())
	}

	// Runtime Loop
	exporter := NewExporter(clientset)
	prometheus.MustRegister(exporter)

	//Prometheus Http handler
	log.Print("Starting Metrics Server...")
	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func kubeClient() (*kubernetes.Clientset, error) {

	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}
	log.Print("Built in cluster configuration...")

	// use the current context in kubeconfig
	// config, err := clientcmd.BuildConfigFromFlags("", "/Users/ishaankalra/Downloads/aws-infra-dev-kubeconfig")
	// if err != nil {
	// 	fmt.Println("Config Error")
	// 	return nil, err
	// }
	// log.Print("Built config from Flags...")
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Println("clientset error")
		return nil, err
	}
	return clientset, nil
}

func namespacesList(client *kubernetes.Clientset) ([]string, error) {
	namespaces, err := client.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var ns []string
	for _, n := range namespaces.Items {
		ns = append(ns, n.Name)
	}
	return ns, nil
}

func ingressDomainsList(client *kubernetes.Clientset, namespaces []string) ([]map[string]string, error) {
	var ingressDomainMap []map[string]string
	for i := 0; i < len(namespaces); i++ {
		ingressList, err := client.NetworkingV1().Ingresses(namespaces[i]).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			fmt.Printf("Unabled to list ingress resources in namespace %s", namespaces[i])
			return nil, err
		}
		for _, v := range ingressList.Items {
			for _, x := range v.Spec.Rules {
				m := map[string]string{"ingress": v.Name,"domain": x.Host}
				ingressDomainMap = append(ingressDomainMap,m)
			}
		}
	}
	return ingressDomainMap, nil
}

// Prometheus Metrics
var ssl_checker = prometheus.NewDesc(
	prometheus.BuildFQName("", "", "ssl_checker"),
	"Checking SSL Expiration Dates of all ingress hosts",
	[]string{"domain", "ingress"},
	nil,
)

type Exporter struct {
	clientset *kubernetes.Clientset
}

func NewExporter(clientset *kubernetes.Clientset) *Exporter {
	return &Exporter{
		clientset: clientset,
	}
}

func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- ssl_checker
}

func (e *Exporter) Collect(ch chan<- prometheus.Metric) {

	// Fetch list of namespaces
	ns, err := namespacesList(e.clientset)
	if err != nil {
		fmt.Println("Unable to fetch list of namespaces")
		panic(err.Error())
	}

	// Fetch all ingress hosts in all namespaces
	ingressDomainMap, err := ingressDomainsList(e.clientset, ns)
	if err != nil {
		fmt.Println("Error fetching ingress")
		panic(err.Error())
	}


	for _, m := range ingressDomainMap {
		conn, err := tls.Dial("tcp", m["domain"]+":443", &tls.Config{
			InsecureSkipVerify: true,
		})
		if err != nil {
			log.Printf(err.Error()+" domain: %s", m["domain"])
			ch <- prometheus.MustNewConstMetric(ssl_checker, prometheus.GaugeValue, -1, m["domain"], m["ingress"])
		} else {
			for _,k := range conn.ConnectionState().PeerCertificates {
				if k.DNSNames != nil{
					expiry := k.NotAfter
					date := time.Now()
					diff := expiry.Sub(date)
					valInDays := math.Round(diff.Hours() / 24)
					ch <- prometheus.MustNewConstMetric(ssl_checker, prometheus.GaugeValue, valInDays, m["domain"], m["ingress"])
				}
			}
			defer conn.Close()
		}
	}
}