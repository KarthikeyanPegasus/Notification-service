package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spidey/notification-service/internal/cache"
	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/repository"
)

// TemplateService manages and renders notification templates.
type TemplateService struct {
	repo  *repository.TemplateRepository
	cache *cache.Client
}

func NewTemplateService(repo *repository.TemplateRepository, cache *cache.Client) *TemplateService {
	return &TemplateService{repo: repo, cache: cache}
}

const templateCacheTTL = time.Hour

// GetByID returns a template by ID, using a 1-hour Redis cache.
func (s *TemplateService) GetByID(ctx context.Context, id uuid.UUID) (*domain.NotificationTemplate, error) {
	cacheKey := fmt.Sprintf("tmpl:id:%s", id)

	var tmpl domain.NotificationTemplate
	if err := s.cache.Get(ctx, cacheKey, &tmpl); err == nil {
		return &tmpl, nil
	}

	t, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	_ = s.cache.Set(ctx, cacheKey, t, templateCacheTTL)
	return t, nil
}

// Render substitutes {{variables}} into a template body and subject.
func (s *TemplateService) Render(tmpl *domain.NotificationTemplate, vars map[string]string) (*domain.RenderedContent, error) {
	body := s.RenderString(tmpl.Body, vars)

	content := &domain.RenderedContent{
		Body: body,
	}

	if tmpl.Subject != nil {
		content.Subject = s.RenderString(*tmpl.Subject, vars)
	}

	// For email: body is plain text, HTML is body wrapped in a basic layout
	if tmpl.Channel == domain.ChannelEmail {
		content.HTML = buildEmailHTML(content.Subject, body)
	}

	return content, nil
}

// RenderForChannel resolves template by ID (if given) or builds ad-hoc content.
func (s *TemplateService) RenderForChannel(
	ctx context.Context,
	templateID *uuid.UUID,
	channel domain.Channel,
	vars map[string]string,
	fallbackBody string,
) (*domain.RenderedContent, error) {
	if templateID != nil {
		tmpl, err := s.GetByID(ctx, *templateID)
		if err != nil {
			return nil, fmt.Errorf("resolving template %s: %w", templateID, err)
		}
		return s.Render(tmpl, vars)
	}

	// No template — use fallback body with variable substitution
	rendered := &domain.RenderedContent{
		Body: s.RenderString(fallbackBody, vars),
	}
	if channel == domain.ChannelEmail {
		rendered.HTML = buildEmailHTML("", rendered.Body)
	}
	return rendered, nil
}

// InvalidateCache removes a template from the cache.
func (s *TemplateService) InvalidateCache(ctx context.Context, id uuid.UUID) {
	_ = s.cache.Del(ctx, fmt.Sprintf("tmpl:id:%s", id))
}

// RenderString replaces {{key}} placeholders with values from vars.
func (s *TemplateService) RenderString(template string, vars map[string]string) string {
	if len(vars) == 0 {
		return template
	}
	result := template
	for k, v := range vars {
		result = strings.ReplaceAll(result, "{{"+k+"}}", v)
	}
	return result
}

// buildEmailHTML wraps plain-text body in a minimal responsive HTML layout.
func buildEmailHTML(subject, body string) string {
	// Escape for HTML display
	escaped := strings.ReplaceAll(body, "\n", "<br>")
	escaped = strings.ReplaceAll(escaped, "&", "&amp;")
	escaped = strings.ReplaceAll(escaped, "<br>", "<br>") // re-allow <br>

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>%s</title>
<style>
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;background:#f4f4f5;margin:0;padding:24px}
.card{background:#fff;border-radius:8px;padding:32px;max-width:600px;margin:0 auto}
p{color:#374151;line-height:1.6}
.footer{color:#9ca3af;font-size:12px;margin-top:24px;text-align:center}
</style>
</head>
<body>
<div class="card">
<p>%s</p>
<div class="footer">You received this notification from Notification Service</div>
</div>
</body>
</html>`, subject, escaped)
}
