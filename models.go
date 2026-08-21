package main

import (
	"database/sql"
	"encoding/csv"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
)

type Model struct {
	ID             string `json:"id"`
	ExternalID     string `json:"externalId"`
	Name           string `json:"name"`
	Type           string `json:"type"`
	NativeType     string `json:"nativeType"`
	TechnologyName string `json:"technologyName"`
	CloudPlatform  string `json:"cloudPlatform"`
	CloudProvider  string `json:"cloudProvider"`
	Status         string `json:"status"`
	Region         string `json:"region"`
	Projects       string `json:"projects"`
	FirstSeen      string `json:"firstSeen"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
}

func migrateModels(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS models (
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
		CREATE INDEX IF NOT EXISTS idx_models_name ON models (lower(name));
	`)
	return err
}

// importModelsFromCSV loads the bundled models CSV export into the models
// table the first time the app runs (the table is left untouched on
// subsequent restarts so manual edits, if any, are preserved).
func importModelsFromCSV(db *sql.DB, path string) error {
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM models`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	f, err := os.Open(path)
	if err != nil {
		log.Printf("models CSV not found at %s, skipping import: %v", path, err)
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
		INSERT INTO models (id, external_id, name, type, native_type, technology_name, cloud_platform, cloud_provider, status, region, projects, first_seen, created_at, updated_at)
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
	log.Printf("imported %d models from %s", imported, path)
	return nil
}

func listModels(w http.ResponseWriter, r *http.Request) {
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
	countQuery := `SELECT count(*) FROM models ` + whereClause
	if err := db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	query := `SELECT id, external_id, name, type, native_type, technology_name, cloud_platform, cloud_provider, status, region, projects, first_seen, created_at, updated_at
	          FROM models ` + whereClause + `
	          ORDER BY name
	          LIMIT $` + strconv.Itoa(len(args)+1) + ` OFFSET $` + strconv.Itoa(len(args)+2)
	args = append(args, limit, offset)

	rows, err := db.Query(query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	items := []Model{}
	for rows.Next() {
		var m Model
		if err := rows.Scan(&m.ID, &m.ExternalID, &m.Name, &m.Type, &m.NativeType, &m.TechnologyName, &m.CloudPlatform, &m.CloudProvider, &m.Status, &m.Region, &m.Projects, &m.FirstSeen, &m.CreatedAt, &m.UpdatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		items = append(items, m)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items":  items,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}
