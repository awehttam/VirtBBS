package files

import (
	"log"
	"time"

	"github.com/virtbbs/virtbbs/internal/config"
)

// StartDailyLocalFile launches a background goroutine that rebuilds LOCALFIL.ZIP
// once per day. Returns a stop function.
func StartDailyLocalFile(store *Store) func() {
	stop := make(chan struct{})
	go func() {
		runLocalFile(store)
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				runLocalFile(store)
			}
		}
	}()
	return func() { close(stop) }
}

func runLocalFile(store *Store) {
	name := config.Get().BBS.Name
	if err := store.BuildLocalFile(name); err != nil {
		log.Printf("localfile: %v", err)
		return
	}
	log.Printf("localfile: rebuilt %s for %q", LocalFileZipName, name)
}