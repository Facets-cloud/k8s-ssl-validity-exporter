# K8s SSL Validity Exporter

Prometheus Exporter that checks the SSL certificates of all ingress hosts within all namespaces and returns expiration days with respect to current date as the gauge value.

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
