package fido

import "database/sql"

// TICExportDB tracks which local files have been exported via TIC per network.
type TICExportDB struct{ db *sql.DB }

func OpenTICExportDB(db *sql.DB) *TICExportDB { return &TICExportDB{db: db} }

func (t *TICExportDB) IsExported(network string, dirID int64, filename string) (bool, error) {
	var n int
	err := t.db.QueryRow(`SELECT COUNT(*) FROM fido_file_exports WHERE network=? AND dir_id=? AND filename=?`,
		network, dirID, filename).Scan(&n)
	return n > 0, err
}

func (t *TICExportDB) MarkExported(network string, dirID int64, filename string) error {
	_, err := t.db.Exec(`INSERT OR IGNORE INTO fido_file_exports (network, dir_id, filename) VALUES (?,?,?)`,
		network, dirID, filename)
	return err
}
