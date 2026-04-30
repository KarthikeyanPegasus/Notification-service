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

const workflowOrchestrationVendorType = "workflow_orchestration"

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
	defaultEngine workflow.WorkflowEngine
	repo          repository.VendorConfigRepository
	cfg           *config.Config
	log           *zap.Logger

	mu    sync.Mutex
	cache map[string]workflow.WorkflowEngine // key = hostPort|namespace
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

func (p *WorkflowClientProvider) ClientForScope(ctx context.Context, apiKeyID *string) (workflow.WorkflowEngine, error) {
	// No scope => use the bootstrapped default engine.
	if apiKeyID == nil || *apiKeyID == "" {
		return p.defaultEngine, nil
	}

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

	provider := cfg.Provider
	hostPort := ""
	namespace := ""
	switch provider {
	case "cadence":
		hostPort = normalizeHostPort(cfg.Cadence.HostPort)
		namespace = cfg.Cadence.Domain
	case "standalone":
		return nil, nil
	default:
		// Treat unknown providers as temporal for forwards-compat.
		provider = "temporal"
		hostPort = normalizeHostPort(cfg.Temporal.HostPort)
		namespace = cfg.Temporal.Namespace
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
	if provider == "cadence" {
		engine, _ = workflow.NewCadenceEngineWith(hostPort, namespace, p.log)
	} else {
		engine, _ = workflow.NewTemporalEngineWith(hostPort, namespace, p.log)
	}

	p.cache[key] = engine
	return engine, nil
}

