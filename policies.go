package main

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
)

type Policy struct {
	ID            string `json:"id"`
	PolicyID      string `json:"policyId"`
	Name          string `json:"name"`
	PolicyType    string `json:"policyType"`
	UpdateType    string `json:"updateType"`
	Severity      string `json:"severity"`
	CloudPlatform string `json:"cloudPlatform"`
	ReleasedAt    string `json:"releasedAt"`
	ApplyDate     string `json:"applyDate"`
	RegoPolicy    string `json:"regoPolicy"`
	Enabled       bool   `json:"enabled"`
}

func migratePolicies(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS policies (
			id TEXT PRIMARY KEY,
			policy_id TEXT,
			name TEXT NOT NULL,
			policy_type TEXT,
			update_type TEXT,
			severity TEXT,
			cloud_platform TEXT,
			released_at TEXT,
			apply_date TEXT
		);
		CREATE INDEX IF NOT EXISTS idx_policies_name ON policies (lower(name));
		ALTER TABLE policies ADD COLUMN IF NOT EXISTS enabled BOOLEAN NOT NULL DEFAULT false;
		ALTER TABLE policies ALTER COLUMN enabled SET DEFAULT false;
	`)
	return err
}

// importPoliciesFromCSV loads the bundled policy updates CSV export into the
// policies table the first time the app runs.
func importPoliciesFromCSV(db *sql.DB, path string) error {
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM policies`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	f, err := os.Open(path)
	if err != nil {
		log.Printf("policies CSV not found at %s, skipping import: %v", path, err)
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
		INSERT INTO policies (id, policy_id, name, policy_type, update_type, severity, cloud_platform, released_at, apply_date)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
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

		id := get(row, "Policy Version ID")
		if id == "" {
			continue
		}

		_, err = stmt.Exec(
			id,
			get(row, "Policy ID"),
			get(row, "Policy Name"),
			get(row, "Policy Type"),
			get(row, "Update Type"),
			get(row, "Severity"),
			get(row, "Cloud Platform"),
			get(row, "Released At"),
			get(row, "Apply Date"),
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
	log.Printf("imported %d policies from %s", imported, path)
	return nil
}

var regoPackageSanitizer = regexp.MustCompile(`[^a-z0-9_]+`)

// regoRuleFragment pairs a keyword to look for in a policy name with the
// deny-rule body it implies.
type regoRuleFragment struct {
	keyword string
	body    string
}

var regoRuleFragments = []regoRuleFragment{
	{"publicly exposed", `	input.resource.public_access == true
	msg := sprintf("%%s is publicly accessible and violates policy %%s", [input.resource.name, %q])`},
	{"internet-facing", `	input.resource.network.internet_facing == true
	msg := sprintf("%%s is internet-facing and violates policy %%s", [input.resource.name, %q])`},
	{"initial access vulnerabilit", `	input.resource.vulnerabilities[_].initial_access == true
	msg := sprintf("%%s has an unpatched initial-access vulnerability, violating policy %%s", [input.resource.name, %q])`},
	{"highly privileged", `	input.resource.iam.privilege_level == "high"
	msg := sprintf("%%s holds excessive IAM privileges, violating policy %%s", [input.resource.name, %q])`},
	{"sensitive data", `	input.resource.data_classification == "sensitive"
	not input.resource.encryption.enabled
	msg := sprintf("%%s stores sensitive data without encryption, violating policy %%s", [input.resource.name, %q])`},
	{"ai agent", `	input.resource.type == "ai_agent"
	input.resource.tools[_].network_access == "public"
	msg := sprintf("AI agent %%s can invoke a publicly reachable tool, violating policy %%s", [input.resource.name, %q])`},
	{"ai training", `	input.resource.purpose == "ai_training"
	input.resource.storage.public_access == true
	msg := sprintf("Training data store %%s is publicly accessible, violating policy %%s", [input.resource.name, %q])`},
	{"mlops", `	input.resource.category == "mlops"
	input.resource.network.internet_facing == true
	msg := sprintf("MLOps resource %%s is internet-facing, violating policy %%s", [input.resource.name, %q])`},
	{"bucket", `	input.resource.type == "storage_bucket"
	input.resource.acl == "public"
	msg := sprintf("Bucket %%s grants public access, violating policy %%s", [input.resource.name, %q])`},
}

// generateRegoPolicy produces a representative (not authoritative) Rego
// snippet for a policy entry, derived from its name/type/severity/cloud
// fields. The source CSV has no actual Rego source, so this approximates
// what a matching OPA rule might look like for illustration purposes.
func generateRegoPolicy(p Policy) string {
	pkgSuffix := regoPackageSanitizer.ReplaceAllString(strings.ToLower(strings.ReplaceAll(p.PolicyID, "-", "_")), "_")
	if pkgSuffix == "" {
		pkgSuffix = "policy"
	}

	ruleName := "deny"
	if p.Severity == "LOW" || p.Severity == "INFORMATIONAL" {
		ruleName = "warn"
	}

	cloud := p.CloudPlatform
	if cloud == "" {
		cloud = "multi-cloud"
	}

	nameLower := strings.ToLower(p.Name)
	var bodies []string
	for _, frag := range regoRuleFragments {
		if strings.Contains(nameLower, frag.keyword) {
			bodies = append(bodies, fmt.Sprintf(frag.body, p.PolicyID))
		}
	}
	if len(bodies) == 0 {
		bodies = append(bodies, fmt.Sprintf(`	input.resource.compliance_status != "pass"
	msg := sprintf("%%s fails control %%s: %%s", [input.resource.name, %q, %q])`, p.PolicyID, p.Name))
	}

	var b strings.Builder
	fmt.Fprintf(&b, "package wiz.controls.%s\n\n", pkgSuffix)
	fmt.Fprintf(&b, "# %s\n", p.Name)
	fmt.Fprintf(&b, "# policy_id: %s\n", p.PolicyID)
	fmt.Fprintf(&b, "# type: %s | update: %s | severity: %s | cloud: %s\n\n", p.PolicyType, p.UpdateType, p.Severity, cloud)
	b.WriteString("import future.keywords.in\n\n")
	for i, body := range bodies {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "%s[msg] {\n%s\n}\n", ruleName, body)
	}

	return b.String()
}

func listPolicies(w http.ResponseWriter, r *http.Request) {
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
		whereClause = `WHERE name ILIKE $1 OR policy_id ILIKE $1 OR policy_type ILIKE $1 OR update_type ILIKE $1 OR severity ILIKE $1 OR cloud_platform ILIKE $1`
		args = append(args, "%"+search+"%")
	}

	var total int
	countQuery := `SELECT count(*) FROM policies ` + whereClause
	if err := db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	query := `SELECT id, policy_id, name, policy_type, update_type, severity, cloud_platform, released_at, apply_date, enabled
	          FROM policies ` + whereClause + `
	          ORDER BY released_at DESC
	          LIMIT $` + strconv.Itoa(len(args)+1) + ` OFFSET $` + strconv.Itoa(len(args)+2)
	args = append(args, limit, offset)

	rows, err := db.Query(query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	items := []Policy{}
	for rows.Next() {
		var p Policy
		if err := rows.Scan(&p.ID, &p.PolicyID, &p.Name, &p.PolicyType, &p.UpdateType, &p.Severity, &p.CloudPlatform, &p.ReleasedAt, &p.ApplyDate, &p.Enabled); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		p.RegoPolicy = generateRegoPolicy(p)
		items = append(items, p)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items":  items,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

type policyEnabledPayload struct {
	Enabled bool `json:"enabled"`
}

func updatePolicyEnabled(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var payload policyEnabledPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	res, err := db.Exec(`UPDATE policies SET enabled = $1 WHERE id = $2`, payload.Enabled, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"id": id, "enabled": payload.Enabled})
}
