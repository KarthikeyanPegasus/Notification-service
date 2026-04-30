package notification

import (
	"context"
	"encoding/json"
	"fmt"

	gcppubsub "cloud.google.com/go/pubsub"
	"google.golang.org/api/option"
)

// IngressPublisher publishes SendRequest payloads to a Pub/Sub topic that the API/worker ingress subscriber consumes.
// This is an alternative to calling POST /v1/notifications over HTTP.
type IngressPublisher struct {
	client  *gcppubsub.Client
	topicID string
}

type IngressPublisherOption func(*IngressPublisherConfig)

type IngressPublisherConfig struct {
	ProjectID          string
	TopicID            string
	CredentialsJSON    []byte
	CredentialsJSONRaw json.RawMessage
}

// WithIngressTopic sets the Pub/Sub topic ID to publish to.
func WithIngressTopic(topicID string) IngressPublisherOption {
	return func(c *IngressPublisherConfig) { c.TopicID = topicID }
}

// WithCredentialsJSON sets service-account credentials (raw JSON bytes).
func WithCredentialsJSON(b []byte) IngressPublisherOption {
	return func(c *IngressPublisherConfig) { c.CredentialsJSON = b }
}

// WithCredentialsJSONRaw sets service-account credentials (json.RawMessage).
func WithCredentialsJSONRaw(b json.RawMessage) IngressPublisherOption {
	return func(c *IngressPublisherConfig) { c.CredentialsJSONRaw = b }
}

// NewIngressPublisher creates a publisher for the ingress topic.
// projectID is required. topic defaults to "notifications-ingress" if not provided.
func NewIngressPublisher(ctx context.Context, projectID string, opts ...IngressPublisherOption) (*IngressPublisher, error) {
	cfg := &IngressPublisherConfig{
		ProjectID: projectID,
		TopicID:   "notifications-ingress",
	}
	for _, o := range opts {
		o(cfg)
	}
	if cfg.ProjectID == "" {
		return nil, fmt.Errorf("projectID is required")
	}

	var clientOpts []option.ClientOption
	creds := cfg.CredentialsJSON
	if len(creds) == 0 && len(cfg.CredentialsJSONRaw) > 0 {
		creds = []byte(cfg.CredentialsJSONRaw)
	}
	if len(creds) > 0 {
		clientOpts = append(clientOpts, option.WithCredentialsJSON(creds))
	}

	cli, err := gcppubsub.NewClient(ctx, cfg.ProjectID, clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("create pubsub client: %w", err)
	}
	return &IngressPublisher{client: cli, topicID: cfg.TopicID}, nil
}

func (p *IngressPublisher) Close() error {
	if p == nil || p.client == nil {
		return nil
	}
	return p.client.Close()
}

// Publish publishes the SendRequest JSON to the ingress topic.
func (p *IngressPublisher) Publish(ctx context.Context, req *SendRequest) (serverMsgID string, err error) {
	if p == nil || p.client == nil {
		return "", fmt.Errorf("publisher not initialized")
	}
	if req == nil {
		return "", fmt.Errorf("req is nil")
	}
	data, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}
	topic := p.client.Topic(p.topicID)
	defer topic.Stop()

	res := topic.Publish(ctx, &gcppubsub.Message{Data: data})
	id, err := res.Get(ctx)
	if err != nil {
		return "", fmt.Errorf("publish: %w", err)
	}
	return id, nil
}

