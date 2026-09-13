package hub

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

type hubLogEntry struct {
	Created string         `json:"created"`
	Level   int            `json:"level"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data"`
}

// getHubLogs returns recent hub log entries for admin users. The `_logs`
// table only persists warn-level entries by default, so this view is most
// useful for failures; query params: `limit` (default 100, max 500),
// `level` (minimum slog level), `system` (record id matched inside the
// entry data), and `q` (substring of the message).
func getHubLogs(e *core.RequestEvent) error {
	limit := parseClampedInt(e.Request.URL.Query().Get("limit"), 100, 1, 500)
	minLevel := parseClampedInt(e.Request.URL.Query().Get("level"), 0, 0, 12)
	system := strings.TrimSpace(e.Request.URL.Query().Get("system"))
	query := strings.TrimSpace(e.Request.URL.Query().Get("q"))

	logQuery := e.App.LogQuery().
		AndWhere(dbx.NewExp("[[level]] >= {:level}", dbx.Params{"level": minLevel})).
		OrderBy("created DESC", "id DESC").
		Limit(int64(limit))
	if system != "" {
		logQuery.AndWhere(dbx.Like("data", `"system":"`+system+`"`).Match(true, true))
	}
	if query != "" {
		logQuery.AndWhere(dbx.Like("message", query).Match(true, true))
	}

	rows := []struct {
		Created types.DateTime     `db:"created"`
		Level   int                `db:"level"`
		Message string             `db:"message"`
		Data    types.JSONMap[any] `db:"data"`
	}{}
	if err := logQuery.All(&rows); err != nil {
		return e.InternalServerError("Unable to read hub logs.", err)
	}

	entries := make([]hubLogEntry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, hubLogEntry{
			Created: row.Created.String(),
			Level:   row.Level,
			Message: row.Message,
			Data:    row.Data,
		})
	}
	return e.JSON(http.StatusOK, map[string]any{"entries": entries})
}

// parseClampedInt parses base-10 ints from query params with defaults and
// bounds; unparsable values fall back to the default.
func parseClampedInt(value string, fallback, minValue, maxValue int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return min(max(parsed, minValue), maxValue)
}
