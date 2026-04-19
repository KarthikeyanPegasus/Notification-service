package worker

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// NotificationsProcessedTotal tracks the number of notifications processed by channel, status, and provider.
	NotificationsProcessedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "notifications_processed_total",
		Help: "Total number of notifications processed by the worker.",
	}, []string{"channel", "status", "provider"})

	// NotificationProcessingDurationSeconds tracks the latency of notification delivery attempts.
	NotificationProcessingDurationSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "notification_processing_duration_seconds",
		Help:    "Latency of notification delivery attempts per provider.",
		Buckets: prometheus.DefBuckets,
	}, []string{"channel", "provider"})
)
