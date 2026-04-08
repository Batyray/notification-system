package metrics

import (
	"fmt"
	"net/http"

	promclient "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/prometheus"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// Init creates an OTel MeterProvider with a Prometheus exporter and registers
// it globally. It returns an http.Handler that serves the /metrics endpoint.
func Init(serviceName string) (http.Handler, error) {
	registry := promclient.NewRegistry()

	exporter, err := prometheus.New(prometheus.WithRegisterer(registry))
	if err != nil {
		return nil, fmt.Errorf("create prometheus exporter: %w", err)
	}

	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter))
	otel.SetMeterProvider(provider)

	handler := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	return handler, nil
}
