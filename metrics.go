package main

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics is a collection of Prometheus metrics for the MCP proxy
type Metrics struct {
	// HTTP metrics
	requestsTotal      *prometheus.CounterVec
	requestDuration    *prometheus.HistogramVec
	requestsInProgress *prometheus.GaugeVec
	requestSizeBytes   *prometheus.HistogramVec
	responseSizeBytes  *prometheus.HistogramVec

	// MCP client metrics
	clientConnections      *prometheus.GaugeVec
	clientConnectionErrors *prometheus.CounterVec
	toolCalls              *prometheus.CounterVec
	toolCallErrors         *prometheus.CounterVec
	toolCallDuration       *prometheus.HistogramVec
}

// newMetrics creates and registers metrics with Prometheus
func newMetrics() *Metrics {
	m := &Metrics{
		// HTTP metrics
		requestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "mcp_proxy",
				Name:      "requests_total",
				Help:      "Total number of HTTP requests processed by the MCP proxy",
			},
			[]string{"method", "path", "status"},
		),
		requestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "mcp_proxy",
				Name:      "request_duration_seconds",
				Help:      "Duration of HTTP requests processed by the MCP proxy",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"method", "path"},
		),
		requestsInProgress: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "mcp_proxy",
				Name:      "requests_in_progress",
				Help:      "Number of HTTP requests currently being processed by the MCP proxy",
			},
			[]string{"method", "path"},
		),
		requestSizeBytes: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "mcp_proxy",
				Name:      "request_size_bytes",
				Help:      "Size of HTTP requests in bytes",
				Buckets:   prometheus.ExponentialBuckets(100, 10, 8), // Start at 100B with 8 buckets, each 10x the previous
			},
			[]string{"method", "path"},
		),
		responseSizeBytes: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "mcp_proxy",
				Name:      "response_size_bytes",
				Help:      "Size of HTTP responses in bytes",
				Buckets:   prometheus.ExponentialBuckets(100, 10, 8),
			},
			[]string{"method", "path"},
		),

		// MCP client metrics
		clientConnections: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "mcp_proxy",
				Name:      "client_connections",
				Help:      "Number of active connections to MCP clients",
			},
			[]string{"client_name", "client_type"},
		),
		clientConnectionErrors: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "mcp_proxy",
				Name:      "client_connection_errors_total",
				Help:      "Total number of connection errors to MCP clients",
			},
			[]string{"client_name", "client_type", "error_type"},
		),
		toolCalls: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "mcp_proxy",
				Name:      "tool_calls_total",
				Help:      "Total number of tool calls made through the MCP proxy",
			},
			[]string{"client_name", "tool_name"},
		),
		toolCallErrors: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "mcp_proxy",
				Name:      "tool_call_errors_total",
				Help:      "Total number of tool call errors",
			},
			[]string{"client_name", "tool_name", "error_type"},
		),
		toolCallDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "mcp_proxy",
				Name:      "tool_call_duration_seconds",
				Help:      "Duration of tool calls",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"client_name", "tool_name"},
		),
	}

	return m
}

// globalMetrics is the singleton instance of Metrics
var globalMetrics *Metrics

// GetMetrics returns the global metrics instance
func GetMetrics() *Metrics {
	if globalMetrics == nil {
		globalMetrics = newMetrics()
	}
	return globalMetrics
}