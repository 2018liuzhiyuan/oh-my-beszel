package hub

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

type localPortAssignment struct {
	Name   string `json:"name"`
	Port   int    `json:"port"`
	Active bool   `json:"active"`
}

func syncLocalPortAssignments(app core.App) {
	directory := filepath.Join(app.DataDir(), "local-ports")
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		app.Logger().Warn("Cannot read local port assignments", "err", err)
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			app.Logger().Warn("Cannot read local port assignment", "file", entry.Name(), "err", err)
			continue
		}
		assignment, err := parseLocalPortAssignment(entry.Name(), data)
		if err != nil {
			app.Logger().Warn("Invalid local port assignment", "file", entry.Name(), "err", err)
			continue
		}
		if !assignment.Active {
			continue
		}
		records, err := app.FindRecordsByFilter("systems", "name = {:name} && host = '127.0.0.1' && ssh_config = ''", "", 2, 0, dbx.Params{"name": assignment.Name})
		if err != nil {
			app.Logger().Warn("Cannot find local tunnel system", "system", assignment.Name, "err", err)
			continue
		}
		if len(records) > 1 {
			app.Logger().Warn("Ambiguous local tunnel assignment", "system", assignment.Name)
			continue
		}
		if len(records) == 0 {
			continue
		}
		record := records[0]
		if record.GetInt("port") == assignment.Port {
			continue
		}
		previousPort := record.GetString("port")
		record.Set("port", assignment.Port)
		if record.GetString("status") != "paused" {
			record.Set("status", "pending")
		}
		if err := app.SaveNoValidate(record); err != nil {
			app.Logger().Warn("Cannot update local tunnel port", "system", assignment.Name, "err", err)
			continue
		}
		app.Logger().Info("Local tunnel port updated", "system", assignment.Name, "previousPort", previousPort, "port", assignment.Port)
	}
}

func parseLocalPortAssignment(filename string, data []byte) (localPortAssignment, error) {
	if len(data) > 1024 {
		return localPortAssignment{}, errors.New("assignment is too large")
	}
	var assignment localPortAssignment
	if err := json.Unmarshal(data, &assignment); err != nil {
		return localPortAssignment{}, err
	}
	if assignment.Name == "" || filename != assignment.Name+".json" || assignment.Port < 1 || assignment.Port > 65535 {
		return localPortAssignment{}, fmt.Errorf("invalid name or port")
	}
	return assignment, nil
}

func startLocalPortSync(app core.App) {
	stop := make(chan struct{})
	done := make(chan struct{})
	app.OnTerminate().BindFunc(func(event *core.TerminateEvent) error {
		close(stop)
		<-done
		return event.Next()
	})
	go func() {
		defer close(done)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				syncLocalPortAssignments(app)
			}
		}
	}()
}
