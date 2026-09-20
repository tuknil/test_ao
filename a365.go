package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/databricks/databricks-sql-go"
)

// A365Agent mirrors a Microsoft 365 Copilot agent inventory row sourced from
// a Databricks table (see importA365AgentsFromDatabricks). Field names/order
// follow the source table's own column list.
type A365Agent struct {
	TitleID                           string `json:"titleId"`
	Name                              string `json:"name"`
	Status                            string `json:"status"`
	Channel                           string `json:"channel"`
	DateCreated                       string `json:"dateCreated"`
	LastModified                      string `json:"lastModified"`
	Publisher                         string `json:"publisher"`
	PublisherType                     string `json:"publisherType"`
	Version                           string `json:"version"`
	Owner                             string `json:"owner"`
	Description                       string `json:"description"`
	Platform                          string `json:"platform"`
	CreatorID                         string `json:"creatorId"`
	EnvironmentID                     string `json:"environmentId"`
	BotID                             string `json:"botId"`
	HasCustomActions                  bool   `json:"hasCustomActions"`
	CustomActionList                  string `json:"customActionList"`
	Sensitivity                       string `json:"sensitivity"`
	CanReadOneDriveAndSharepointItems bool   `json:"canReadOneDriveAndSharepointItems"`
	OneDriveAndSharepointItems        string `json:"oneDriveAndSharepointItems"`
	CanReadOneDriveFiles              bool   `json:"canReadOneDriveFiles"`
	OneDriveFiles                     string `json:"oneDriveFiles"`
	OneDriveSites                     string `json:"oneDriveSites"`
	CanReadSharepointSitesAndFiles    bool   `json:"canReadSharepointSitesAndFiles"`
	SharepointFiles                   string `json:"sharepointFiles"`
	SharepointSites                   string `json:"sharepointSites"`
	CanExtendToGraphConnector         bool   `json:"canExtendToGraphConnector"`
	GraphConnectorDetails             string `json:"graphConnectorDetails"`
	CanGenerateImages                 bool   `json:"canGenerateImages"`
	CanUseCodeInterpreter             bool   `json:"canUseCodeInterpreter"`
	ContainsUploadedFiles             bool   `json:"containsUploadedFiles"`
	UploadedFiles                     string `json:"uploadedFiles"`
	Instructions                      string `json:"instructions"`
	GroupsShared                      string `json:"groupsShared"`
	UsersShared                       string `json:"usersShared"`
	Deployment                        string `json:"deployment"`
	RunTime                           string `json:"runTime"`
	Risks                             int    `json:"risks"`
	ActiveUsers                       int    `json:"activeUsers"`
	TotalSessions                     int    `json:"totalSessions"`
	ExceptionRate                     string `json:"exceptionRate"`
	Tags                              string `json:"tags"`
	LastUsed                          string `json:"lastUsed"`
}

const redisKeyA365AgentsMapped = "agentic_overlay:a365_agents_mapped"

// databricksConfigured reports whether all four env vars needed to reach the
// A365 agent inventory table are set. The whole A365 code path is a no-op
// when they aren't — no error, just nothing to import.
func databricksConfigured() bool {
	return os.Getenv("DATABRICKS_DSN") != "" &&
		os.Getenv("DATABRICKS_CATALOG_A365") != "" &&
		os.Getenv("DATABRICKS_SCHEMA_A365") != "" &&
		os.Getenv("DATABRICKS_TABLE_A365") != ""
}

func migrateA365Agents(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS a365_agents (
			title_id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			status TEXT,
			channel TEXT,
			date_created TEXT,
			last_modified TEXT,
			publisher TEXT,
			publisher_type TEXT,
			version TEXT,
			owner TEXT,
			description TEXT,
			platform TEXT,
			creator_id TEXT,
			environment_id TEXT,
			bot_id TEXT,
			has_custom_actions BOOLEAN NOT NULL DEFAULT false,
			custom_action_list TEXT,
			sensitivity TEXT,
			can_read_onedrive_and_sharepoint_items BOOLEAN NOT NULL DEFAULT false,
			onedrive_and_sharepoint_items TEXT,
			can_read_onedrive_files BOOLEAN NOT NULL DEFAULT false,
			onedrive_files TEXT,
			onedrive_sites TEXT,
			can_read_sharepoint_sites_and_files BOOLEAN NOT NULL DEFAULT false,
			sharepoint_files TEXT,
			sharepoint_sites TEXT,
			can_extend_to_graph_connector BOOLEAN NOT NULL DEFAULT false,
			graph_connector_details TEXT,
			can_generate_images BOOLEAN NOT NULL DEFAULT false,
			can_use_code_interpreter BOOLEAN NOT NULL DEFAULT false,
			contains_uploaded_files BOOLEAN NOT NULL DEFAULT false,
			uploaded_files TEXT,
			instructions TEXT,
			groups_shared TEXT,
			users_shared TEXT,
			deployment TEXT,
			run_time TEXT,
			risks INTEGER NOT NULL DEFAULT 0,
			active_users INTEGER NOT NULL DEFAULT 0,
			total_sessions INTEGER NOT NULL DEFAULT 0,
			exception_rate TEXT,
			tags TEXT,
			last_used TEXT
		);
		CREATE INDEX IF NOT EXISTS idx_a365_agents_name ON a365_agents (lower(name));
	`)
	return err
}

// migrateKPIs creates the generic key/value KPI table. It's currently only
// used for a365_agents_mapped, but is a plain KV store rather than an
// A365-specific table in case other standalone KPIs need the same
// Postgres-plus-Redis persistence later.
func migrateKPIs(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS kpis (
			key TEXT PRIMARY KEY,
			value INTEGER NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
	`)
	return err
}

// a365SourceColumns lists the Databricks table's column names in the exact
// order importA365AgentsFromDatabricks selects them. The source uses spaces
// in several identifiers, so each is backtick-quoted for Spark SQL.
var a365SourceColumns = []string{
	"Name", "Status", "Channel", "Date created", "Last Modified", "Publisher",
	"Publisher Type", "Version", "Owner", "Description", "Platform",
	"Creator Id", "Environment Id", "Bot Id", "Custom actions",
	"Custom action list", "Title ID", "Sensitivity",
	"Can read OneDrive and Sharepoint items", "OneDrive and Sharepoint items",
	"Can read OneDrive files", "OneDrive files", "OneDrive sites",
	"Can read Sharepoint sites and files", "Sharepoint files", "Sharepoint sites",
	"Can extend to Graph connector", "Graph connector details",
	"Can generate images using user prompt", "Can use code interpreter",
	"Contains uploaded files", "Uploaded files", "Instructions",
	"Groups shared", "Users shared", "Deployment", "Run Time", "Risks",
	"Active Users", "Total sessions", "Exception rate", "Tags", "Last used",
}

// dbxToString converts a value returned by the Databricks driver (whose Go
// type depends on the source Spark SQL column type — string, []byte, bool,
// an integer kind, or time.Time) into a display string.
func dbxToString(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []byte:
		return string(t)
	case time.Time:
		return t.Format(time.RFC3339)
	default:
		return fmt.Sprintf("%v", t)
	}
}

// dbxToBool treats a Databricks BOOLEAN column, or a STRING column using
// "Yes"/"true" as its truthy marker (as this source table's flag columns
// do), as a boolean.
func dbxToBool(v interface{}) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.EqualFold(t, "true") || strings.EqualFold(t, "yes")
	case []byte:
		s := string(t)
		return strings.EqualFold(s, "true") || strings.EqualFold(s, "yes")
	default:
		return false
	}
}

// dbxToInt converts any of the numeric-ish Go types the Databricks driver
// might return for an INT/BIGINT/DECIMAL column into an int.
func dbxToInt(v interface{}) int {
	switch t := v.(type) {
	case int64:
		return int(t)
	case int32:
		return int(t)
	case int:
		return t
	case float64:
		return int(t)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(t))
		return n
	case []byte:
		n, _ := strconv.Atoi(strings.TrimSpace(string(t)))
		return n
	default:
		return 0
	}
}

// importA365AgentsFromDatabricks wipes and reloads the a365_agents table
// from the Databricks table named by DATABRICKS_CATALOG_A365/
// DATABRICKS_SCHEMA_A365/DATABRICKS_TABLE_A365, every time the server
// starts — same wipe-and-reseed approach as the CSV-backed inventory
// tables. A no-op (not an error) when Databricks isn't configured.
// Databricks is only ever read from here; every API read goes through
// Postgres, matching how Redis is used elsewhere in this app.
func importA365AgentsFromDatabricks(pgDB *sql.DB) error {
	if !databricksConfigured() {
		return nil
	}

	dsn := os.Getenv("DATABRICKS_DSN")
	catalog := os.Getenv("DATABRICKS_CATALOG_A365")
	schema := os.Getenv("DATABRICKS_SCHEMA_A365")
	table := os.Getenv("DATABRICKS_TABLE_A365")

	dbxDB, err := sql.Open("databricks", dsn)
	if err != nil {
		return fmt.Errorf("databricks: failed to open connection: %w", err)
	}
	defer dbxDB.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	quoted := make([]string, len(a365SourceColumns))
	for i, c := range a365SourceColumns {
		quoted[i] = "`" + c + "`"
	}
	query := fmt.Sprintf(
		"SELECT %s FROM `%s`.`%s`.`%s`",
		strings.Join(quoted, ", "), catalog, schema, table,
	)

	rows, err := dbxDB.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("databricks: query failed: %w", err)
	}
	defer rows.Close()

	colIndex := make(map[string]int, len(a365SourceColumns))
	for i, c := range a365SourceColumns {
		colIndex[c] = i
	}

	tx, err := pgDB.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`TRUNCATE a365_agents`); err != nil {
		tx.Rollback()
		return err
	}
	stmt, err := tx.Prepare(`
		INSERT INTO a365_agents (
			title_id, name, status, channel, date_created, last_modified, publisher, publisher_type, version, owner, description, platform, creator_id, environment_id, bot_id,
			has_custom_actions, custom_action_list, sensitivity,
			can_read_onedrive_and_sharepoint_items, onedrive_and_sharepoint_items, can_read_onedrive_files, onedrive_files, onedrive_sites, can_read_sharepoint_sites_and_files, sharepoint_files, sharepoint_sites,
			can_extend_to_graph_connector, graph_connector_details, can_generate_images, can_use_code_interpreter, contains_uploaded_files, uploaded_files,
			instructions, groups_shared, users_shared, deployment, run_time, risks, active_users, total_sessions, exception_rate, tags, last_used
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15,
			$16, $17, $18,
			$19, $20, $21, $22, $23, $24, $25, $26,
			$27, $28, $29, $30, $31, $32,
			$33, $34, $35, $36, $37, $38, $39, $40, $41, $42, $43
		)
	`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()

	raw := make([]interface{}, len(a365SourceColumns))
	ptrs := make([]interface{}, len(raw))
	for i := range raw {
		ptrs[i] = &raw[i]
	}
	get := func(col string) interface{} { return raw[colIndex[col]] }

	imported := 0
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			tx.Rollback()
			return fmt.Errorf("databricks: scan failed: %w", err)
		}

		_, err = stmt.Exec(
			dbxToString(get("Title ID")),
			dbxToString(get("Name")),
			dbxToString(get("Status")),
			dbxToString(get("Channel")),
			dbxToString(get("Date created")),
			dbxToString(get("Last Modified")),
			dbxToString(get("Publisher")),
			dbxToString(get("Publisher Type")),
			dbxToString(get("Version")),
			dbxToString(get("Owner")),
			dbxToString(get("Description")),
			dbxToString(get("Platform")),
			dbxToString(get("Creator Id")),
			dbxToString(get("Environment Id")),
			dbxToString(get("Bot Id")),
			dbxToBool(get("Custom actions")),
			dbxToString(get("Custom action list")),
			dbxToString(get("Sensitivity")),
			dbxToBool(get("Can read OneDrive and Sharepoint items")),
			dbxToString(get("OneDrive and Sharepoint items")),
			dbxToBool(get("Can read OneDrive files")),
			dbxToString(get("OneDrive files")),
			dbxToString(get("OneDrive sites")),
			dbxToBool(get("Can read Sharepoint sites and files")),
			dbxToString(get("Sharepoint files")),
			dbxToString(get("Sharepoint sites")),
			dbxToBool(get("Can extend to Graph connector")),
			dbxToString(get("Graph connector details")),
			dbxToBool(get("Can generate images using user prompt")),
			dbxToBool(get("Can use code interpreter")),
			dbxToBool(get("Contains uploaded files")),
			dbxToString(get("Uploaded files")),
			dbxToString(get("Instructions")),
			dbxToString(get("Groups shared")),
			dbxToString(get("Users shared")),
			dbxToString(get("Deployment")),
			dbxToString(get("Run Time")),
			dbxToInt(get("Risks")),
			dbxToInt(get("Active Users")),
			dbxToInt(get("Total sessions")),
			dbxToString(get("Exception rate")),
			dbxToString(get("Tags")),
			dbxToString(get("Last used")),
		)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("databricks: insert failed: %w", err)
		}
		imported++
	}
	if err := rows.Err(); err != nil {
		tx.Rollback()
		return fmt.Errorf("databricks: row iteration failed: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	log.Printf("imported %d A365 agents from databricks (%s.%s.%s)", imported, catalog, schema, table)

	var count int
	if err := pgDB.QueryRow(`SELECT count(*) FROM a365_agents`).Scan(&count); err != nil {
		log.Printf("a365: failed to compute mapped count: %v", err)
		return nil
	}
	persistA365MappedKPI(count)
	return nil
}

// persistA365MappedKPI stores the "number of A365 agents mapped" KPI as a
// standalone key/value pair in both Postgres (the generic kpis table) and
// Redis. This is separate from pushMappedCountsToRedis's agents/models/
// policies/wiz-integrations counts, since A365 data only exists at all when
// Databricks is configured.
func persistA365MappedKPI(count int) {
	if _, err := db.Exec(`
		INSERT INTO kpis (key, value, updated_at) VALUES ($1, $2, now())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()
	`, "a365_agents_mapped", count); err != nil {
		log.Printf("a365: failed to persist mapped KPI to postgres: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rdb.Set(ctx, redisKeyA365AgentsMapped, count, 0).Err(); err != nil {
		log.Printf("a365: failed to push mapped KPI to redis: %v", err)
		return
	}
	log.Printf("redis: %s = %d", redisKeyA365AgentsMapped, count)
}

func listA365Agents(w http.ResponseWriter, r *http.Request) {
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
		whereClause = `WHERE name ILIKE $1 OR status ILIKE $1 OR publisher ILIKE $1 OR owner ILIKE $1 OR platform ILIKE $1`
		args = append(args, "%"+search+"%")
	}

	var total int
	if err := db.QueryRow(`SELECT count(*) FROM a365_agents `+whereClause, args...).Scan(&total); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	query := `SELECT title_id, name, status, channel, date_created, last_modified, publisher, publisher_type, version, owner, description, platform, creator_id, environment_id, bot_id,
	                 has_custom_actions, custom_action_list, sensitivity,
	                 can_read_onedrive_and_sharepoint_items, onedrive_and_sharepoint_items, can_read_onedrive_files, onedrive_files, onedrive_sites, can_read_sharepoint_sites_and_files, sharepoint_files, sharepoint_sites,
	                 can_extend_to_graph_connector, graph_connector_details, can_generate_images, can_use_code_interpreter, contains_uploaded_files, uploaded_files,
	                 instructions, groups_shared, users_shared, deployment, run_time, risks, active_users, total_sessions, exception_rate, tags, last_used
	          FROM a365_agents ` + whereClause + `
	          ORDER BY name
	          LIMIT $` + strconv.Itoa(len(args)+1) + ` OFFSET $` + strconv.Itoa(len(args)+2)
	args = append(args, limit, offset)

	rows, err := db.Query(query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	items := []A365Agent{}
	for rows.Next() {
		var a A365Agent
		if err := rows.Scan(
			&a.TitleID, &a.Name, &a.Status, &a.Channel, &a.DateCreated, &a.LastModified, &a.Publisher, &a.PublisherType, &a.Version, &a.Owner, &a.Description, &a.Platform, &a.CreatorID, &a.EnvironmentID, &a.BotID,
			&a.HasCustomActions, &a.CustomActionList, &a.Sensitivity,
			&a.CanReadOneDriveAndSharepointItems, &a.OneDriveAndSharepointItems, &a.CanReadOneDriveFiles, &a.OneDriveFiles, &a.OneDriveSites, &a.CanReadSharepointSitesAndFiles, &a.SharepointFiles, &a.SharepointSites,
			&a.CanExtendToGraphConnector, &a.GraphConnectorDetails, &a.CanGenerateImages, &a.CanUseCodeInterpreter, &a.ContainsUploadedFiles, &a.UploadedFiles,
			&a.Instructions, &a.GroupsShared, &a.UsersShared, &a.Deployment, &a.RunTime, &a.Risks, &a.ActiveUsers, &a.TotalSessions, &a.ExceptionRate, &a.Tags, &a.LastUsed,
		); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		items = append(items, a)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items":  items,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}
