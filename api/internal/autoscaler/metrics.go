package autoscaler

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// WorkerParallelismDesired is the desired worker parallelism per key.
	workerParallelismDesired = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "notif_autoscaler_parallelism_desired",
			Help: "Desired worker parallelism per (client, channel, priority)",
		},
		[]string{"client_id", "channel", "priority"},
	)

	// WorkerParallelismActual is the actual running worker count per key.
	// Set externally by the WorkerManager after reconcile.
	WorkerParallelismActual = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "notif_autoscaler_parallelism_actual",
			Help: "Actual running worker count per (client, channel, priority)",
		},
		[]string{"client_id", "channel", "priority"},
	)

	kafkaLagGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "notif_autoscaler_kafka_lag",
			Help: "Kafka consumer lag per topic",
		},
		[]string{"topic"},
	)

	mttdGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "notif_autoscaler_mttd_ms",
			Help: "Mean time to delivery in milliseconds per (client, priority)",
		},
		[]string{"client_id", "priority"},
	)

	scaleUpTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "notif_autoscaler_scale_up_total",
			Help: "Total number of scale-up decisions per key",
		},
		[]string{"client_id", "channel", "priority"},
	)

	scaleDownTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "notif_autoscaler_scale_down_total",
			Help: "Total number of scale-down decisions per key",
		},
		[]string{"client_id", "channel", "priority"},
	)
)

// exportMetrics updates Prometheus gauges with the current autoscaler state,
// lag data, and MTTD data. Called from evaluateAll().
func exportMetrics(
	states map[scaleKey]*ScalingState,
	lagByChannelPriority map[string]int64,
	mttdMap map[string]float64,
) {
	// Desired parallelism
	workerParallelismDesired.Reset()
	for k, s := range states {
		workerParallelismDesired.WithLabelValues(k.ClientID, k.Channel, k.Priority).Set(float64(s.Desired))
	}

	// Kafka lag
	kafkaLagGauge.Reset()
	for key, lag := range lagByChannelPriority {
		kafkaLagGauge.WithLabelValues(key).Set(float64(lag))
	}

	// MTTD
	mttdGauge.Reset()
	for key, ms := range mttdMap {
		clientID, priority := splitMttdKey(key)
		if clientID != "" && priority != "" {
			mttdGauge.WithLabelValues(clientID, priority).Set(ms)
		}
	}
}

func splitMttdKey(key string) (clientID, priority string) {
	// Key format: "clientID|priority"
	for i := 0; i < len(key); i++ {
		if key[i] == '|' {
			return key[:i], key[i+1:]
		}
	}
	return "", ""
}
