package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	notification "github.com/spidey/notification-service/sdk/go"
)

type result struct {
	latency time.Duration
	err     error
}

func main() {
	// CLI Flags
	apiKeysStr := flag.String("api-keys", "", "Comma-separated list of API keys to use")
	channel := flag.String("channel", "email", "Notification channel (email, sms)")
	recipient := flag.String("recipient", "", "Recipient (email address or phone number)")
	subject := flag.String("subject", "Stress Test Notification", "Subject for email notifications")
	body := flag.String("body", "This is a stress test notification.", "Body of the notification")
	baseURL := flag.String("base-url", "http://localhost:8080/v1", "Base URL of the Notification Service")
	notifType := flag.String("type", "stress_test", "Notification type")
	priority := flag.String("priority", "medium", "Notification priority (high, medium, low)")
	count := flag.Int("count", 0, "Total number of notifications to send (0 for unlimited until duration)")
	concurrency := flag.Int("concurrency", 5, "Number of concurrent workers")
	duration := flag.Duration("duration", 10*time.Second, "How long to run the stress test (e.g., 30s, 1m)")
	rps := flag.Int("rps", 0, "Target requests per second (0 for no limit)")

	flag.Parse()

	if *apiKeysStr == "" {
		log.Fatal("Error: At least one API key must be provided via -api-keys")
	}
	if *recipient == "" {
		log.Fatal("Error: Recipient must be provided via -recipient")
	}

	apiKeys := strings.Split(*apiKeysStr, ",")
	for i := range apiKeys {
		apiKeys[i] = strings.TrimSpace(apiKeys[i])
	}

	fmt.Printf("🔥 Starting STRESS TEST\n")
	fmt.Printf("📍 Target: %s (%s) @ %s\n", *recipient, *channel, *baseURL)
	fmt.Printf("👥 Concurrency: %d | ⏱️ Duration: %v | 🚀 RPS Limit: %d\n", *concurrency, *duration, *rps)
	fmt.Println("--------------------------------------------------------------------------------")

	results := make(chan result, 10000)
	var wg sync.WaitGroup
	var totalSent int64
	var successCount int64
	var failCount int64

	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()

	start := time.Now()

	// Rate limiter
	var ticker *time.Ticker
	if *rps > 0 {
		ticker = time.NewTicker(time.Second / time.Duration(*rps))
		defer ticker.Stop()
	}

	// Workers
	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			
			for {
				select {
				case <-ctx.Done():
					return
				default:
					if *count > 0 && atomic.LoadInt64(&totalSent) >= int64(*count) {
						return
					}

					if ticker != nil {
						<-ticker.C
					}

					currentTotal := atomic.AddInt64(&totalSent, 1)
					if *count > 0 && currentTotal > int64(*count) {
						return
					}

					apiKey := apiKeys[int(currentTotal-1)%len(apiKeys)]
					client := notification.New(
						notification.WithBaseURL(*baseURL),
						notification.WithAPIKey(apiKey),
					)

					idempotencyKey := fmt.Sprintf("stress-%d-%d", start.Unix(), currentTotal)
					opts := &notification.NotifyOptions{
						Subject:  *subject,
						Body:     *body,
						Priority: notification.Priority(*priority),
					}

					reqStart := time.Now()
					var err error
					
					switch strings.ToLower(*channel) {
					case "email":
						_, err = client.Notifications.NotifyByEmail(ctx, "stress-user", idempotencyKey, *notifType, *recipient, opts)
					case "sms":
						_, err = client.Notifications.NotifyBySMS(ctx, "stress-user", idempotencyKey, *notifType, *recipient, opts)
					}
					
					latency := time.Since(reqStart)
					results <- result{latency: latency, err: err}

					if err != nil {
						atomic.AddInt64(&failCount, 1)
					} else {
						atomic.AddInt64(&successCount, 1)
					}

					if currentTotal%10 == 0 {
						fmt.Printf("\rProgress: %d sent... (Success: %d, Fail: %d)", currentTotal, atomic.LoadInt64(&successCount), atomic.LoadInt64(&failCount))
					}
				}
			}
		}(i)
	}

	// Metrics collector
	var allLatencies []time.Duration
	errorCounts := make(map[string]int)
	done := make(chan bool)
	go func() {
		for res := range results {
			if res.err != nil {
				errorCounts[res.err.Error()]++
			}
			allLatencies = append(allLatencies, res.latency)
		}
		done <- true
	}()

	wg.Wait()
	close(results)
	<-done

	totalTime := time.Since(start)
	
	fmt.Printf("\r\n--------------------------------------------------------------------------------\n")
	fmt.Printf("✨ TEST COMPLETE\n")
	fmt.Printf("⏱️  Total Time: %v\n", totalTime)
	fmt.Printf("📊 Total Sent: %d\n", atomic.LoadInt64(&totalSent))
	fmt.Printf("✅ Successes:  %d\n", atomic.LoadInt64(&successCount))
	fmt.Printf("❌ Failures:   %d\n", atomic.LoadInt64(&failCount))
	
	if len(allLatencies) > 0 {
		sort.Slice(allLatencies, func(i, j int) bool { return allLatencies[i] < allLatencies[j] })
		
		var sum time.Duration
		for _, l := range allLatencies {
			sum += l
		}
		
		avg := sum / time.Duration(len(allLatencies))
		p50 := allLatencies[len(allLatencies)*50/100]
		p95 := allLatencies[len(allLatencies)*95/100]
		p99 := allLatencies[len(allLatencies)*99/100]

		fmt.Printf("\n📈 Latency Statistics:\n")
		fmt.Printf("   Min: %v | Max: %v | Avg: %v\n", allLatencies[0], allLatencies[len(allLatencies)-1], avg)
		fmt.Printf("   P50: %v | P95: %v | P99: %v\n", p50, p95, p99)
		
		rpsActual := float64(atomic.LoadInt64(&totalSent)) / totalTime.Seconds()
		fmt.Printf("\n🚀 Actual Throughput: %.2f requests/sec\n", rpsActual)
	}

	if len(errorCounts) > 0 {
		fmt.Printf("\n❌ Error Breakdown:\n")
		for errMsg, count := range errorCounts {
			fmt.Printf("   [%d] %s\n", count, errMsg)
		}
	}
}
