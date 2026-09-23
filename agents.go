package main

import (
	"context"
	"crypto/md5"
	"database/sql"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type Agent struct {
	ID               string `json:"id"`
	AgenticOverlayID string `json:"agenticOverlayId"`
	ExternalID       string `json:"externalId"`
	Name             string `json:"name"`
	Type             string `json:"type"`
	NativeType       string `json:"nativeType"`
	TechnologyName   string `json:"technologyName"`
	CloudPlatform    string `json:"cloudPlatform"`
	CloudProvider    string `json:"cloudProvider"`
	Status           string `json:"status"`
	Region           string `json:"region"`
	Projects         string `json:"projects"`
	FirstSeen        string `json:"firstSeen"`
	CreatedAt        string `json:"createdAt"`
	UpdatedAt        string `json:"updatedAt"`
	Risks            int    `json:"risks"`
	Monitor          bool   `json:"monitor"`
	Source           string `json:"source"`
	KillSwitchAction string `json:"killSwitchAction"`
	RiskScore        int    `json:"riskScore"`

	// Security-signal fields carried through from the Wiz agent inventory
	// export; not all exports populate all of these.
	HasAdminPrivileges               bool   `json:"hasAdminPrivileges"`
	HasHighPrivileges                bool   `json:"hasHighPrivileges"`
	HasAdminSaaSPrivileges           bool   `json:"hasAdminSaaSPrivileges"`
	HasHighSaaSPrivileges            bool   `json:"hasHighSaaSPrivileges"`
	HasAdminKubernetesPrivileges     bool   `json:"hasAdminKubernetesPrivileges"`
	HasHighKubernetesPrivileges      bool   `json:"hasHighKubernetesPrivileges"`
	HasAccessToSensitiveData         bool   `json:"hasAccessToSensitiveData"`
	IAMAccessFromOutsideOrg          string `json:"iamAccessFromOutsideOrg"`
	OpenToAllInternet                bool   `json:"openToAllInternet"`
	MaxExposureLevel                 string `json:"maxExposureLevel"`
	AccessibleFromInternet           bool   `json:"accessibleFromInternet"`
	AccessibleFromVPN                bool   `json:"accessibleFromVpn"`
	AccessibleFromOtherSubscriptions bool   `json:"accessibleFromOtherSubscriptions"`
	AccessibleFromOtherVnets         bool   `json:"accessibleFromOtherVnets"`
	DetectedIacPlatform              string `json:"detectedIacPlatform"`
	IacStatus                        string `json:"iacStatus"`
}

var validKillSwitchActions = map[string]bool{
	"not taken":   true,
	"deactivated": true,
	"reactivated": true,
}

func md5Hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

func migrateAgents(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS agents (
			id TEXT PRIMARY KEY,
			external_id TEXT,
			name TEXT NOT NULL,
			type TEXT,
			native_type TEXT,
			technology_name TEXT,
			cloud_platform TEXT,
			cloud_provider TEXT,
			status TEXT,
			region TEXT,
			projects TEXT,
			first_seen TEXT,
			created_at TEXT,
			updated_at TEXT
		);
		CREATE INDEX IF NOT EXISTS idx_agents_name ON agents (lower(name));
		ALTER TABLE agents ADD COLUMN IF NOT EXISTS risks INTEGER NOT NULL DEFAULT 0;
		ALTER TABLE agents ADD COLUMN IF NOT EXISTS monitor BOOLEAN NOT NULL DEFAULT false;
		ALTER TABLE agents ALTER COLUMN monitor SET DEFAULT false;
		ALTER TABLE agents ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'Wiz-Prod';
		ALTER TABLE agents ALTER COLUMN source SET DEFAULT 'Wiz-Prod';
		ALTER TABLE agents ADD COLUMN IF NOT EXISTS kill_switch_action TEXT NOT NULL DEFAULT 'not taken';
		ALTER TABLE agents ALTER COLUMN kill_switch_action SET DEFAULT 'not taken';
		ALTER TABLE agents ADD COLUMN IF NOT EXISTS risk_score INTEGER NOT NULL DEFAULT 0;
		ALTER TABLE agents ADD COLUMN IF NOT EXISTS has_admin_privileges BOOLEAN NOT NULL DEFAULT false;
		ALTER TABLE agents ADD COLUMN IF NOT EXISTS has_high_privileges BOOLEAN NOT NULL DEFAULT false;
		ALTER TABLE agents ADD COLUMN IF NOT EXISTS has_admin_saas_privileges BOOLEAN NOT NULL DEFAULT false;
		ALTER TABLE agents ADD COLUMN IF NOT EXISTS has_high_saas_privileges BOOLEAN NOT NULL DEFAULT false;
		ALTER TABLE agents ADD COLUMN IF NOT EXISTS has_admin_kubernetes_privileges BOOLEAN NOT NULL DEFAULT false;
		ALTER TABLE agents ADD COLUMN IF NOT EXISTS has_high_kubernetes_privileges BOOLEAN NOT NULL DEFAULT false;
		ALTER TABLE agents ADD COLUMN IF NOT EXISTS has_access_to_sensitive_data BOOLEAN NOT NULL DEFAULT false;
		ALTER TABLE agents ADD COLUMN IF NOT EXISTS iam_access_from_outside_org TEXT NOT NULL DEFAULT '';
		ALTER TABLE agents ADD COLUMN IF NOT EXISTS open_to_all_internet BOOLEAN NOT NULL DEFAULT false;
		ALTER TABLE agents ADD COLUMN IF NOT EXISTS max_exposure_level TEXT NOT NULL DEFAULT '';
		ALTER TABLE agents ADD COLUMN IF NOT EXISTS accessible_from_internet BOOLEAN NOT NULL DEFAULT false;
		ALTER TABLE agents ADD COLUMN IF NOT EXISTS accessible_from_vpn BOOLEAN NOT NULL DEFAULT false;
		ALTER TABLE agents ADD COLUMN IF NOT EXISTS accessible_from_other_subscriptions BOOLEAN NOT NULL DEFAULT false;
		ALTER TABLE agents ADD COLUMN IF NOT EXISTS accessible_from_other_vnets BOOLEAN NOT NULL DEFAULT false;
		ALTER TABLE agents ADD COLUMN IF NOT EXISTS detected_iac_platform TEXT NOT NULL DEFAULT '';
		ALTER TABLE agents ADD COLUMN IF NOT EXISTS iac_status TEXT NOT NULL DEFAULT '';
	`)
	return err
}

// importAgentsFromCSV wipes the agents table (and its FK-dependent history
// tables) and reloads it from the bundled CSV export every time the server
// starts, so a fresh CSV always wins over whatever was there before — any
// manual monitor/kill-switch/risk-score changes made through the API are
// discarded on every restart, not just the first one.
func importAgentsFromCSV(db *sql.DB, path string) error {
	f, err := os.Open(path)
	if err != nil {
		log.Printf("agents CSV not found at %s, skipping import: %v", path, err)
		return nil
	}
	defer f.Close()

	reader := csv.NewReader(f)
	header, err := reader.Read()
	if err != nil {
		return err
	}
	col := make(map[string]int, len(header))
	for i, h := range header {
		col[h] = i
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`TRUNCATE agent_monitor_history, agent_kill_switch_history, agent_risk_score_history, agents CASCADE`); err != nil {
		tx.Rollback()
		return err
	}
	stmt, err := tx.Prepare(`
		INSERT INTO agents (
			id, external_id, name, type, native_type, technology_name, cloud_platform, cloud_provider, status, region, projects, first_seen, created_at, updated_at, risk_score,
			has_admin_privileges, has_high_privileges, has_admin_saas_privileges, has_high_saas_privileges, has_admin_kubernetes_privileges, has_high_kubernetes_privileges, has_access_to_sensitive_data, iam_access_from_outside_org, open_to_all_internet, max_exposure_level, accessible_from_internet, accessible_from_vpn, accessible_from_other_subscriptions, accessible_from_other_vnets, detected_iac_platform, iac_status
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30, $31)
	`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()

	historyStmt, err := tx.Prepare(`INSERT INTO agent_risk_score_history (agent_id, risk_score) VALUES ($1, $2)`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer historyStmt.Close()

	// Deterministic seed so re-imports (fresh DBs) get the same distribution.
	rng := rand.New(rand.NewSource(42))

	get := func(row []string, key string) string {
		if i, ok := col[key]; ok && i < len(row) {
			if v := row[i]; v != "null" {
				return v
			}
		}
		return ""
	}
	getBool := func(row []string, key string) bool {
		return strings.EqualFold(get(row, key), "TRUE")
	}

	// This export has no dedicated "id" field (unlike the original Wiz
	// export this importer was written for); externalId is unique across
	// every row, so it doubles as the primary key. There's likewise no
	// separate cloud-provider or technology-name column, so cloud_provider
	// mirrors cloud_platform and technology_name falls back to publisher.
	imported := 0
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			tx.Rollback()
			return err
		}

		id := get(row, "externalId")
		if id == "" {
			continue
		}

		riskScore := seedRiskScoreValue(rng)
		_, err = stmt.Exec(
			id,
			id,
			get(row, "name"),
			"AI_AGENT",
			get(row, "nativeType"),
			get(row, "publisher"),
			get(row, "cloudPlatform"),
			get(row, "cloudPlatform"),
			get(row, "status"),
			get(row, "region"),
			"",
			get(row, "creationDate"),
			get(row, "creationDate"),
			get(row, "updatedAt"),
			riskScore,
			getBool(row, "hasAdminPrivileges"),
			getBool(row, "hasHighPrivileges"),
			getBool(row, "hasAdminSaaSPrivileges"),
			getBool(row, "hasHighSaaSPrivileges"),
			getBool(row, "hasAdminKubernetesPrivileges"),
			getBool(row, "hasHighKubernetesPrivileges"),
			getBool(row, "hasAccessToSensitiveData"),
			get(row, "hasIAMAccessFromOutsideOrganization"),
			getBool(row, "openToAllInternet"),
			get(row, "maxExposureLevel"),
			getBool(row, "accessibleFrom_internet"),
			getBool(row, "accessibleFrom_VPN"),
			getBool(row, "accessibleFrom_otherSubscriptions"),
			getBool(row, "accessibleFrom_otherVnets"),
			get(row, "detectedIacPlatform"),
			get(row, "iacStatus"),
		)
		if err != nil {
			tx.Rollback()
			return err
		}
		if _, err := historyStmt.Exec(id, riskScore); err != nil {
			tx.Rollback()
			return err
		}
		imported++
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	log.Printf("imported %d agents from %s", imported, path)
	return nil
}

// syncAllAgentsToRedis mirrors every agent row into Redis (one JSON key per
// agent), pipelined into a single round trip. Called on every server start
// (whether or not this boot actually imported the CSV), so a Redis that
// lost its data across restarts still gets fully repopulated from Postgres.
// Best-effort: Postgres remains the system of record and every API read
// goes through it, not Redis.
func syncAllAgentsToRedis(db *sql.DB) {
	rows, err := db.Query(`SELECT id, external_id, name, type, native_type, technology_name, cloud_platform, cloud_provider, status, region, projects, first_seen, created_at, updated_at, risks, monitor, source, kill_switch_action, risk_score, has_admin_privileges, has_high_privileges, has_admin_saas_privileges, has_high_saas_privileges, has_admin_kubernetes_privileges, has_high_kubernetes_privileges, has_access_to_sensitive_data, iam_access_from_outside_org, open_to_all_internet, max_exposure_level, accessible_from_internet, accessible_from_vpn, accessible_from_other_subscriptions, accessible_from_other_vnets, detected_iac_platform, iac_status FROM agents`)
	if err != nil {
		log.Printf("redis: failed to read agents for sync: %v", err)
		return
	}
	defer rows.Close()

	pipe := rdb.Pipeline()
	n := 0
	for rows.Next() {
		var a Agent
		if err := rows.Scan(&a.ID, &a.ExternalID, &a.Name, &a.Type, &a.NativeType, &a.TechnologyName, &a.CloudPlatform, &a.CloudProvider, &a.Status, &a.Region, &a.Projects, &a.FirstSeen, &a.CreatedAt, &a.UpdatedAt, &a.Risks, &a.Monitor, &a.Source, &a.KillSwitchAction, &a.RiskScore, &a.HasAdminPrivileges, &a.HasHighPrivileges, &a.HasAdminSaaSPrivileges, &a.HasHighSaaSPrivileges, &a.HasAdminKubernetesPrivileges, &a.HasHighKubernetesPrivileges, &a.HasAccessToSensitiveData, &a.IAMAccessFromOutsideOrg, &a.OpenToAllInternet, &a.MaxExposureLevel, &a.AccessibleFromInternet, &a.AccessibleFromVPN, &a.AccessibleFromOtherSubscriptions, &a.AccessibleFromOtherVnets, &a.DetectedIacPlatform, &a.IacStatus); err != nil {
			log.Printf("redis: failed to scan agent for sync: %v", err)
			continue
		}
		a.AgenticOverlayID = md5Hex(a.ID)
		pipelineSetJSON(pipe, agentRedisKey(a.ID), a)
		n++
	}
	if err := rows.Err(); err != nil {
		log.Printf("redis: failed to read agents for sync: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := pipe.Exec(ctx); err != nil {
		log.Printf("redis: failed to sync agents: %v", err)
		return
	}
	log.Printf("redis: synced %d agents", n)
}

// seedRiskScoreValue picks a risk score for a freshly-seeded or reseeded
// agent: ~90% score 0, ~8% score 50-60, ~2% score 70-80.
func seedRiskScoreValue(rng *rand.Rand) int {
	roll := rng.Float64()
	switch {
	case roll < 0.02:
		return 70 + rng.Intn(11)
	case roll < 0.10:
		return 50 + rng.Intn(11)
	default:
		return 0
	}
}

// reseedAgentRiskScores resets every agent's risk_score using the same
// distribution as the initial import, and rebuilds agent_risk_score_history
// from scratch to match — so the two never drift, and the Dashboard's
// risk-scoring stats/trend (which read history, not the live column) are
// consistent with what the Agents list shows immediately afterward.
func reseedAgentRiskScores(db *sql.DB) (int, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	rows, err := tx.Query(`SELECT id FROM agents ORDER BY id`)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	updateStmt, err := tx.Prepare(`UPDATE agents SET risk_score = $1 WHERE id = $2`)
	if err != nil {
		return 0, err
	}
	defer updateStmt.Close()

	rng := rand.New(rand.NewSource(42))
	for _, id := range ids {
		if _, err := updateStmt.Exec(seedRiskScoreValue(rng), id); err != nil {
			return 0, err
		}
	}

	if _, err := tx.Exec(`DELETE FROM agent_risk_score_history`); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(`INSERT INTO agent_risk_score_history (agent_id, risk_score) SELECT id, risk_score FROM agents`); err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(ids), nil
}

func reseedAgentRiskScoresHandler(w http.ResponseWriter, r *http.Request) {
	n, err := reseedAgentRiskScores(db)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	syncAllAgentsToRedis(db)
	pushMappedCountsToRedis()
	writeJSON(w, http.StatusOK, map[string]interface{}{"agentsReseeded": n})
}

// formatProjects turns the CSV's JSON array of {id, name} objects into a
// simple comma-separated list of project names for display.
func formatProjects(raw string) string {
	if raw == "" || raw == "null" {
		return ""
	}
	var items []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return ""
	}
	names := make([]string, 0, len(items))
	for _, it := range items {
		if it.Name != "" {
			names = append(names, it.Name)
		}
	}
	return strings.Join(names, ", ")
}

func listAgents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	search := strings.TrimSpace(q.Get("search"))

	limit, err := strconv.Atoi(q.Get("limit"))
	if err != nil || limit <= 0 || limit > 200 {
		limit = 50
	}
	offset, err := strconv.Atoi(q.Get("offset"))
	if err != nil || offset < 0 {
		offset = 0
	}

	whereClause := ""
	args := []interface{}{}
	if search != "" {
		whereClause = `WHERE name ILIKE $1 OR technology_name ILIKE $1 OR cloud_platform ILIKE $1 OR status ILIKE $1 OR region ILIKE $1`
		args = append(args, "%"+search+"%")
	}

	var total int
	countQuery := `SELECT count(*) FROM agents ` + whereClause
	if err := db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	query := `SELECT id, external_id, name, type, native_type, technology_name, cloud_platform, cloud_provider, status, region, projects, first_seen, created_at, updated_at, risks, monitor, source, kill_switch_action, risk_score, has_admin_privileges, has_high_privileges, has_admin_saas_privileges, has_high_saas_privileges, has_admin_kubernetes_privileges, has_high_kubernetes_privileges, has_access_to_sensitive_data, iam_access_from_outside_org, open_to_all_internet, max_exposure_level, accessible_from_internet, accessible_from_vpn, accessible_from_other_subscriptions, accessible_from_other_vnets, detected_iac_platform, iac_status
	          FROM agents ` + whereClause + `
	          ORDER BY name
	          LIMIT $` + strconv.Itoa(len(args)+1) + ` OFFSET $` + strconv.Itoa(len(args)+2)
	args = append(args, limit, offset)

	rows, err := db.Query(query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	items := []Agent{}
	for rows.Next() {
		var a Agent
		if err := rows.Scan(&a.ID, &a.ExternalID, &a.Name, &a.Type, &a.NativeType, &a.TechnologyName, &a.CloudPlatform, &a.CloudProvider, &a.Status, &a.Region, &a.Projects, &a.FirstSeen, &a.CreatedAt, &a.UpdatedAt, &a.Risks, &a.Monitor, &a.Source, &a.KillSwitchAction, &a.RiskScore, &a.HasAdminPrivileges, &a.HasHighPrivileges, &a.HasAdminSaaSPrivileges, &a.HasHighSaaSPrivileges, &a.HasAdminKubernetesPrivileges, &a.HasHighKubernetesPrivileges, &a.HasAccessToSensitiveData, &a.IAMAccessFromOutsideOrg, &a.OpenToAllInternet, &a.MaxExposureLevel, &a.AccessibleFromInternet, &a.AccessibleFromVPN, &a.AccessibleFromOtherSubscriptions, &a.AccessibleFromOtherVnets, &a.DetectedIacPlatform, &a.IacStatus); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		a.AgenticOverlayID = md5Hex(a.ID)
		items = append(items, a)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items":  items,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// updateAgentReturning runs an UPDATE against agents (the caller supplies the
// SET/WHERE clause and its args) and returns the full updated row, so
// mutating handlers can mirror the current agent state into Redis without a
// separate SELECT round trip.
func updateAgentReturning(tx *sql.Tx, query string, args ...interface{}) (Agent, error) {
	var a Agent
	err := tx.QueryRow(
		query+` RETURNING id, external_id, name, type, native_type, technology_name, cloud_platform, cloud_provider, status, region, projects, first_seen, created_at, updated_at, risks, monitor, source, kill_switch_action, risk_score, has_admin_privileges, has_high_privileges, has_admin_saas_privileges, has_high_saas_privileges, has_admin_kubernetes_privileges, has_high_kubernetes_privileges, has_access_to_sensitive_data, iam_access_from_outside_org, open_to_all_internet, max_exposure_level, accessible_from_internet, accessible_from_vpn, accessible_from_other_subscriptions, accessible_from_other_vnets, detected_iac_platform, iac_status`,
		args...,
	).Scan(&a.ID, &a.ExternalID, &a.Name, &a.Type, &a.NativeType, &a.TechnologyName, &a.CloudPlatform, &a.CloudProvider, &a.Status, &a.Region, &a.Projects, &a.FirstSeen, &a.CreatedAt, &a.UpdatedAt, &a.Risks, &a.Monitor, &a.Source, &a.KillSwitchAction, &a.RiskScore, &a.HasAdminPrivileges, &a.HasHighPrivileges, &a.HasAdminSaaSPrivileges, &a.HasHighSaaSPrivileges, &a.HasAdminKubernetesPrivileges, &a.HasHighKubernetesPrivileges, &a.HasAccessToSensitiveData, &a.IAMAccessFromOutsideOrg, &a.OpenToAllInternet, &a.MaxExposureLevel, &a.AccessibleFromInternet, &a.AccessibleFromVPN, &a.AccessibleFromOtherSubscriptions, &a.AccessibleFromOtherVnets, &a.DetectedIacPlatform, &a.IacStatus)
	if err != nil {
		return a, err
	}
	a.AgenticOverlayID = md5Hex(a.ID)
	return a, nil
}

type agentMonitorPayload struct {
	Monitor bool `json:"monitor"`
}

func updateAgentMonitor(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var payload agentMonitorPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	tx, err := db.Begin()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()

	a, err := updateAgentReturning(tx, `UPDATE agents SET monitor = $1 WHERE id = $2`, payload.Monitor, id)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := tx.Exec(
		`INSERT INTO agent_monitor_history (agent_id, monitor) VALUES ($1, $2)`,
		id, payload.Monitor,
	); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	setInventoryJSON(agentRedisKey(a.ID), a)
	pushMappedCountsToRedis()
	writeJSON(w, http.StatusOK, map[string]interface{}{"id": id, "monitor": payload.Monitor})
}

type agentKillSwitchPayload struct {
	Action string `json:"action"`
}

// updateAgentKillSwitchAction is intended for use by an external service
// (not the UI) to mark an agent's kill-switch state as one of "not taken",
// "deactivated", or "reactivated".
func updateAgentKillSwitchAction(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var payload agentKillSwitchPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !validKillSwitchActions[payload.Action] {
		writeError(w, http.StatusBadRequest, `action must be one of "not taken", "deactivated", "reactivated"`)
		return
	}

	tx, err := db.Begin()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()

	a, err := updateAgentReturning(tx, `UPDATE agents SET kill_switch_action = $1 WHERE id = $2`, payload.Action, id)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := tx.Exec(
		`INSERT INTO agent_kill_switch_history (agent_id, action) VALUES ($1, $2)`,
		id, payload.Action,
	); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	setInventoryJSON(agentRedisKey(a.ID), a)
	pushMappedCountsToRedis()
	writeJSON(w, http.StatusOK, map[string]interface{}{"id": id, "killSwitchAction": payload.Action})
}

type agentRiskScorePayload struct {
	RiskScore int `json:"riskScore"`
}

// updateAgentRiskScore is intended for use by an external service (not the
// UI) to push a computed risk score (0-100) for an agent.
func updateAgentRiskScore(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var payload agentRiskScorePayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if payload.RiskScore < 0 || payload.RiskScore > 100 {
		writeError(w, http.StatusBadRequest, "riskScore must be between 0 and 100")
		return
	}

	tx, err := db.Begin()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()

	a, err := updateAgentReturning(tx, `UPDATE agents SET risk_score = $1 WHERE id = $2`, payload.RiskScore, id)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := tx.Exec(
		`INSERT INTO agent_risk_score_history (agent_id, risk_score) VALUES ($1, $2)`,
		id, payload.RiskScore,
	); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	setInventoryJSON(agentRedisKey(a.ID), a)
	pushMappedCountsToRedis()
	writeJSON(w, http.StatusOK, map[string]interface{}{"id": id, "riskScore": payload.RiskScore})
}
