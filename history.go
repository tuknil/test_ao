package main

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"
)

func migrateHistoryTables(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS agent_monitor_history (
			id SERIAL PRIMARY KEY,
			agent_id TEXT NOT NULL REFERENCES agents(id),
			old_monitor BOOLEAN,
			new_monitor BOOLEAN NOT NULL,
			changed_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE INDEX IF NOT EXISTS idx_agent_monitor_history_agent_id ON agent_monitor_history (agent_id, changed_at DESC);

		CREATE TABLE IF NOT EXISTS agent_kill_switch_history (
			id SERIAL PRIMARY KEY,
			agent_id TEXT NOT NULL REFERENCES agents(id),
			old_action TEXT,
			new_action TEXT NOT NULL,
			changed_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE INDEX IF NOT EXISTS idx_agent_kill_switch_history_agent_id ON agent_kill_switch_history (agent_id, changed_at DESC);

		CREATE TABLE IF NOT EXISTS agent_risk_score_history (
			id SERIAL PRIMARY KEY,
			agent_id TEXT NOT NULL REFERENCES agents(id),
			old_risk_score INTEGER,
			new_risk_score INTEGER NOT NULL,
			changed_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE INDEX IF NOT EXISTS idx_agent_risk_score_history_agent_id ON agent_risk_score_history (agent_id, changed_at DESC);
	`)
	return err
}

type MonitorHistoryEntry struct {
	ID         int64     `json:"id"`
	AgentID    string    `json:"agentId"`
	OldMonitor *bool     `json:"oldMonitor"`
	NewMonitor bool      `json:"newMonitor"`
	ChangedAt  time.Time `json:"changedAt"`
}

type KillSwitchHistoryEntry struct {
	ID        int64     `json:"id"`
	AgentID   string    `json:"agentId"`
	OldAction *string   `json:"oldAction"`
	NewAction string    `json:"newAction"`
	ChangedAt time.Time `json:"changedAt"`
}

type RiskScoreHistoryEntry struct {
	ID           int64     `json:"id"`
	AgentID      string    `json:"agentId"`
	OldRiskScore *int      `json:"oldRiskScore"`
	NewRiskScore int       `json:"newRiskScore"`
	ChangedAt    time.Time `json:"changedAt"`
}

// historyLimit parses a shared `limit` query param used by the history
// endpoints (most-recent-first, bounded so a heavily-toggled agent can't
// return an unbounded response).
func historyLimit(r *http.Request) int {
	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || limit <= 0 || limit > 500 {
		return 100
	}
	return limit
}

func listAgentMonitorHistory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	limit := historyLimit(r)

	rows, err := db.Query(
		`SELECT id, agent_id, old_monitor, new_monitor, changed_at
		 FROM agent_monitor_history WHERE agent_id = $1
		 ORDER BY changed_at DESC LIMIT $2`,
		id, limit,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	items := []MonitorHistoryEntry{}
	for rows.Next() {
		var e MonitorHistoryEntry
		if err := rows.Scan(&e.ID, &e.AgentID, &e.OldMonitor, &e.NewMonitor, &e.ChangedAt); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		items = append(items, e)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": items})
}

func listAgentKillSwitchHistory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	limit := historyLimit(r)

	rows, err := db.Query(
		`SELECT id, agent_id, old_action, new_action, changed_at
		 FROM agent_kill_switch_history WHERE agent_id = $1
		 ORDER BY changed_at DESC LIMIT $2`,
		id, limit,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	items := []KillSwitchHistoryEntry{}
	for rows.Next() {
		var e KillSwitchHistoryEntry
		if err := rows.Scan(&e.ID, &e.AgentID, &e.OldAction, &e.NewAction, &e.ChangedAt); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		items = append(items, e)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": items})
}

func listAgentRiskScoreHistory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	limit := historyLimit(r)

	rows, err := db.Query(
		`SELECT id, agent_id, old_risk_score, new_risk_score, changed_at
		 FROM agent_risk_score_history WHERE agent_id = $1
		 ORDER BY changed_at DESC LIMIT $2`,
		id, limit,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	items := []RiskScoreHistoryEntry{}
	for rows.Next() {
		var e RiskScoreHistoryEntry
		if err := rows.Scan(&e.ID, &e.AgentID, &e.OldRiskScore, &e.NewRiskScore, &e.ChangedAt); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		items = append(items, e)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": items})
}
