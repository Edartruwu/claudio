package storage

import "database/sql"

// GetPluginData retrieves the value for (pluginName, key).
// Returns ("", nil) if the key does not exist.
func (db *DB) GetPluginData(pluginName, key string) (string, error) {
	var value string
	err := db.conn.QueryRow(
		`SELECT value FROM plugin_data WHERE plugin_name = ? AND key = ?`,
		pluginName, key,
	).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return value, nil
}

// SetPluginData upserts a value for (pluginName, key).
func (db *DB) SetPluginData(pluginName, key, value string) error {
	_, err := db.conn.Exec(
		`INSERT OR REPLACE INTO plugin_data (plugin_name, key, value, updated_at)
		 VALUES (?, ?, ?, CURRENT_TIMESTAMP)`,
		pluginName, key, value,
	)
	return err
}

// DeletePluginData removes a single key for a plugin.
func (db *DB) DeletePluginData(pluginName, key string) error {
	_, err := db.conn.Exec(
		`DELETE FROM plugin_data WHERE plugin_name = ? AND key = ?`,
		pluginName, key,
	)
	return err
}

// ListPluginData returns all key-value pairs for a plugin.
func (db *DB) ListPluginData(pluginName string) (map[string]string, error) {
	rows, err := db.conn.Query(
		`SELECT key, value FROM plugin_data WHERE plugin_name = ? ORDER BY key ASC`,
		pluginName,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		result[k] = v
	}
	return result, rows.Err()
}
