package main

import (
	"context"
	"crypto/tls"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/redis/go-redis/v9"
)

const (
	redisKeyAgentsMapped   = "agentic_overlay:agents_mapped"
	redisKeyModelsMapped   = "agentic_overlay:models_mapped"
	redisKeyPoliciesMapped = "agentic_overlay:policies_mapped"

	azureRedisHost     = "36665-eastus2-nprd-ao-redis.eastus2.redis.azure.net"
	azureRedisPort     = 10000
	azureRedisUsername = "c55d605a-491e-4505-86f4-3f73ec401a9e"
)

// newRedisClient picks a client based on REDIS_ENV: "PROD" connects to Azure
// Managed Redis using a Managed Identity token, anything else (including
// unset) connects to the local/docker-compose Redis container.
func newRedisClient() *redis.Client {
	if strings.EqualFold(os.Getenv("REDIS_ENV"), "PROD") {
		return newProdRedisClient()
	}
	return newDevRedisClient()
}

// newDevRedisClient connects using REDIS_ADDR (defaults to localhost:6379 for
// running the server outside Docker; docker-compose sets it to redis:6379).
func newDevRedisClient() *redis.Client {
	return redis.NewClient(&redis.Options{Addr: envOrDefault("REDIS_ADDR", "localhost:6379")})
}

// newProdRedisClient connects to Azure Managed Redis using an Entra ID token
// fetched via Managed Identity, matching the auth model required by Azure
// Managed Redis with Entra ID auth (no access keys).
//
// The token is fetched through CredentialsProviderContext rather than once
// at startup, so every new or recycled pool connection re-authenticates with
// a live token instead of one that may have expired long into the process's
// life. azidentity's credential caches the token internally and only makes a
// network call when it's actually close to expiry, so calling GetToken on
// every dial is cheap. ConnMaxLifetime forces connections to be recycled
// periodically, which is what actually triggers that re-authentication —
// without it, a connection opened at startup would keep using its original
// token for as long as the process runs and start failing once Azure expires
// it server-side.
//
// Best-effort at the credential-creation step: if the managed identity
// credential itself can't be constructed, this still returns a client (so
// the caller doesn't have to handle a nil client) but every command against
// it will fail and be logged by the best-effort callers.
func newProdRedisClient() *redis.Client {
	addr := fmt.Sprintf("%s:%d", azureRedisHost, azureRedisPort)

	cred, err := azidentity.NewManagedIdentityCredential(nil)
	if err != nil {
		log.Printf("redis: failed to create managed identity credential: %v", err)
		return redis.NewClient(&redis.Options{Addr: addr})
	}

	return redis.NewClient(&redis.Options{
		Addr: addr,
		CredentialsProviderContext: func(ctx context.Context) (string, string, error) {
			token, err := cred.GetToken(ctx, policy.TokenRequestOptions{
				Scopes: []string{"https://redis.azure.com/.default"},
			})
			if err != nil {
				return "", "", fmt.Errorf("redis: fetch azure ad token: %w", err)
			}
			return azureRedisUsername, token.Token, nil
		},
		ConnMaxLifetime:       30 * time.Minute,
		ConnMaxLifetimeJitter: 5 * time.Minute,
		TLSConfig:             &tls.Config{MinVersion: tls.VersionTLS12},
	})
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
