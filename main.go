package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	_ "github.com/lib/pq"
)

type WizIntegration struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	Type         string    `json:"type"`
	BaseURL      string    `json:"baseUrl"`
	ClientID     string    `json:"clientId"`
	ClientSecret string    `json:"clientSecret,omitempty"`
	HasSecret    bool      `json:"hasSecret"`
	McpServer    string    `json:"mcpServer"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

var db *sql.DB

func main() {
	pg := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
		Username("postgres").
		Password("postgres").
		Database("wizworkspace").
		Port(5433).
		Logger(nil))

	log.Println("starting embedded postgres...")
	if err := pg.Start(); err != nil {
		log.Fatalf("failed to start embedded postgres: %v", err)
	}
	defer func() {
		log.Println("stopping embedded postgres...")
		_ = pg.Stop()
	}()

	var err error
	db, err = sql.Open("postgres", "host=localhost port=5433 user=postgres password=postgres dbname=wizworkspace sslmode=disable")
	if err != nil {
		log.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	if err := migrate(db); err != nil {
		log.Fatalf("failed to migrate: %v", err)
	}
	if err := migrateAgents(db); err != nil {
		log.Fatalf("failed to migrate agents: %v", err)
	}
	if err := migrateHistoryTables(db); err != nil {
		log.Fatalf("failed to migrate history tables: %v", err)
	}
	if err := importAgentsFromCSV(db, "./data/agents.csv"); err != nil {
		log.Fatalf("failed to import agents: %v", err)
	}
	if err := migratePolicies(db); err != nil {
		log.Fatalf("failed to migrate policies: %v", err)
	}
	if err := importPoliciesFromCSV(db, "./data/policies.csv"); err != nil {
		log.Fatalf("failed to import policies: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/wiz-integrations", listWizIntegrations)
	mux.HandleFunc("POST /api/wiz-integrations", createWizIntegration)
	mux.HandleFunc("GET /api/wiz-integrations/{id}", getWizIntegration)
	mux.HandleFunc("PUT /api/wiz-integrations/{id}", updateWizIntegration)
	mux.HandleFunc("DELETE /api/wiz-integrations/{id}", deleteWizIntegration)
	mux.HandleFunc("GET /api/agents", listAgents)
	mux.HandleFunc("PATCH /api/agents/{id}/monitor", updateAgentMonitor)
	mux.HandleFunc("PATCH /api/agents/{id}/kill-switch-action", updateAgentKillSwitchAction)
	mux.HandleFunc("PATCH /api/agents/{id}/risk-score", updateAgentRiskScore)
	mux.HandleFunc("GET /api/agents/{id}/monitor-history", listAgentMonitorHistory)
	mux.HandleFunc("GET /api/agents/{id}/kill-switch-history", listAgentKillSwitchHistory)
	mux.HandleFunc("GET /api/agents/{id}/risk-score-history", listAgentRiskScoreHistory)
	mux.HandleFunc("GET /api/policies", listPolicies)
	mux.HandleFunc("PATCH /api/policies/{id}/enabled", updatePolicyEnabled)
	mux.HandleFunc("GET /api/dashboard/stats", getDashboardStats)
	mux.HandleFunc("GET /api/dashboard/reporting", getDashboardReporting)
	mux.Handle("/", noCacheStatic(http.FileServer(http.Dir("./web"))))

	addr := ":8080"
	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS wiz_integrations (
			id SERIAL PRIMARY KEY,
			base_url TEXT NOT NULL,
			client_id TEXT NOT NULL,
			client_secret TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		ALTER TABLE wiz_integrations ADD COLUMN IF NOT EXISTS name TEXT NOT NULL DEFAULT '';
		ALTER TABLE wiz_integrations ADD COLUMN IF NOT EXISTS type TEXT NOT NULL DEFAULT 'Wiz';
		ALTER TABLE wiz_integrations ADD COLUMN IF NOT EXISTS mcp_server TEXT NOT NULL DEFAULT '';
	`)
	return err
}

// noCacheStatic forces the browser to revalidate static assets on every
// request instead of relying on heuristic caching, which otherwise serves
// stale JS/CSS during local development.
func noCacheStatic(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		h.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func scanWizIntegration(row interface {
	Scan(dest ...interface{}) error
}) (WizIntegration, error) {
	var w WizIntegration
	var secret string
	err := row.Scan(&w.ID, &w.Name, &w.Type, &w.BaseURL, &w.ClientID, &secret, &w.McpServer, &w.CreatedAt, &w.UpdatedAt)
	if err != nil {
		return w, err
	}
	w.HasSecret = secret != ""
	return w, nil
}

func listWizIntegrations(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`SELECT id, name, type, base_url, client_id, client_secret, mcp_server, created_at, updated_at FROM wiz_integrations ORDER BY id`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	results := []WizIntegration{}
	for rows.Next() {
		wi, err := scanWizIntegration(rows)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		results = append(results, wi)
	}
	writeJSON(w, http.StatusOK, results)
}

func getWizIntegration(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	row := db.QueryRow(`SELECT id, name, type, base_url, client_id, client_secret, mcp_server, created_at, updated_at FROM wiz_integrations WHERE id = $1`, id)
	wi, err := scanWizIntegration(row)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, wi)
}

type wizPayload struct {
	Name         string `json:"name"`
	Type         string `json:"type"`
	BaseURL      string `json:"baseUrl"`
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
	McpServer    string `json:"mcpServer"`
}

func createWizIntegration(w http.ResponseWriter, r *http.Request) {
	var payload wizPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if payload.Name == "" || payload.BaseURL == "" || payload.ClientID == "" || payload.ClientSecret == "" {
		writeError(w, http.StatusBadRequest, "name, baseUrl, clientId and clientSecret are required")
		return
	}
	if payload.Type == "" {
		payload.Type = "Wiz"
	}

	row := db.QueryRow(
		`INSERT INTO wiz_integrations (name, type, base_url, client_id, client_secret, mcp_server) VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, name, type, base_url, client_id, client_secret, mcp_server, created_at, updated_at`,
		payload.Name, payload.Type, payload.BaseURL, payload.ClientID, payload.ClientSecret, payload.McpServer,
	)
	wi, err := scanWizIntegration(row)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, wi)
}

func updateWizIntegration(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	var payload wizPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if payload.Name == "" || payload.BaseURL == "" || payload.ClientID == "" {
		writeError(w, http.StatusBadRequest, "name, baseUrl and clientId are required")
		return
	}

	var row *sql.Row
	if payload.ClientSecret != "" {
		row = db.QueryRow(
			`UPDATE wiz_integrations SET name = $1, base_url = $2, client_id = $3, client_secret = $4, mcp_server = $5, updated_at = now()
			 WHERE id = $6
			 RETURNING id, name, type, base_url, client_id, client_secret, mcp_server, created_at, updated_at`,
			payload.Name, payload.BaseURL, payload.ClientID, payload.ClientSecret, payload.McpServer, id,
		)
	} else {
		row = db.QueryRow(
			`UPDATE wiz_integrations SET name = $1, base_url = $2, client_id = $3, mcp_server = $4, updated_at = now()
			 WHERE id = $5
			 RETURNING id, name, type, base_url, client_id, client_secret, mcp_server, created_at, updated_at`,
			payload.Name, payload.BaseURL, payload.ClientID, payload.McpServer, id,
		)
	}

	wi, err := scanWizIntegration(row)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, wi)
}

func deleteWizIntegration(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	res, err := db.Exec(`DELETE FROM wiz_integrations WHERE id = $1`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
