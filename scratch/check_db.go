package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	dsn := os.Getenv("NS_DATABASE_DSN")
	if dsn == "" {
		dsn = "postgres://notif:notif@localhost:5432/notifdb?sslmode=disable"
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	fmt.Println("--- Checking notifications ---")
	rows, _ := pool.Query(ctx, "SELECT id, channel, status FROM notifications ORDER BY created_at DESC LIMIT 5")
	for rows.Next() {
		var id, channel, status string
		rows.Scan(&id, &channel, &status)
		fmt.Printf("Notif: %s | %s | %s\n", id, channel, status)
	}

	fmt.Println("\n--- Checking notification_attempts ---")
	rows, _ = pool.Query(ctx, "SELECT notification_id, status, latency_ms FROM notification_attempts ORDER BY created_at DESC LIMIT 5")
	for rows.Next() {
		var nid, status string
		var latency *int
		rows.Scan(&nid, &status, &latency)
		latVal := -1
		if latency != nil {
			latVal = *latency
		}
		fmt.Printf("Attempt: %s | %s | Latency: %dms\n", nid, status, latVal)
	}
}
