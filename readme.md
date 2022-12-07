# ssl-checker

Prometheus Exporter that checks the SSL certificates of all ingress hosts within all namespaces.

# Metrics Overview

1. `ssl-checker` calculates the number of days left with respect to the present date for the domain SSL certificate to expire, and exports the same as gauge value of the metric along with other labels:

```
ssl_checker{domain="ssl-checker.com",ingress="default"}
```
2. If the exporter is unable to resolve the domain or if could not get any SSL certificate from the resquested domain, it will return gauge value as `-1`

