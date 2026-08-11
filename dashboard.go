package main

import "net/http"

type DashboardStats struct {
	AgentsTotal     int `json:"agentsTotal"`
	AgentsMonitored int `json:"agentsMonitored"`
	PoliciesTotal   int `json:"policiesTotal"`
	PoliciesEnabled int `json:"policiesEnabled"`
}

func getDashboardStats(w http.ResponseWriter, r *http.Request) {
	var stats DashboardStats

	if err := db.QueryRow(`SELECT count(*) FROM agents`).Scan(&stats.AgentsTotal); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := db.QueryRow(`SELECT count(*) FROM agents WHERE monitor = true`).Scan(&stats.AgentsMonitored); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := db.QueryRow(`SELECT count(*) FROM policies`).Scan(&stats.PoliciesTotal); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := db.QueryRow(`SELECT count(*) FROM policies WHERE enabled = true`).Scan(&stats.PoliciesEnabled); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, stats)
}
