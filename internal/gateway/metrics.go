package gateway

import (
	"log"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// ConnectedClients tracks the number of currently connected WebSocket clients.
	ConnectedClients = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "goclaw_connected_clients",
		Help: "The number of currently connected WebSocket clients",
	})

	// MessagesTotal tracks the total number of messages sent and received.
	MessagesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "goclaw_messages_total",
		Help: "The total number of messages sent and received",
	}, []string{"direction"}) // "in", "out"

	// ErrorsTotal tracks the total number of errors encountered.
	ErrorsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "goclaw_errors_total",
		Help: "The total number of errors encountered",
	}, []string{"type"}) // "auth", "protocol", "internal"
)

// MetricsHandler returns the HTTP handler for Prometheus metrics.
func MetricsHandler() http.Handler {
	return promhttp.Handler()
}

// IncConnectedClients increments the connected clients gauge.
func IncConnectedClients() {
	ConnectedClients.Inc()
}

// DecConnectedClients decrements the connected clients gauge.
func DecConnectedClients() {
	ConnectedClients.Dec()
}

// IncMessageIn increments the incoming message counter.
func IncMessageIn() {
	MessagesTotal.WithLabelValues("in").Inc()
}

// IncMessageOut increments the outgoing message counter.
func IncMessageOut() {
	MessagesTotal.WithLabelValues("out").Inc()
}

// IncError increments the error counter for the given type.
func IncError(errType string) {
	ErrorsTotal.WithLabelValues(errType).Inc()
}

func init() {
	// Optional: Unregister default Go/Process metrics if we want a cleaner output,
	// but keeping them is standard practice.
	log.Println("Metrics initialized")
}
