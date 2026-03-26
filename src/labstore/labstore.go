package labstore

import (
	"database/sql"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"github.com/montybeatnik/arista-lab/laber/labplanner"
)

type LabRecord struct {
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	NodeType  string    `json:"nodeType,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

type LabPlanView struct {
	Nodes     []labplanner.NodePlan
	Links     []labplanner.LinkAssigned
	Protocols labplanner.ProtocolSet
}

func OpenLabDB(baseDir string) (*sql.DB, error) {
	dbPath := filepath.Join(baseDir, ".lab-index.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	if err := initLabDB(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func UpsertLab(db *sql.DB, name, path string) error {
	_, err := db.Exec(`
		INSERT INTO labs (name, path, created_at)
		VALUES (?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
			path = excluded.path,
			created_at = excluded.created_at;
	`, name, path, time.Now().UTC().Format(time.RFC3339))
	return err
}

func ListLabs(db *sql.DB) ([]LabRecord, error) {
	rows, err := db.Query(`
		SELECT name, path, created_at
		FROM labs
		ORDER BY created_at DESC;
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var labs []LabRecord
	for rows.Next() {
		var name string
		var path string
		var created string
		if err := rows.Scan(&name, &path, &created); err != nil {
			return nil, err
		}
		t, _ := time.Parse(time.RFC3339, created)
		labs = append(labs, LabRecord{Name: name, Path: path, CreatedAt: t})
	}
	return labs, rows.Err()
}

func SaveLabPlan(db *sql.DB, labName string, plan labplanner.LabPlan, protocols labplanner.ProtocolSet) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec("DELETE FROM nodes WHERE lab_name = ?", labName); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM links WHERE lab_name = ?", labName); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM protocols WHERE lab_name = ?", labName); err != nil {
		return err
	}

	for _, n := range plan.Nodes {
		if _, err := tx.Exec(
			"INSERT INTO nodes (lab_name, name, role, asn, loopback, edge_ip, edge_prefix) VALUES (?, ?, ?, ?, ?, ?, ?)",
			labName, n.Name, n.Role, n.ASN, n.Loopback, n.EdgeIP, n.EdgePrefix,
		); err != nil {
			return err
		}
	}
	for _, l := range plan.Links {
		if _, err := tx.Exec(
			"INSERT INTO links (lab_name, a, b, a_if, b_if, subnet, a_ip, b_ip) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
			labName, l.A, l.B, l.AIf, l.BIf, l.Subnet, l.AIP, l.BIP,
		); err != nil {
			return err
		}
	}
	for _, p := range protocols.Global {
		if _, err := tx.Exec(
			"INSERT INTO protocols (lab_name, scope, scope_name, proto) VALUES (?, ?, ?, ?)",
			labName, "global", "global", p,
		); err != nil {
			return err
		}
	}
	for role, list := range protocols.Roles {
		for _, p := range list {
			if _, err := tx.Exec(
				"INSERT INTO protocols (lab_name, scope, scope_name, proto) VALUES (?, ?, ?, ?)",
				labName, "role", role, p,
			); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

func LoadLabPlan(db *sql.DB, labName string) (LabPlanView, error) {
	var out LabPlanView
	rows, err := db.Query(`
		SELECT name, role, asn, loopback, edge_ip, edge_prefix
		FROM nodes
		WHERE lab_name = ?
		ORDER BY name;
	`, labName)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var n labplanner.NodePlan
		if err := rows.Scan(&n.Name, &n.Role, &n.ASN, &n.Loopback, &n.EdgeIP, &n.EdgePrefix); err != nil {
			_ = rows.Close()
			return out, err
		}
		out.Nodes = append(out.Nodes, n)
	}
	_ = rows.Close()

	rows, err = db.Query(`
		SELECT a, b, a_if, b_if, subnet, a_ip, b_ip
		FROM links
		WHERE lab_name = ?;
	`, labName)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var l labplanner.LinkAssigned
		if err := rows.Scan(&l.A, &l.B, &l.AIf, &l.BIf, &l.Subnet, &l.AIP, &l.BIP); err != nil {
			_ = rows.Close()
			return out, err
		}
		out.Links = append(out.Links, l)
	}
	_ = rows.Close()

	rows, err = db.Query(`
		SELECT scope, scope_name, proto
		FROM protocols
		WHERE lab_name = ?;
	`, labName)
	if err != nil {
		return out, err
	}
	out.Protocols = labplanner.ProtocolSet{Roles: map[string][]string{}}
	for rows.Next() {
		var scope, scopeName, proto string
		if err := rows.Scan(&scope, &scopeName, &proto); err != nil {
			_ = rows.Close()
			return out, err
		}
		if scope == "global" {
			out.Protocols.Global = append(out.Protocols.Global, proto)
		} else if scope == "role" {
			out.Protocols.Roles[scopeName] = append(out.Protocols.Roles[scopeName], proto)
		}
	}
	_ = rows.Close()

	return out, nil
}

func initLabDB(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS labs (
			name TEXT PRIMARY KEY,
			path TEXT NOT NULL,
			created_at TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS nodes (
			lab_name TEXT NOT NULL,
			name TEXT NOT NULL,
			role TEXT NOT NULL,
			asn INTEGER NOT NULL,
			loopback TEXT,
			edge_ip TEXT,
			edge_prefix INTEGER,
			PRIMARY KEY (lab_name, name)
		);
		CREATE TABLE IF NOT EXISTS links (
			lab_name TEXT NOT NULL,
			a TEXT NOT NULL,
			b TEXT NOT NULL,
			a_if TEXT,
			b_if TEXT,
			subnet TEXT,
			a_ip TEXT,
			b_ip TEXT
		);
		CREATE TABLE IF NOT EXISTS protocols (
			lab_name TEXT NOT NULL,
			scope TEXT NOT NULL,
			scope_name TEXT NOT NULL,
			proto TEXT NOT NULL
		);
	`)
	_, _ = db.Exec(`ALTER TABLE nodes ADD COLUMN edge_ip TEXT`)
	_, _ = db.Exec(`ALTER TABLE nodes ADD COLUMN edge_prefix INTEGER`)
	return err
}
