package security

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/textproto"
	"strings"

	"github.com/spidey/notification-service/internal/config"
	"github.com/spidey/notification-service/internal/domain"
	"go.uber.org/zap"
)

type ContentFilter interface {
	CheckContent(ctx context.Context, rendered *domain.RenderedContent) error
}

type contentFilter struct {
	cfg config.Config
	log *zap.Logger
}

func NewContentFilter(cfg config.Config) ContentFilter {
	return &contentFilter{cfg: cfg}
}

func NewContentFilterWithLogger(cfg config.Config, log *zap.Logger) ContentFilter {
	return &contentFilter{cfg: cfg, log: log}
}

func (f *contentFilter) CheckContent(ctx context.Context, rendered *domain.RenderedContent) error {
	// 1. Static Content Checks
	if f.cfg.Security.ContentSecurity.Enabled {
		if err := f.checkSussyWords(rendered.Body); err != nil {
			return err
		}
		if err := f.checkBlockedLinks(rendered.Body); err != nil {
			return err
		}
		if rendered.HTML != "" {
			if err := f.checkSussyWords(rendered.HTML); err != nil {
				return err
			}
			if err := f.checkBlockedLinks(rendered.HTML); err != nil {
				return err
			}
		}
	}

	// 2. SpamAssassin Check
	if f.cfg.Security.SpamAssassin.Enabled {
		if err := f.checkSpamAssassin(ctx, rendered); err != nil {
			return err
		}
	}

	return nil
}

func (f *contentFilter) checkSussyWords(content string) error {
	lowerContent := strings.ToLower(content)
	for _, word := range f.cfg.Security.ContentSecurity.SussyWords {
		if strings.Contains(lowerContent, strings.ToLower(word)) {
			return fmt.Errorf("content contains forbidden word: %q", word)
		}
	}
	return nil
}

func (f *contentFilter) checkBlockedLinks(content string) error {
	for _, link := range f.cfg.Security.ContentSecurity.BlockedLinks {
		if strings.Contains(content, link) {
			return fmt.Errorf("content contains blocked link: %q", link)
		}
	}
	return nil
}

func (f *contentFilter) checkSpamAssassin(ctx context.Context, rendered *domain.RenderedContent) error {
	addr := fmt.Sprintf("%s:%d", f.cfg.Security.SpamAssassin.Host, f.cfg.Security.SpamAssassin.Port)

	// Use a timeout-aware dialer that respects the context deadline
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		// Graceful degradation: if spamd is unreachable, log a warning and skip.
		// This keeps the service running when SpamAssassin isn't deployed.
		if f.log != nil {
			f.log.Warn("spamassassin: unreachable, skipping check", zap.String("addr", addr), zap.Error(err))
		} else {
			fmt.Printf("spamassassin: unreachable (%s), skipping check\n", err)
		}
		return nil
	}
	defer conn.Close()

	tp := textproto.NewConn(conn)

	// Format as a simple email for SpamAssassin
	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", rendered.Subject))
	msg.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(rendered.Body)
	if rendered.HTML != "" {
		msg.WriteString("\r\n\r\n--- HTML Content ---\r\n")
		msg.WriteString(rendered.HTML)
	}
	
	payload := msg.String()

	// SPAMC/1.5 CHECK spamd/1.5
	if err := tp.PrintfLine("CHECK SPAMC/1.5"); err != nil {
		return err
	}
	if err := tp.PrintfLine("Content-length: %d", len(payload)); err != nil {
		return err
	}
	if err := tp.PrintfLine(""); err != nil {
		return err
	}
	if _, err := conn.Write([]byte(payload)); err != nil {
		return err
	}

	// Read response
	reader := bufio.NewReader(conn)
	line, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	if !strings.Contains(line, "EX_OK") {
		return fmt.Errorf("spamassassin: unexpected response: %s", line)
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil || line == "\r\n" || line == "\n" {
			break
		}
		if strings.HasPrefix(line, "Spam: True") {
			return fmt.Errorf("spamassassin: content flagged as spam")
		}
	}

	return nil
}
