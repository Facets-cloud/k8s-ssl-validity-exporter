# k8s-ingress-ssl-metrics-exporter

Prometheus Exporter that checks the SSL certificates of all ingress hosts within all namespaces and returns expiration days with respect to current date as the gauge value.

# Metrics Overview

`ssl_expiry` calculates the number of days left with respect to the present date for the domain SSL certificate to expire, and exports the same as gauge value of the metric along with other labels:

```
ssl_expiry{common_name="commonName",domain="ssl-checker.com",ingress="default",namespace="default"} 57
```

# Add Helm Chart Repository

```
helm repo add k8s-ingress-ssl-metrics-exporter https://facets-cloud.github.io/k8s-ingress-ssl-metrics-exporter/helm/charts
helm repo update
```

# Install Chart

```
helm install [RELEASE_NAME] k8s-ingress-ssl-metrics-exporter/k8s-ingress-ssl-metrics-exporter
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
