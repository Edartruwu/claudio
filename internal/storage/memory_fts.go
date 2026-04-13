package storage

import (
	"database/sql"
	"fmt"
	"strings"
)

// FTSIndex wraps the SQLite FTS5 memory search index.
type FTSIndex struct {
	db *sql.DB
}

// NewFTSIndex returns an FTSIndex backed by the given connection.
func NewFTSIndex(db *sql.DB) *FTSIndex {
	return &FTSIndex{db: db}
}

// NameScope is a (name, scope) pair returned by Search.
type NameScope struct {
	Name  string
	Scope string
}

// Upsert inserts or replaces a memory entry in the FTS index and meta table.
// mtime is the unix timestamp of the source .md file.
func (f *FTSIndex) Upsert(name, scope, description, tagsText, factsText, conceptsText string, mtime int64) error {
	// FTS5 doesn't support UPDATE — delete then insert
	if _, err := f.db.Exec(`DELETE FROM memory_fts WHERE name=? AND scope=?`, name, scope); err != nil {
		return fmt.Errorf("fts delete before upsert: %w", err)
	}
	if _, err := f.db.Exec(
		`INSERT INTO memory_fts(name,scope,description,tags_text,facts_text,concepts_text) VALUES(?,?,?,?,?,?)`,
		name, scope, description, tagsText, factsText, conceptsText,
	); err != nil {
		return fmt.Errorf("fts insert: %w", err)
	}
	if _, err := f.db.Exec(
		`INSERT OR REPLACE INTO memory_fts_meta(name,scope,file_mtime) VALUES(?,?,?)`,
		name, scope, mtime,
	); err != nil {
		return fmt.Errorf("fts meta upsert: %w", err)
	}
	return nil
}

// Delete removes an entry from the FTS index and meta table.
func (f *FTSIndex) Delete(name, scope string) error {
	if _, err := f.db.Exec(`DELETE FROM memory_fts WHERE name=? AND scope=?`, name, scope); err != nil {
		return err
	}
	_, err := f.db.Exec(`DELETE FROM memory_fts_meta WHERE name=? AND scope=?`, name, scope)
	return err
}

// DeleteByName removes all FTS entries matching name regardless of scope.
// Used from Store.Remove() which manages a single scope's directory but
// doesn't have the scope string readily available.
func (f *FTSIndex) DeleteByName(name string) error {
	if _, err := f.db.Exec(`DELETE FROM memory_fts WHERE name=?`, name); err != nil {
		return err
	}
	_, err := f.db.Exec(`DELETE FROM memory_fts_meta WHERE name=?`, name)
	return err
}

// Search returns (name, scope) pairs ranked by BM25 relevance.
// scopes filters results — nil means all scopes.
// limit caps the number of results.
func (f *FTSIndex) Search(query string, scopes []string, limit int) ([]NameScope, error) {
	if query == "" {
		return nil, nil
	}
	ftsQuery := sanitizeFTSQuery(query)
	if ftsQuery == "" {
		return nil, nil
	}

	var (
		q    strings.Builder
		args []interface{}
	)
	q.WriteString(`SELECT name, scope FROM memory_fts WHERE memory_fts MATCH ?`)
	args = append(args, ftsQuery)

	if len(scopes) > 0 {
		placeholders := make([]string, len(scopes))
		for i, s := range scopes {
			placeholders[i] = "?"
			args = append(args, s)
		}
		q.WriteString(` AND scope IN (` + strings.Join(placeholders, ",") + `)`)
	}
	q.WriteString(` ORDER BY rank LIMIT ?`)
	args = append(args, limit)

	rows, err := f.db.Query(q.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []NameScope
	for rows.Next() {
		var ns NameScope
		if err := rows.Scan(&ns.Name, &ns.Scope); err != nil {
			continue
		}
		results = append(results, ns)
	}
	return results, rows.Err()
}

// LoadMeta returns a map of name→file_mtime for all entries in a given scope.
func (f *FTSIndex) LoadMeta(scope string) (map[string]int64, error) {
	rows, err := f.db.Query(`SELECT name, file_mtime FROM memory_fts_meta WHERE scope=?`, scope)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	meta := make(map[string]int64)
	for rows.Next() {
		var name string
		var mtime int64
		if err := rows.Scan(&name, &mtime); err != nil {
			continue
		}
		meta[name] = mtime
	}
	return meta, rows.Err()
}

// sanitizeFTSQuery makes a user query safe for FTS5 MATCH.
// Strips special FTS5 characters and joins words as separate terms.
func sanitizeFTSQuery(q string) string {
	var sb strings.Builder
	for _, r := range q {
		switch r {
		case '"', '\'', '(', ')', '*', '^', '+', '-', ':', '.':
			sb.WriteRune(' ')
		default:
			sb.WriteRune(r)
		}
	}
	clean := strings.TrimSpace(sb.String())
	if clean == "" {
		return ""
	}
	// Use each word as a separate term (implicit AND in FTS5)
	words := strings.Fields(clean)
	return strings.Join(words, " ")
}
