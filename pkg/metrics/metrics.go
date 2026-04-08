package metrics

import (
	"context"
	"fmt"
	"net/http"

	promclient "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/prometheus"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// Init creates an OTel MeterProvider with a Prometheus exporter and registers
// it globally. It returns an http.Handler that serves the /metrics endpoint
// and a shutdown function for graceful cleanup.
func Init(serviceName string) (http.Handler, func(context.Context) error, error) {
	registry := promclient.NewRegistry()

	exporter, err := prometheus.New(prometheus.WithRegisterer(registry))
	if err != nil {
		return nil, nil, fmt.Errorf("create prometheus exporter: %w", err)
	}

	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter))
	otel.SetMeterProvider(provider)

	handler := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	return handler, provider.Shutdown, nil
}
