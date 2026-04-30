package service

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spidey/notification-service/internal/circuit"
	nsconfig "github.com/spidey/notification-service/internal/config"
	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/provider"
	"go.uber.org/zap"
)

// DeliveryService encapsulates the logic for choosing a provider and sending a notification.
// It supports various routing modes (backup, round-robin, only, publish_all) and handles circuit breaking.
type DeliveryService struct {
	mu           sync.RWMutex
	senders      map[domain.Channel][]provider.Sender
	routing      map[domain.Channel]nsconfig.RoutingConfig
	slackSender  provider.Sender
	registry     *circuit.Registry
	log          *zap.Logger
	rrIndices    map[domain.Channel]*uint64
}

func NewDeliveryService(registry *circuit.Registry, log *zap.Logger) *DeliveryService {
	return &DeliveryService{
		senders:   make(map[domain.Channel][]provider.Sender),
		routing:   make(map[domain.Channel]nsconfig.RoutingConfig),
		registry:  registry,
		log:       log,
		rrIndices: make(map[domain.Channel]*uint64),
	}
}

func (s *DeliveryService) Registry() *circuit.Registry {
	return s.registry
}

func (s *DeliveryService) Reload(ctx context.Context, cfg nsconfig.ProviderConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Email
	emailSenders := provider.InitializeEmailSenders(ctx, cfg.Email)
	s.senders[domain.ChannelEmail] = emailSenders
	emailRouting := cfg.EmailRouting
	if emailRouting.Prefer == "" {
		emailRouting.Prefer = preferredProviderFromPrimary(cfg.Email.Primary)
	}
	s.routing[domain.ChannelEmail] = emailRouting
	if s.rrIndices[domain.ChannelEmail] == nil {
		var zero uint64
		s.rrIndices[domain.ChannelEmail] = &zero
	}

	// SMS
	smsSenders := provider.InitializeSMSSenders(cfg.SMS)
	s.senders[domain.ChannelSMS] = smsSenders
	s.routing[domain.ChannelSMS] = cfg.SMSRouting
	if s.rrIndices[domain.ChannelSMS] == nil {
		var zero uint64
		s.rrIndices[domain.ChannelSMS] = &zero
	}

	// Push
	pushSenders := provider.InitializePushSenders(cfg.Push)
	s.senders[domain.ChannelPush] = pushSenders
	s.routing[domain.ChannelPush] = cfg.PushRouting
	if s.rrIndices[domain.ChannelPush] == nil {
		var zero uint64
		s.rrIndices[domain.ChannelPush] = &zero
	}

	// Slack
	s.slackSender = provider.InitializeSlackSender(cfg.Slack)

	s.log.Info("delivery service reloaded with new configuration")
}

func (s *DeliveryService) Deliver(ctx context.Context, n *domain.Notification) (domain.DeliveryResult, error) {
	s.mu.RLock()
	channel := n.Channel
	senders := s.senders[channel]
	routing := s.routing[channel]
	slackSender := s.slackSender
	rrIndex := s.rrIndices[channel]
	s.mu.RUnlock()

	return s.deliver(ctx, n, channel, senders, routing, slackSender, rrIndex)
}

func (s *DeliveryService) deliver(
	ctx context.Context,
	n *domain.Notification,
	channel domain.Channel,
	senders []provider.Sender,
	routing nsconfig.RoutingConfig,
	slackSender provider.Sender,
	rrIndex *uint64,
) (domain.DeliveryResult, error) {
	if channel == domain.ChannelSlack {
		if slackSender == nil {
			return domain.DeliveryResult{}, fmt.Errorf("slack vendor not configured")
		}
		return slackSender.Send(ctx, n)
	}

	if len(senders) == 0 {
		return domain.DeliveryResult{}, fmt.Errorf("no providers configured for channel %s", channel)
	}

	// Handle ForcedVendor (from test deliveries)
	if n.ForcedVendor != "" {
		vendor := s.normalizeVendorName(channel, n.ForcedVendor)
		for _, snd := range senders {
			if snd.ProviderName() == vendor {
				return snd.Send(ctx, n)
			}
		}
		return domain.DeliveryResult{}, fmt.Errorf("forced vendor %q not configured for %s channel", n.ForcedVendor, channel)
	}

	mode := s.normalizeRoutingMode(routing.Mode)
	
	// Ensure stable order for predictable routing/RR
	senders = s.stableVendors(senders)

	switch mode {
	case "only":
		only := s.normalizeVendorName(channel, routing.Only)
		if only == "" {
			only = s.normalizeVendorName(channel, routing.Prefer)
		}
		if only == "" {
			return domain.DeliveryResult{}, fmt.Errorf("%s routing mode=only requires routing.only (or routing.prefer)", channel)
		}
		for _, snd := range senders {
			if snd.ProviderName() == only {
				res, err := snd.Send(ctx, n)
				if res.Provider == "" {
					res.Provider = snd.ProviderName()
				}
				return res, err
			}
		}
		return domain.DeliveryResult{}, fmt.Errorf("%s routing vendor %q not configured", channel, only)

	case "round_robin":
		participants := make(map[string]struct{}, len(routing.Participants))
		for _, p := range routing.Participants {
			participants[s.normalizeVendorName(channel, p)] = struct{}{}
		}
		rrSenders := senders
		if len(participants) > 0 {
			rrSenders = make([]provider.Sender, 0, len(senders))
			for _, snd := range senders {
				if _, ok := participants[snd.ProviderName()]; ok {
					rrSenders = append(rrSenders, snd)
				}
			}
		}

		if len(rrSenders) == 0 {
			return domain.DeliveryResult{ErrorMessage: "no providers configured"}, domain.ErrAllProvidersOpen
		}
		idx := int(atomic.AddUint64(rrIndex, 1)-1) % len(rrSenders)
		snd := rrSenders[idx]
		cb := s.registry.GetOrDefault(snd.ProviderName())
		if cb.IsOpen() {
			return domain.DeliveryResult{ErrorMessage: "circuit breaker open"}, domain.ErrAllProvidersOpen
		}

		var result domain.DeliveryResult
		start := time.Now()
		_, execErr := cb.Execute(func() (any, error) {
			r, err := snd.Send(ctx, n)
			result = r
			return r, err
		})
		result.LatencyMs = int(time.Since(start).Milliseconds())
		if result.Provider == "" {
			result.Provider = snd.ProviderName()
		}
		if execErr != nil {
			result.Success = false
			result.ErrorMessage = execErr.Error()
			return result, execErr
		}
		return result, nil

	case "publish_all":
		// Best-effort send through all
		anySuccess := false
		var lastErr error
		for _, snd := range senders {
			cb := s.registry.GetOrDefault(snd.ProviderName())
			if cb.IsOpen() {
				continue
			}
			res, err := snd.Send(ctx, n)
			if res.Provider == "" {
				res.Provider = snd.ProviderName()
			}
			if err == nil && res.Success {
				anySuccess = true
			} else if err != nil {
				lastErr = err
			}
		}
		if anySuccess {
			return domain.DeliveryResult{Success: true}, nil
		}
		if lastErr != nil {
			return domain.DeliveryResult{}, lastErr
		}
		return domain.DeliveryResult{}, fmt.Errorf("all providers failed in publish_all mode")

	case "backup":
		fallthrough
	default:
		prefer := s.normalizeVendorName(channel, routing.Prefer)
		fallback := s.normalizeVendorName(channel, routing.Fallback)
		
		if prefer != "" || fallback != "" {
			ordered := make([]provider.Sender, 0, len(senders))
			preferFallback := s.shouldPreferFallback(prefer, routing)
			first := prefer
			second := fallback
			if preferFallback && fallback != "" {
				first, second = fallback, prefer
			}
			for _, snd := range senders {
				if first != "" && snd.ProviderName() == first {
					ordered = append(ordered, snd)
				}
			}
			for _, snd := range senders {
				if second != "" && snd.ProviderName() == second && snd.ProviderName() != first {
					ordered = append(ordered, snd)
				}
			}
			for _, snd := range senders {
				if snd.ProviderName() != first && snd.ProviderName() != second {
					ordered = append(ordered, snd)
				}
			}
			senders = ordered
		}
		return s.withFallback(ctx, senders, n)
	}
}

func (s *DeliveryService) DeliverScoped(ctx context.Context, n *domain.Notification, cfg nsconfig.ProviderConfig) (domain.DeliveryResult, error) {
	channel := n.Channel
	var senders []provider.Sender
	var routing nsconfig.RoutingConfig
	var slackSender provider.Sender

	switch channel {
	case domain.ChannelEmail:
		senders = provider.InitializeEmailSenders(ctx, cfg.Email)
		routing = cfg.EmailRouting
		if routing.Prefer == "" {
			routing.Prefer = preferredProviderFromPrimary(cfg.Email.Primary)
		}
	case domain.ChannelSMS:
		senders = provider.InitializeSMSSenders(cfg.SMS)
		routing = cfg.SMSRouting
	case domain.ChannelPush:
		senders = provider.InitializePushSenders(cfg.Push)
		routing = cfg.PushRouting
	case domain.ChannelSlack:
		slackSender = provider.InitializeSlackSender(cfg.Slack)
	}

	var rrIndex uint64
	return s.deliver(ctx, n, channel, senders, routing, slackSender, &rrIndex)
}

func (s *DeliveryService) withFallback(ctx context.Context, senders []provider.Sender, n *domain.Notification) (domain.DeliveryResult, error) {
	var lastErr error
	for _, snd := range senders {
		vendor := snd.ProviderName()
		cb := s.registry.GetOrDefault(vendor)
		if cb.IsOpen() {
			continue
		}

		var result domain.DeliveryResult
		_, err := cb.Execute(func() (any, error) {
			r, err := snd.Send(ctx, n)
			result = r
			return r, err
		})
		if result.Provider == "" {
			result.Provider = vendor
		}

		if err == nil {
			return result, nil
		}
		lastErr = err
		s.log.Warn("provider failed — trying next", zap.String("vendor", vendor), zap.Error(err))
	}

	if lastErr != nil {
		return domain.DeliveryResult{ErrorMessage: "all providers failed"}, lastErr
	}
	return domain.DeliveryResult{ErrorMessage: "no providers configured"}, domain.ErrAllProvidersOpen
}

func (s *DeliveryService) shouldPreferFallback(primaryVendor string, routing nsconfig.RoutingConfig) bool {
	if s.registry == nil || routing.ErrorRateThreshold <= 0 || primaryVendor == "" {
		return false
	}
	minReq := routing.MinRequests
	if minReq <= 0 {
		minReq = 20
	}
	cb := s.registry.GetOrDefault(primaryVendor)
	counts := cb.Counts()
	total := int(counts.TotalSuccesses + counts.TotalFailures)
	if total < minReq || counts.TotalFailures == 0 {
		return false
	}
	errRate := float64(counts.TotalFailures) / float64(total)
	return errRate >= routing.ErrorRateThreshold
}

func (s *DeliveryService) normalizeVendorName(ch domain.Channel, v string) string {
	if ch == domain.ChannelEmail {
		switch v {
		case "ses", "amazon-ses", "amazon_ses":
			return "amazon-ses"
		case "smtp", "smtp-relay", "smtp_relay":
			return "smtp-relay"
		case "mailgun":
			return "mailgun"
		}
	}
	return v
}

func (s *DeliveryService) normalizeRoutingMode(mode string) string {
	switch mode {
	case "preference", "prefer", "":
		return "backup"
	default:
		return mode
	}
}

func (s *DeliveryService) stableVendors(senders []provider.Sender) []provider.Sender {
	sort.SliceStable(senders, func(i, j int) bool {
		return senders[i].ProviderName() < senders[j].ProviderName()
	})
	return senders
}

func preferredProviderFromPrimary(primary string) string {
	switch primary {
	case "ses":
		return "amazon-ses"
	case "smtp":
		return "smtp-relay"
	case "mailgun":
		return "mailgun"
	default:
		return ""
	}
}
