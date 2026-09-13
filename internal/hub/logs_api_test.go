package hub_test

import (
	"net/http"
	"testing"

	beszelTests "github.com/henrygd/beszel/internal/tests"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	pbTests "github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/require"
)

// insertHubLogRow writes directly into the auxiliary `_logs` table with a
// fixed timestamp so ordering-sensitive scenarios stay deterministic. The API
// handler only reads that table, so seeding it synchronously keeps the test
// independent of PocketBase's 3-second log batch flush.
func insertHubLogRow(t *testing.T, app core.App, id, created string, level int, message, dataJSON string) {
	t.Helper()
	_, err := app.AuxDB().NewQuery(
		"INSERT INTO _logs (id, created, level, message, data) VALUES ({:id}, {:created}, {:level}, {:message}, {:data})",
	).Bind(dbx.Params{
		"id":      id,
		"created": created,
		"level":   level,
		"message": message,
		"data":    dataJSON,
	}).Execute()
	require.NoError(t, err)
}

func seedHubLogs(t *testing.T, app core.App) {
	t.Helper()
	// scenarios run sequentially against the same app; clear prior seeds
	_, err := app.AuxDB().NewQuery("DELETE FROM _logs WHERE id LIKE 'seed%'").Execute()
	require.NoError(t, err)
	insertHubLogRow(t, app, "seedwarn0001", "2024-01-01 00:00:00.000", 4, "QA warn entry for logs api", `{"system":"seeded-system-id"}`)
	insertHubLogRow(t, app, "seederr0002", "2024-01-01 00:00:01.000", 8, "QA error entry for logs api", `{"system":"seeded-system-id"}`)
}

func TestHubLogsApiPermissionsAndFilters(t *testing.T) {
	testHub, user := beszelTests.GetHubWithUser(t)
	defer testHub.Cleanup()

	adminUser, err := beszelTests.CreateUserWithRole(testHub, "logs-admin@example.com", "password123", "admin")
	require.NoError(t, err)
	adminToken, err := adminUser.NewAuthToken()
	require.NoError(t, err)
	readOnlyUser, err := beszelTests.CreateUserWithRole(testHub, "logs-readonly@example.com", "password123", "readonly")
	require.NoError(t, err)
	readOnlyToken, err := readOnlyUser.NewAuthToken()
	require.NoError(t, err)
	userToken, err := user.NewAuthToken()
	require.NoError(t, err)

	testAppFactory := func(t testing.TB) *pbTests.TestApp {
		return testHub.TestApp
	}
	seeded := func(t testing.TB, app *pbTests.TestApp, e *core.ServeEvent) {
		seedHubLogs(t.(*testing.T), app)
	}

	scenarios := []beszelTests.ApiScenario{
		{
			Name:            "unauthenticated request is rejected",
			Method:          http.MethodGet,
			URL:             "/api/beszel/hub-logs",
			ExpectedStatus:  401,
			ExpectedContent: []string{"requires valid record authorization token"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "readonly user cannot read hub logs",
			Method: http.MethodGet,
			URL:    "/api/beszel/hub-logs",
			Headers: map[string]string{
				"Authorization": readOnlyToken,
			},
			ExpectedStatus:  403,
			ExpectedContent: []string{"not allowed"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "regular editable user cannot read hub logs",
			Method: http.MethodGet,
			URL:    "/api/beszel/hub-logs",
			Headers: map[string]string{
				"Authorization": userToken,
			},
			ExpectedStatus:  403,
			ExpectedContent: []string{"not allowed"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "admin sees warn and error entries by default",
			Method: http.MethodGet,
			URL:    "/api/beszel/hub-logs",
			Headers: map[string]string{
				"Authorization": adminToken,
			},
			ExpectedStatus:  200,
			ExpectedContent: []string{"QA warn entry for logs api", "QA error entry for logs api", "seeded-system-id"},
			BeforeTestFunc:  seeded,
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "level filter excludes warn entries",
			Method: http.MethodGet,
			URL:    "/api/beszel/hub-logs?level=8",
			Headers: map[string]string{
				"Authorization": adminToken,
			},
			ExpectedStatus:     200,
			ExpectedContent:    []string{"QA error entry for logs api"},
			NotExpectedContent: []string{"QA warn entry for logs api"},
			BeforeTestFunc:     seeded,
			TestAppFactory:     testAppFactory,
		},
		{
			Name:   "message filter matches substrings",
			Method: http.MethodGet,
			URL:    "/api/beszel/hub-logs?q=error+entry",
			Headers: map[string]string{
				"Authorization": adminToken,
			},
			ExpectedStatus:     200,
			ExpectedContent:    []string{"QA error entry for logs api"},
			NotExpectedContent: []string{"QA warn entry for logs api"},
			BeforeTestFunc:     seeded,
			TestAppFactory:     testAppFactory,
		},
		{
			Name:   "limit clamps the number of entries",
			Method: http.MethodGet,
			URL:    "/api/beszel/hub-logs?limit=1",
			Headers: map[string]string{
				"Authorization": adminToken,
			},
			ExpectedStatus: 200,
			// newest first: only the error entry is returned
			ExpectedContent:    []string{"QA error entry for logs api"},
			NotExpectedContent: []string{"QA warn entry for logs api"},
			BeforeTestFunc:     seeded,
			TestAppFactory:     testAppFactory,
		},
		{
			Name:   "system filter matches the data payload",
			Method: http.MethodGet,
			URL:    "/api/beszel/hub-logs?system=seeded-system-id",
			Headers: map[string]string{
				"Authorization": adminToken,
			},
			ExpectedStatus:  200,
			ExpectedContent: []string{"QA warn entry for logs api", "QA error entry for logs api"},
			BeforeTestFunc:  seeded,
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "unknown system filter returns no entries",
			Method: http.MethodGet,
			URL:    "/api/beszel/hub-logs?system=missing-system",
			Headers: map[string]string{
				"Authorization": adminToken,
			},
			ExpectedStatus:     200,
			ExpectedContent:    []string{"entries"},
			NotExpectedContent: []string{"QA warn entry", "QA error entry"},
			BeforeTestFunc:     seeded,
			TestAppFactory:     testAppFactory,
		},
	}

	for index := range scenarios {
		scenarios[index].Test(t)
	}
}
