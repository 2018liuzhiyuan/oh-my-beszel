//go:build testing

package hub_test

import (
	"net/http"
	"testing"

	beszelTests "github.com/henrygd/beszel/internal/tests"
	"github.com/pocketbase/pocketbase/core"
	pbTests "github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/require"
)

func TestAgentDeploymentIdentityDoesNotGrantSystemAccess(t *testing.T) {
	t.Setenv("SHARE_ALL_SYSTEMS", "false")
	testHub, user := beszelTests.GetHubWithUser(t)
	defer testHub.Cleanup()
	otherUser, err := beszelTests.CreateUser(testHub, "identity-other@example.com", "password123")
	require.NoError(t, err)
	token, err := user.NewAuthToken()
	require.NoError(t, err)

	makeSystem := func(name, userID string) *core.Record {
		record, err := beszelTests.CreateRecord(testHub, "systems", map[string]any{
			"name": name, "host": "192.0.2.10", "port": "45876", "status": "paused",
			"users":                     []string{userID},
			"agent_machine_fingerprint": "same-machine-fingerprint", "agent_remote_uid": "1000",
			"agent_install_dir": "/home/deploy/.local/share/beszel-agent", "agent_instance_id": "same-agent-instance",
		})
		require.NoError(t, err)
		return record
	}
	owned := makeSystem("identity-owned", user.Id)
	other := makeSystem("identity-other", otherUser.Id)
	reloaded, err := testHub.FindRecordById("systems", other.Id)
	require.NoError(t, err)
	require.Equal(t, owned.GetString("agent_instance_id"), reloaded.GetString("agent_instance_id"))
	require.Equal(t, "same-agent-instance", reloaded.GetString("agent_instance_id"))
	factory := func(t testing.TB) *pbTests.TestApp { return testHub.TestApp }

	scenarios := []beszelTests.ApiScenario{
		{
			Name:   "matching instance does not reveal another user's system in list",
			Method: http.MethodGet, URL: "/api/collections/systems/records",
			Headers: map[string]string{"Authorization": token}, ExpectedStatus: http.StatusOK,
			ExpectedContent: []string{owned.Id}, NotExpectedContent: []string{other.Id}, TestAppFactory: factory,
		},
		{
			Name:   "matching instance does not grant system view",
			Method: http.MethodGet, URL: "/api/collections/systems/records/" + other.Id,
			Headers: map[string]string{"Authorization": token}, ExpectedStatus: http.StatusNotFound,
			ExpectedContent: []string{"resource wasn't found"}, TestAppFactory: factory,
		},
		{
			Name:   "matching instance does not grant system update",
			Method: http.MethodPatch, URL: "/api/collections/systems/records/" + other.Id,
			Body:    jsonReader(map[string]any{"name": "unauthorized-change"}),
			Headers: map[string]string{"Authorization": token}, ExpectedStatus: http.StatusNotFound,
			ExpectedContent: []string{"resource wasn't found"}, TestAppFactory: factory,
		},
		{
			Name:   "explicit users membership grants system view",
			Method: http.MethodGet, URL: "/api/collections/systems/records/" + other.Id,
			Headers: map[string]string{"Authorization": token}, ExpectedStatus: http.StatusOK,
			ExpectedContent: []string{other.Id, "same-agent-instance"}, TestAppFactory: factory,
			BeforeTestFunc: func(t testing.TB, app *pbTests.TestApp, e *core.ServeEvent) {
				record, err := app.FindRecordById("systems", other.Id)
				require.NoError(t, err)
				require.Equal(t, "identity-other", record.GetString("name"))
				record.Set("users", []string{otherUser.Id, user.Id})
				require.NoError(t, app.Save(record))
			},
		},
		{
			Name:   "member can update ordinary fields without resubmitting identity",
			Method: http.MethodPatch, URL: "/api/collections/systems/records/" + other.Id,
			Body:    jsonReader(map[string]any{"name": "authorized-change"}),
			Headers: map[string]string{"Authorization": token}, ExpectedStatus: http.StatusOK,
			ExpectedContent: []string{"authorized-change", "same-agent-instance"}, TestAppFactory: factory,
		},
	}
	for i := range scenarios {
		scenarios[i].Test(t)
	}
}

func TestAgentDeploymentIdentityRejectsClientSpoofing(t *testing.T) {
	testHub, user := beszelTests.GetHubWithUser(t)
	defer testHub.Cleanup()
	token, err := user.NewAuthToken()
	require.NoError(t, err)
	identity := map[string]string{
		"agent_machine_fingerprint": "verified-fingerprint", "agent_remote_uid": "1000",
		"agent_install_dir": "/home/deploy/.local/share/beszel-agent", "agent_instance_id": "verified-instance",
	}
	fields := map[string]any{
		"name": "identity-protected", "host": "192.0.2.10", "port": "45876", "status": "paused",
		"users": []string{user.Id},
	}
	for key, value := range identity {
		fields[key] = value
	}
	system, err := beszelTests.CreateRecord(testHub, "systems", fields)
	require.NoError(t, err)

	for _, field := range []string{"agent_machine_fingerprint", "agent_remote_uid", "agent_install_dir", "agent_instance_id"} {
		for _, method := range []string{http.MethodPost, http.MethodPatch} {
			body := map[string]any{"name": "spoofed-name", field: "spoofed-value"}
			requestURL := "/api/collections/systems/records/" + system.Id
			if method == http.MethodPost {
				requestURL = "/api/collections/systems/records"
				body["host"] = "192.0.2.11"
				body["port"] = "45876"
				body["status"] = "paused"
				body["users"] = []string{user.Id}
			}
			scenario := beszelTests.ApiScenario{
				Name: method + " rejects " + field, Method: method, URL: requestURL,
				Body: jsonReader(body), Headers: map[string]string{"Authorization": token},
				ExpectedStatus: http.StatusBadRequest, ExpectedContent: []string{"Agent identity is managed by the Hub"},
				TestAppFactory: func(t testing.TB) *pbTests.TestApp { return testHub.TestApp },
				AfterTestFunc: func(t testing.TB, app *pbTests.TestApp, res *http.Response) {
					count, err := app.CountRecords("systems")
					require.NoError(t, err)
					require.EqualValues(t, 1, count)
					record, err := app.FindRecordById("systems", system.Id)
					require.NoError(t, err)
					require.Equal(t, "identity-protected", record.GetString("name"))
					for key, value := range identity {
						require.Equal(t, value, record.GetString(key))
					}
				},
			}
			scenario.Test(t)
		}
	}
}

func TestAgentDeploymentIdentitySharedSystemKeepsPersonalAlerts(t *testing.T) {
	testHub, user := beszelTests.GetHubWithUser(t)
	defer testHub.Cleanup()
	otherUser, err := beszelTests.CreateUser(testHub, "identity-alerts-other@example.com", "password123")
	require.NoError(t, err)
	token, err := user.NewAuthToken()
	require.NoError(t, err)
	system, err := beszelTests.CreateRecord(testHub, "systems", map[string]any{
		"name": "shared-agent-alerts", "host": "192.0.2.10", "port": "45876", "status": "paused",
		"users": []string{user.Id, otherUser.Id}, "agent_instance_id": "shared-alerts-instance",
	})
	require.NoError(t, err)
	makeAlert := func(userID string, value int) *core.Record {
		record, err := beszelTests.CreateRecord(testHub, "alerts", map[string]any{
			"system": system.Id, "user": userID, "name": "CPU", "value": value, "min": 10,
		})
		require.NoError(t, err)
		return record
	}
	owned := makeAlert(user.Id, 80)
	other := makeAlert(otherUser.Id, 90)
	factory := func(t testing.TB) *pbTests.TestApp { return testHub.TestApp }
	scenarios := []beszelTests.ApiScenario{
		{
			Name:   "shared system exposes only current user's alerts",
			Method: http.MethodGet, URL: "/api/collections/alerts/records",
			Headers: map[string]string{"Authorization": token}, ExpectedStatus: http.StatusOK,
			ExpectedContent: []string{owned.Id}, NotExpectedContent: []string{other.Id}, TestAppFactory: factory,
		},
		{
			Name:   "alert overwrite uses authenticated user even if another user is supplied",
			Method: http.MethodPost, URL: "/api/beszel/user-alerts",
			Body: jsonReader(map[string]any{
				"systems": []string{system.Id}, "name": "CPU", "value": 70, "min": 10,
				"overwrite": true, "user": otherUser.Id,
			}),
			Headers: map[string]string{"Authorization": token}, ExpectedStatus: http.StatusOK,
			ExpectedContent: []string{`"success":true`}, TestAppFactory: factory,
			AfterTestFunc: func(t testing.TB, app *pbTests.TestApp, res *http.Response) {
				record, err := app.FindRecordById("alerts", owned.Id)
				require.NoError(t, err)
				require.Equal(t, 70.0, record.GetFloat("value"))
				record, err = app.FindRecordById("alerts", other.Id)
				require.NoError(t, err)
				require.Equal(t, 90.0, record.GetFloat("value"))
			},
		},
		{
			Name:   "deleting personal alerts preserves another member's alert",
			Method: http.MethodDelete, URL: "/api/beszel/user-alerts",
			Body:    jsonReader(map[string]any{"systems": []string{system.Id}, "name": "CPU", "user": otherUser.Id}),
			Headers: map[string]string{"Authorization": token}, ExpectedStatus: http.StatusOK,
			ExpectedContent: []string{`"success":true`, `"count":1`}, TestAppFactory: factory,
			AfterTestFunc: func(t testing.TB, app *pbTests.TestApp, res *http.Response) {
				_, err := app.FindRecordById("alerts", owned.Id)
				require.Error(t, err)
				record, err := app.FindRecordById("alerts", other.Id)
				require.NoError(t, err)
				require.Equal(t, otherUser.Id, record.GetString("user"))
				require.Equal(t, 90.0, record.GetFloat("value"))
			},
		},
	}
	for i := range scenarios {
		scenarios[i].Test(t)
	}
}
