package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	redisKeyAgentsMapped   = "agentic_overlay:agents_mapped"
	redisKeyModelsMapped   = "agentic_overlay:models_mapped"
	redisKeyPoliciesMapped = "agentic_overlay:policies_mapped"
)

// newRedisClient connects using REDIS_ADDR (defaults to localhost:6379 for
// running the server outside Docker; docker-compose sets it to redis:6379).
func newRedisClient() *redis.Client {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	return redis.NewClient(&redis.Options{Addr: addr})
}

// pushMappedCountsToRedis computes the current agents/models/policies counts
// from Postgres and writes them into Redis. Best-effort: Redis is a
// supplementary cache here, not the system of record, so a failure here logs
// and returns rather than taking down the whole app.
func pushMappedCountsToRedis(db *sql.DB, rdb *redis.Client) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	counts := map[string]string{
		redisKeyAgentsMapped:   "SELECT count(*) FROM agents",
		redisKeyModelsMapped:   "SELECT count(*) FROM models",
		redisKeyPoliciesMapped: "SELECT count(*) FROM policies",
	}

	for key, query := range counts {
		var n int
		if err := db.QueryRow(query).Scan(&n); err != nil {
			log.Printf("redis: failed to compute %s: %v", key, err)
			continue
		}
		if err := rdb.Set(ctx, key, n, 0).Err(); err != nil {
			log.Printf("redis: failed to push %s: %v", key, err)
			continue
		}
		log.Printf("redis: %s = %d", key, n)
	}
}
