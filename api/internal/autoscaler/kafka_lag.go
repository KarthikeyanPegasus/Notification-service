package autoscaler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

// priorityTopicNames returns all 18 priority topic names.
func priorityTopicNames() []string {
	channels := allChannels()
	priorities := allPriorities()
	names := make([]string, 0, len(channels)*len(priorities))
	for _, ch := range channels {
		for _, pr := range priorities {
			names = append(names, fmt.Sprintf("notifications-%s-%s", ch, pr))
		}
	}
	return names
}

// KafkaLagMonitor periodically fetches consumer lag for all priority topics.
// It uses the kafka-go Client API to read:
//   - The latest offset (watermark) for each partition via ListOffsets
//   - The committed offset for each consumer group via OffsetFetch
//
// Lag = latest_offset - committed_offset (summed across all partitions).
type KafkaLagMonitor struct {
	brokers     []string
	groupIDBase string
	client      *kafka.Client
	mu          sync.Mutex
	log         *zap.Logger
}

// NewKafkaLagMonitor creates a KafkaLagMonitor.
// groupIDBase is the consumer group prefix (e.g. "notification-service").
// The monitor derives per-topic group IDs as {groupIDBase}-notif-dispatcher-{channel}-{priority}.
func NewKafkaLagMonitor(brokers []string, groupIDBase string, log *zap.Logger) *KafkaLagMonitor {
	client := &kafka.Client{
		Addr:    kafka.TCP(brokers...),
		Timeout: 10 * time.Second,
	}
	return &KafkaLagMonitor{
		brokers:     brokers,
		groupIDBase: groupIDBase,
		client:      client,
		log:         log.With(zap.String("component", "kafka_lag_monitor")),
	}
}

// Close is a no-op for this version of kafka-go Client (no Close method).
func (m *KafkaLagMonitor) Close() error {
	return nil
}

// GetConsumerLag returns the total consumer lag per topic, summed across all
// partitions. The key is the full topic name
// (e.g. "notifications-email-high").
func (m *KafkaLagMonitor) GetConsumerLag(ctx context.Context) (map[string]int64, error) {
	topics := priorityTopicNames()
	result := make(map[string]int64, len(topics))

	for _, topic := range topics {
		lag, err := m.topicLag(ctx, topic)
		if err != nil {
			m.log.Warn("failed to get lag for topic",
				zap.String("topic", topic), zap.Error(err))
			continue
		}
		result[topic] = lag
	}

	return result, nil
}

// topicLag computes the total consumer lag for a single topic across
// all partitions.
func (m *KafkaLagMonitor) topicLag(ctx context.Context, topic string) (int64, error) {
	// 1. Fetch partition metadata to discover how many partitions exist
	metaReq := &kafka.MetadataRequest{
		Topics: []string{topic},
	}
	metaResp, err := m.client.Metadata(ctx, metaReq)
	if err != nil {
		return 0, fmt.Errorf("metadata for %s: %w", topic, err)
	}
	if len(metaResp.Topics) == 0 {
		return 0, nil
	}
	topicMeta := metaResp.Topics[0]
	if topicMeta.Error != nil {
		return 0, fmt.Errorf("metadata error for %s: %w", topic, topicMeta.Error)
	}

	if len(topicMeta.Partitions) == 0 {
		return 0, nil
	}

	// 2. Fetch latest offsets for all partitions
	offsetReqs := make([]kafka.OffsetRequest, len(topicMeta.Partitions))
	partitionIDs := make([]int, len(topicMeta.Partitions))
	for i, p := range topicMeta.Partitions {
		offsetReqs[i] = kafka.LastOffsetOf(p.ID)
		partitionIDs[i] = p.ID
	}

	offsetsReq := &kafka.ListOffsetsRequest{
		Topics: map[string][]kafka.OffsetRequest{
			topic: offsetReqs,
		},
	}
	offsetsResp, err := m.client.ListOffsets(ctx, offsetsReq)
	if err != nil {
		return 0, fmt.Errorf("list offsets for %s: %w", topic, err)
	}

	// Build partition → latest offset map
	latestOffsets := make(map[int]int64)
	for _, partitionOffsets := range offsetsResp.Topics[topic] {
		pid := partitionOffsets.Partition
		// LastOffsetOf populates LastOffset (since we requested LastOffset)
		latestOffsets[pid] = partitionOffsets.LastOffset
	}

	// 3. Derive the dispatcher consumer group ID for this topic
	groupID := m.dispatcherGroupID(topic)
	if groupID == "" {
		return 0, nil
	}

	// 4. Fetch committed offsets for the consumer group using OffsetFetchRequest
	committedOffsets := make(map[int]int64) // partition → committed offset

	groupReq := &kafka.OffsetFetchRequest{
		GroupID: groupID,
		Topics: map[string][]int{
			topic: partitionIDs,
		},
	}
	groupResp, err := m.client.OffsetFetch(ctx, groupReq)
	if err != nil {
		// Consumer group might not exist yet — assume full lag
		m.log.Debug("consumer group not found, assuming full lag",
			zap.String("topic", topic),
			zap.String("group", groupID),
			zap.Error(err),
		)
		var total int64
		for _, off := range latestOffsets {
			total += off
		}
		return total, nil
	}

	if partitions, ok := groupResp.Topics[topic]; ok {
		for _, part := range partitions {
			if part.Error != nil {
				continue
			}
			committed := part.CommittedOffset
			if committed < 0 {
				committed = 0
			}
			committedOffsets[part.Partition] = committed
		}
	}

	// 5. Compute total lag: sum(latest - committed) across all partitions
	var totalLag int64
	for pid, latest := range latestOffsets {
		committed := committedOffsets[pid]
		lag := latest - committed
		if lag > 0 {
			totalLag += lag
		}
	}

	return totalLag, nil
}

// dispatcherGroupID derives the dispatcher consumer group ID for a given topic.
// Topic format: notifications-{channel}-{priority}
// Group format: {groupIDBase}-notif-dispatcher-{channel}-{priority}
func (m *KafkaLagMonitor) dispatcherGroupID(topic string) string {
	base := m.groupIDBase
	if base == "" {
		base = "notification-service"
	}

	// Parse topic name: notifications-{channel}-{priority}
	var channel, priority string
	_, err := fmt.Sscanf(topic, "notifications-%s-%s", &channel, &priority)
	if err != nil {
		return ""
	}

	return fmt.Sprintf("%s-notif-dispatcher-%s-%s", base, channel, priority)
}
