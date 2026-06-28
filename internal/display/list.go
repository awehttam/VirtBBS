package display

import (
	"os"
	"path/filepath"
	"strings"
)

// Bulletin describes a display file callers can read on login.
type Bulletin struct {
	Name  string // base name without extension (e.g. LOGON, BINKPDAY)
	Title string // friendly label for menus
}

var bulletinTitles = map[string]string{
	"LOGON":    "Logon Message",
	"BINKPDAY": "BinkP Statistics (24h)",
	"BINKPALL": "BinkP Statistics (All Time)",
	"BULLETIN": "Bulletin",
}

// skipBulletins are display files not shown in the bulletin list.
var skipBulletins = map[string]bool{
	"LOGOFF":  true,
	"NEWUSER": true,
}

// bulletinOrder controls sort priority (lower first).
var bulletinOrder = map[string]int{
	"LOGON":    0,
	"BINKPDAY": 1,
	"BINKPALL": 2,
	"BULLETIN": 3,
}

// ListBulletins returns display files in displayDir suitable for caller bulletins.
func ListBulletins(displayDir string) ([]Bulletin, error) {
	entries, err := os.ReadDir(displayDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	seen := map[string]bool{}
	var out []Bulletin
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		base := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		upper := strings.ToUpper(base)
		if skipBulletins[upper] || seen[upper] {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext != ".ans" && ext != ".asc" && ext != "" {
			continue
		}
		seen[upper] = true
		title := bulletinTitles[upper]
		if title == "" {
			title = strings.ReplaceAll(base, "_", " ")
		}
		out = append(out, Bulletin{Name: upper, Title: title})
	}
	sortBulletins(out)
	return out, nil
}

func sortBulletins(b []Bulletin) {
	for i := 0; i < len(b); i++ {
		for j := i + 1; j < len(b); j++ {
			oi := bulletinOrder[b[i].Name]
			oj := bulletinOrder[b[j].Name]
			if oj < oi || (oj == oi && b[j].Name < b[i].Name) {
				b[i], b[j] = b[j], b[i]
			}
		}
	}
}
