package fido

import (
	"database/sql"
	"fmt"
	"strings"
)

// RenameNetwork updates a network's display name in config and all SQLite
// tables that key on network name.
func RenameNetwork(cfg *Config, db *sql.DB, oldName, newName string) error {
	oldName = strings.TrimSpace(oldName)
	newName = strings.TrimSpace(newName)
	if oldName == "" || newName == "" {
		return fmt.Errorf("network names required")
	}
	if strings.EqualFold(oldName, newName) {
		return nil
	}
	if cfg.NetworkByName(newName) != nil {
		return fmt.Errorf("network %q already exists", newName)
	}

	if strings.EqualFold(oldName, cfg.EffectivePrimaryName()) {
		cfg.Name = newName
	} else {
		found := false
		for i := range cfg.Networks {
			if strings.EqualFold(cfg.Networks[i].Name, oldName) {
				cfg.Networks[i].Name = newName
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("network %q not found", oldName)
		}
	}

	if db != nil {
		tables := []string{
			"fido_areafix_subs",
			"fido_filefix_subs",
			"fido_file_exports",
			"fido_nodelist_versions",
			"fido_join_requests",
			"fido_members",
			"fido_routes",
			"fido_binkp_stats",
			"fido_binkp_link_stats",
		}
		for _, tbl := range tables {
			if _, err := db.Exec(`UPDATE `+tbl+` SET network = ? WHERE network = ?`, newName, oldName); err != nil {
				return fmt.Errorf("update %s: %w", tbl, err)
			}
		}
	}
	return nil
}
