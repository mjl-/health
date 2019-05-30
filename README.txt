health - web server serving a /health endpoint, for each request it performs health checks on other configurable services and reflects the result

Useful for letting external alerting tools watch your monitoring infrastructure.

	$ cat >health.conf <<EOF
	Endpoints:
		-
			Name: alertmanager
			URL: http://monitor:9093/api/v1/status
		-
			Name: prometheus
			URL: http://monitor:9090/status
	EOF

	$ health testconfig health.conf
	config OK

	$ ./health serve  -address localhost:8100 -monitor-address localhost:8101 health.conf 
	health version dev, listening on localhost:8100
	...

	$ curl http://localhost:8100/health
	ok

	$ curl http://localhost:8100/health
	500 internal server error - unhealthy: prometheus

	# Health has logged the failure. Possibly a timeout after 5 seconds, or non-2xx HTTP response.

Build info is exposed on localhost:8001/info.
Prometheus metrics are exposed on localhost:8001/metrics.
