package security

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/spidey/notification-service/internal/config"
	"github.com/spidey/notification-service/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestContentFilter_StaticChecks(t *testing.T) {
	cfg := config.Config{
		Security: config.SecurityConfig{
			ContentSecurity: config.ContentSecurityConfig{
				Enabled:      true,
				SussyWords:   []string{"casino", "urgent"},
				BlockedLinks: []string{"malicious.com"},
			},
		},
	}
	filter := NewContentFilter(cfg)

	tests := []struct {
		name    string
		content *domain.RenderedContent
		wantErr bool
		errSub  string
	}{
		{
			name: "clean content",
			content: &domain.RenderedContent{
				Body: "Hello, how are you?",
			},
			wantErr: false,
		},
		{
			name: "sussy word",
			content: &domain.RenderedContent{
				Body: "Win big at our casino!",
			},
			wantErr: true,
			errSub:  "forbidden word: \"casino\"",
		},
		{
			name: "blocked link",
			content: &domain.RenderedContent{
				Body: "Click here: http://malicious.com/hack",
			},
			wantErr: true,
			errSub:  "blocked link: \"malicious.com\"",
		},
		{
			name: "sussy word in HTML",
			content: &domain.RenderedContent{
				Body: "Clean body",
				HTML: "<html>URGENT ACTION REQUIRED</html>",
			},
			wantErr: true,
			errSub:  "forbidden word: \"urgent\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := filter.CheckContent(context.Background(), tt.content)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errSub)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestContentFilter_SpamAssassin(t *testing.T) {
	// Start a mock spamd server with a connection-ready channel
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port

	cfg := config.Config{
		Security: config.SecurityConfig{
			SpamAssassin: config.SpamAssassinConfig{
				Enabled: true,
				Host:    "127.0.0.1",
				Port:    port,
			},
		},
	}
	filter := NewContentFilter(cfg)

	// Mock spamd behavior: accept one connection per request, handle synchronously
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			func() {
				defer conn.Close()
				buf := make([]byte, 2048)
				n, err := conn.Read(buf)
				if err != nil || n == 0 {
					return
				}
				request := string(buf[:n])

				if strings.Contains(request, "SPAMC/1.5") {
					if strings.Contains(request, "SPAMMY") {
						fmt.Fprint(conn, "SPAMD/1.1 0 EX_OK\r\nSpam: True ; 15.0 / 5.0\r\n\r\n")
					} else {
						fmt.Fprint(conn, "SPAMD/1.1 0 EX_OK\r\nSpam: False ; 0.1 / 5.0\r\n\r\n")
					}
				}
			}()
		}
	}()

	// Allow server to start
	time.Sleep(50 * time.Millisecond)

	t.Run("not spam", func(t *testing.T) {
		content := &domain.RenderedContent{
			Subject: "Hello",
			Body:    "This is a normal message.",
		}
		err := filter.CheckContent(context.Background(), content)
		assert.NoError(t, err)
	})

	t.Run("is spam", func(t *testing.T) {
		content := &domain.RenderedContent{
			Subject: "SPAMMY",
			Body:    "This is spam content.",
		}
		err := filter.CheckContent(context.Background(), content)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "flagged as spam")
	})
}
