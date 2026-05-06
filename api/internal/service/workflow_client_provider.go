package service

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"github.com/spidey/notification-service/internal/config"
	"github.com/spidey/notification-service/internal/repository"
	"github.com/spidey/notification-service/internal/workflow"
	"go.uber.org/zap"
)

const (
	workflowOrchestrationVendorType = "workflow_orchestration"
	autoscalerConfigVendorType      = "autoscaler"
)

type workflowOrchestrationConfig struct {
	Provider string `json:"provider"`
	Temporal struct {
		HostPort    string `json:"host_port"`
		Namespace   string `json:"namespace"`
		FrontendURL string `json:"frontend_url"`
	} `json:"temporal"`
	Cadence struct {
		HostPort    string `json:"host_port"`
		Domain      string `json:"domain"`
		FrontendURL string `json:"frontend_url"`
	} `json:"cadence"`
}

type WorkflowClientProvider struct {
	defaultEngine    workflow.WorkflowEngine
	repo             repository.VendorConfigRepository
	retryConfigRepo  repository.VendorRetryConfigRepository
	cfg              *config.Config
	log              *zap.Logger
	activities       *workflow.Activities // Only set in Worker process for Go Routines mode

	mu    sync.Mutex
	cache map[string]workflow.WorkflowEngine // key = hostPort|namespace|provider
}

func NewWorkflowClientProvider(defaultEngine workflow.WorkflowEngine, repo repository.VendorConfigRepository, cfg *config.Config, log *zap.Logger) *WorkflowClientProvider {
	return &WorkflowClientProvider{
		defaultEngine: defaultEngine,
		repo:          repo,
		cfg:           cfg,
		log:           log,
		cache:         map[string]workflow.WorkflowEngine{},
	}
}

// WithRetryConfigRepo attaches the retry config repository for go_routines engine.
func (p *WorkflowClientProvider) WithRetryConfigRepo(retryRepo repository.VendorRetryConfigRepository) *WorkflowClientProvider {
	p.retryConfigRepo = retryRepo
	return p
}

// WithActivities attaches the Activities instance needed for Go Routines mode.
// This should only be called in the Worker process.
func (p *WorkflowClientProvider) WithActivities(activities *workflow.Activities) *WorkflowClientProvider {
	p.activities = activities
	return p
}

func normalizeHostPort(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	// UI may provide "http(s)://host:port" – we only want "host:port".
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimPrefix(s, "https://")
	// Drop any path/query fragments if present.
	if i := strings.IndexAny(s, "/?"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

func normalizeMode(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func (p *WorkflowClientProvider) ClientForScope(ctx context.Context, apiKeyID *string) (workflow.WorkflowEngine, error) {
	// No scope => check global mode and use default engine if applicable.
	if apiKeyID == nil || *apiKeyID == "" {
		// Standalone mode: the API never starts workflows directly.
		// All notifications are published to Kafka for the Worker to pick up.
		if normalizeMode(p.cfg.Cadence.Mode) == "standalone" {
			return nil, nil
		}
		return p.defaultEngine, nil
	}

	// Check for client-specific orchestration config.
	// Client configs override the global standalone mode.
	vc, err := p.repo.GetByType(ctx, workflowOrchestrationVendorType, apiKeyID)
	if err != nil {
		// Non-fatal: DB/decryption errors on config lookup should not block delivery.
		p.log.Warn("failed to load workflow orchestration config; using default engine",
			zap.String("api_key_id", *apiKeyID),
			zap.Error(err),
		)
		return p.defaultEngine, nil
	}
	if vc == nil || len(vc.ConfigJSON) == 0 {
		// No client-specific config => fall back to global mode/engine.
		if normalizeMode(p.cfg.Cadence.Mode) == "standalone" {
			return nil, nil
		}
		return p.defaultEngine, nil
	}

	var cfg workflowOrchestrationConfig
	if err := json.Unmarshal(vc.ConfigJSON, &cfg); err != nil {
		p.log.Warn("invalid workflow orchestration config json; falling back to default",
			zap.String("api_key_id", *apiKeyID),
			zap.Error(err),
		)
		return p.defaultEngine, nil
	}

	provider := normalizeMode(cfg.Provider)
	hostPort := ""
	namespace := ""
	switch provider {
	case "cadence":
		hostPort = normalizeHostPort(cfg.Cadence.HostPort)
		namespace = cfg.Cadence.Domain
	case "go_routines":
		// go_routines uses in-process goroutines; no host/port needed.
	case "standalone":
		return nil, nil
	default:
		// Treat unknown providers as temporal for forwards-compat.
		provider = "temporal"
		hostPort = normalizeHostPort(cfg.Temporal.HostPort)
		namespace = cfg.Temporal.Namespace
	}

	if provider == "go_routines" {
		// Go Routines mode requires Activities (only available in Worker process).
		// In API process, activities will be nil, so we return nil to fall back
		// to Kafka publishing (standalone mode behavior).
		if p.activities == nil {
			p.log.Debug("go_routines mode requested but activities not available; falling back to standalone behavior",
				zap.String("api_key_id", *apiKeyID),
			)
			return nil, nil
		}

		key := "go_routines"
		p.mu.Lock()
		defer p.mu.Unlock()
		if c, ok := p.cache[key]; ok {
			return c, nil
		}
		engine := workflow.NewGoRoutinesEngineWith(p.log, p.retryConfigRepo, p.activities)
		p.cache[key] = engine
		return engine, nil
	}

	if hostPort == "" || namespace == "" {
		return p.defaultEngine, nil
	}

	// If override matches default, reuse default.
	if p.defaultEngine != nil && hostPort == normalizeHostPort(p.cfg.Cadence.HostPort) && namespace == p.cfg.Cadence.Domain {
		return p.defaultEngine, nil
	}

	key := provider + "|" + hostPort + "|" + namespace
	p.mu.Lock()
	defer p.mu.Unlock()
	if c, ok := p.cache[key]; ok {
		return c, nil
	}

	var engine workflow.WorkflowEngine
	var engineErr error
	if provider == "cadence" {
		engine, engineErr = workflow.NewCadenceEngineWith(hostPort, namespace, p.log)
	} else {
		engine, engineErr = workflow.NewTemporalEngineWith(hostPort, namespace, p.log)
	}

	if engineErr != nil {
		p.log.Error("failed to create workflow engine",
			zap.String("provider", provider),
			zap.String("host_port", hostPort),
			zap.String("namespace", namespace),
			zap.Error(engineErr),
		)
		return nil, engineErr
	}

	p.cache[key] = engine
	return engine, nil
}

func (p *WorkflowClientProvider) EngineFromConfig(ctx context.Context, configJSON json.RawMessage) (workflow.WorkflowEngine, error) {
	if len(configJSON) == 0 {
		return p.defaultEngine, nil
	}

	var cfg workflowOrchestrationConfig
	if err := json.Unmarshal(configJSON, &cfg); err != nil {
		return nil, err
	}

	provider := normalizeMode(cfg.Provider)
	hostPort := ""
	namespace := ""
	switch provider {
	case "cadence":
		hostPort = normalizeHostPort(cfg.Cadence.HostPort)
		namespace = cfg.Cadence.Domain
	case "go_routines":
	case "standalone":
		return nil, nil
	default:
		provider = "temporal"
		hostPort = normalizeHostPort(cfg.Temporal.HostPort)
		namespace = cfg.Temporal.Namespace
	}

	if provider == "go_routines" {
		// Go Routines mode requires Activities (only available in Worker process).
		if p.activities == nil {
			p.log.Debug("go_routines mode requested but activities not available; falling back to standalone behavior")
			return nil, nil
		}

		key := "go_routines"
		p.mu.Lock()
		defer p.mu.Unlock()
		if c, ok := p.cache[key]; ok {
			return c, nil
		}
		engine := workflow.NewGoRoutinesEngineWith(p.log, p.retryConfigRepo, p.activities)
		p.cache[key] = engine
		return engine, nil
	}

	if hostPort == "" || namespace == "" {
		return p.defaultEngine, nil
	}

	key := provider + "|" + hostPort + "|" + namespace
	p.mu.Lock()
	defer p.mu.Unlock()
	if c, ok := p.cache[key]; ok {
		return c, nil
	}

	var engine workflow.WorkflowEngine
	var engineErr error
	if provider == "cadence" {
		engine, engineErr = workflow.NewCadenceEngineWith(hostPort, namespace, p.log)
	} else {
		engine, engineErr = workflow.NewTemporalEngineWith(hostPort, namespace, p.log)
	}

	if engineErr != nil {
		p.log.Error("failed to create workflow engine",
			zap.String("provider", provider),
			zap.String("host_port", hostPort),
			zap.String("namespace", namespace),
			zap.Error(engineErr),
		)
		return nil, engineErr
	}

	p.cache[key] = engine
	return engine, nil
}

