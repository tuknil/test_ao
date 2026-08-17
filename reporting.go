package main

import (
	"net/http"
	"time"
)

// This file implements the Milestone 1 Reporting dashboard: Mapping,
// Measuring, Monitoring, and Kill Switch. Prompt Injection reporting is
// intentionally out of scope. Everything here is derived from the existing
// agents/policies tables and their history tables — no new tables needed.

type ReportingMapping struct {
	AgentsMapped   int `json:"agentsMapped"`
	PoliciesMapped int `json:"policiesMapped"`
}

type RiskTrendPoint struct {
	Date   string `json:"date"`
	Low    int    `json:"low"`
	Medium int    `json:"medium"`
	High   int    `json:"high"`
}

type HighRiskAgent struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	RiskScore        int    `json:"riskScore"`
	Source           string `json:"source"`
	KillSwitchAction string `json:"killSwitchAction"`
	Monitor          bool   `json:"monitor"`
}

type ReportingMeasuring struct {
	AgentsLow24h    int              `json:"agentsLow24h"`
	AgentsMedium24h int              `json:"agentsMedium24h"`
	AgentsHigh24h   int              `json:"agentsHigh24h"`
	RiskTrend       []RiskTrendPoint `json:"riskTrend"`
	HighRiskAgents  []HighRiskAgent  `json:"highRiskAgents"`
}

type TrendPoint struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

type ReportingMonitoring struct {
	AgentsMonitored24h int          `json:"agentsMonitored24h"`
	MonitoredTrend     []TrendPoint `json:"monitoredTrend"`
}

type ReportingKillSwitch struct {
	AgentsDisabled24h int          `json:"agentsDisabled24h"`
	DisabledTrend     []TrendPoint `json:"disabledTrend"`
}

type ReportingResponse struct {
	Mapping    ReportingMapping    `json:"mapping"`
	Measuring  ReportingMeasuring  `json:"measuring"`
	Monitoring ReportingMonitoring `json:"monitoring"`
	KillSwitch ReportingKillSwitch `json:"killSwitch"`
}

func getDashboardReporting(w http.ResponseWriter, r *http.Request) {
	var resp ReportingResponse

	if err := db.QueryRow(`SELECT count(*) FROM agents`).Scan(&resp.Mapping.AgentsMapped); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := db.QueryRow(`SELECT count(*) FROM policies`).Scan(&resp.Mapping.PoliciesMapped); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := db.QueryRow(`
		SELECT
			count(DISTINCT agent_id) FILTER (WHERE risk_score < 50),
			count(DISTINCT agent_id) FILTER (WHERE risk_score >= 50 AND risk_score < 70),
			count(DISTINCT agent_id) FILTER (WHERE risk_score >= 70)
		FROM agent_risk_score_history
		WHERE changed_at >= now() - interval '24 hours'
	`).Scan(&resp.Measuring.AgentsLow24h, &resp.Measuring.AgentsMedium24h, &resp.Measuring.AgentsHigh24h); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	riskTrend, err := queryRiskTrend()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp.Measuring.RiskTrend = riskTrend

	highRisk, err := queryHighRiskAgents()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp.Measuring.HighRiskAgents = highRisk

	if err := db.QueryRow(`
		SELECT count(DISTINCT agent_id) FROM agent_monitor_history
		WHERE monitor = true AND changed_at >= now() - interval '24 hours'
	`).Scan(&resp.Monitoring.AgentsMonitored24h); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	monitoredTrend, err := queryDailyDistinctAgentTrend(
		`SELECT changed_at, agent_id FROM agent_monitor_history WHERE monitor = true AND changed_at >= now() - interval '30 days'`,
		30,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp.Monitoring.MonitoredTrend = monitoredTrend

	if err := db.QueryRow(`
		SELECT count(DISTINCT agent_id) FROM agent_kill_switch_history
		WHERE action = 'deactivated' AND changed_at >= now() - interval '24 hours'
	`).Scan(&resp.KillSwitch.AgentsDisabled24h); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	disabledTrend, err := queryDailyDistinctAgentTrend(
		`SELECT changed_at, agent_id FROM agent_kill_switch_history WHERE action = 'deactivated' AND changed_at >= now() - interval '30 days'`,
		30,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp.KillSwitch.DisabledTrend = disabledTrend

	writeJSON(w, http.StatusOK, resp)
}

// queryRiskTrend returns one point per day for the last 14 days, counting
// distinct agents whose risk score fell in each bucket that day. Days with
// no events still appear, with zero counts, so the chart has no gaps.
func queryRiskTrend() ([]RiskTrendPoint, error) {
	rows, err := db.Query(`
		WITH days AS (
			SELECT generate_series(
				date_trunc('day', now()) - interval '13 days',
				date_trunc('day', now()),
				interval '1 day'
			)::date AS day
		),
		scored AS (
			SELECT
				date_trunc('day', changed_at)::date AS day,
				agent_id,
				CASE
					WHEN risk_score >= 70 THEN 'high'
					WHEN risk_score >= 50 THEN 'medium'
					ELSE 'low'
				END AS bucket
			FROM agent_risk_score_history
			WHERE changed_at >= now() - interval '14 days'
		)
		SELECT
			d.day,
			count(DISTINCT s.agent_id) FILTER (WHERE s.bucket = 'low'),
			count(DISTINCT s.agent_id) FILTER (WHERE s.bucket = 'medium'),
			count(DISTINCT s.agent_id) FILTER (WHERE s.bucket = 'high')
		FROM days d
		LEFT JOIN scored s ON s.day = d.day
		GROUP BY d.day
		ORDER BY d.day
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	points := []RiskTrendPoint{}
	for rows.Next() {
		var p RiskTrendPoint
		var day time.Time
		if err := rows.Scan(&day, &p.Low, &p.Medium, &p.High); err != nil {
			return nil, err
		}
		p.Date = day.Format("2006-01-02")
		points = append(points, p)
	}
	return points, rows.Err()
}

// queryDailyDistinctAgentTrend runs the given query (which must select
// changed_at, agent_id) and buckets the results into one point per day over
// the last `days` days, counting distinct agents per day.
func queryDailyDistinctAgentTrend(eventQuery string, days int) ([]TrendPoint, error) {
	query := `
		WITH days AS (
			SELECT generate_series(
				date_trunc('day', now()) - ($1::text || ' days')::interval,
				date_trunc('day', now()),
				interval '1 day'
			)::date AS day
		),
		events AS (
			SELECT date_trunc('day', changed_at)::date AS day, agent_id
			FROM (` + eventQuery + `) e
		)
		SELECT d.day, count(DISTINCT e.agent_id)
		FROM days d
		LEFT JOIN events e ON e.day = d.day
		GROUP BY d.day
		ORDER BY d.day
	`
	rows, err := db.Query(query, days-1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	points := []TrendPoint{}
	for rows.Next() {
		var p TrendPoint
		var day time.Time
		if err := rows.Scan(&day, &p.Count); err != nil {
			return nil, err
		}
		p.Date = day.Format("2006-01-02")
		points = append(points, p)
	}
	return points, rows.Err()
}

func queryHighRiskAgents() ([]HighRiskAgent, error) {
	rows, err := db.Query(`
		SELECT id, name, risk_score, source, kill_switch_action, monitor
		FROM agents
		ORDER BY risk_score DESC, name ASC
		LIMIT 10
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	agents := []HighRiskAgent{}
	for rows.Next() {
		var a HighRiskAgent
		if err := rows.Scan(&a.ID, &a.Name, &a.RiskScore, &a.Source, &a.KillSwitchAction, &a.Monitor); err != nil {
			return nil, err
		}
		agents = append(agents, a)
	}
	return agents, rows.Err()
}
