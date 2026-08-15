package main

import (
	"crypto/md5"
	"database/sql"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
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
	`)
	return err
}

// importAgentsFromCSV loads the bundled agents CSV export into the agents
// table the first time the app runs (the table is left untouched on
// subsequent restarts so manual edits, if any, are preserved).
func importAgentsFromCSV(db *sql.DB, path string) error {
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM agents`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

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
	stmt, err := tx.Prepare(`
		INSERT INTO agents (id, external_id, name, type, native_type, technology_name, cloud_platform, cloud_provider, status, region, projects, first_seen, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (id) DO NOTHING
	`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()

	get := func(row []string, key string) string {
		if i, ok := col[key]; ok && i < len(row) {
			return row[i]
		}
		return ""
	}

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

		id := get(row, "id")
		if id == "" {
			continue
		}

		_, err = stmt.Exec(
			id,
			get(row, "externalId"),
			get(row, "name"),
			get(row, "type"),
			get(row, "nativeType"),
			get(row, "technology.name"),
			get(row, "cloudPlatform"),
			get(row, "cloudAccount.cloudProvider"),
			get(row, "status"),
			get(row, "region"),
			formatProjects(get(row, "projects")),
			get(row, "firstSeen"),
			get(row, "createdAt"),
			get(row, "updatedAt"),
		)
		if err != nil {
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

	query := `SELECT id, external_id, name, type, native_type, technology_name, cloud_platform, cloud_provider, status, region, projects, first_seen, created_at, updated_at, risks, monitor, source, kill_switch_action
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
		if err := rows.Scan(&a.ID, &a.ExternalID, &a.Name, &a.Type, &a.NativeType, &a.TechnologyName, &a.CloudPlatform, &a.CloudProvider, &a.Status, &a.Region, &a.Projects, &a.FirstSeen, &a.CreatedAt, &a.UpdatedAt, &a.Risks, &a.Monitor, &a.Source, &a.KillSwitchAction); err != nil {
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

	res, err := db.Exec(`UPDATE agents SET monitor = $1 WHERE id = $2`, payload.Monitor, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
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

	res, err := db.Exec(`UPDATE agents SET kill_switch_action = $1 WHERE id = $2`, payload.Action, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"id": id, "killSwitchAction": payload.Action})
}
