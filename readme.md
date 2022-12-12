# ssl-expiry-prometheus-operator

Prometheus Exporter that checks the SSL certificates of all ingress hosts within all namespaces.

# Metrics Overview

1. `ssl-expiry-prometheus-operator` calculates the number of days left with respect to the present date for the domain SSL certificate to expire, and exports the same as gauge value of the metric along with other labels:

```
ssl-expiry-prometheus-operator{domain="ssl-checker.com",ingress="default"}
```
2. If the exporter is unable to resolve the domain or if could not get any SSL certificate from the resquested domain, it will return gauge value as `-1`

# Add Helm Chart Repository

```
helm repo add ssl-expiry-prometheus-operator https://facets-cloud.github.io/ssl-expiry-prometheus-operator/helm/charts
helm repo update
```

# Install Chart

```
helm install [RELEASE_NAME] ssl-expiry-prometheus-operator/ssl-expiry-prometheus-operator
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