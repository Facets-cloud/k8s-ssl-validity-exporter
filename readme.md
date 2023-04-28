# K8s SSL Validity Exporter

A exporter that scans for Kubernetes ingress objects to determine the unique set of domains to monitor. It then initiates a TLS connection and retrieves the certificate chain for each domain. For each certificate in the chain, the exporter publishes a gauge metric called ssl_expiry, with the number of days until expiry as the gauge value, and relevant labels.

# Metrics Overview

`ssl_expiry` calculates the number of days left with respect to the present date for the domain SSL certificate to expire:

```
ssl_expiry{common_name="commonName",domain="ssl-checker.com",ingress="default",namespace="default"} 57
```

# Add Helm Chart Repository

```
helm repo add k8s-ssl-validity-exporter https://facets-cloud.github.io/k8s-ssl-validity-exporter/charts
helm repo update
```

# Install Chart

```
helm install k8s-ssl-validity-exporter k8s-ssl-validity-exporter/k8s-ssl-validity-exporter
```

# Uninstall Chart

```
helm uninstall [RELEASE_NAME]
```

# Getting Started

## Build

```
go run main.go -v
```

## Check the metrics

```
curl http://<pod-ip>:8080/metrics
```
