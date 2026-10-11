//go:build testing

package hub_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	beszelTests "github.com/henrygd/beszel/internal/tests"
	"github.com/pocketbase/pocketbase/core"
	pbTests "github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/auth"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

type roleTestOAuthProvider struct {
	auth.BaseProvider
}

func (p *roleTestOAuthProvider) FetchToken(code string, _ ...oauth2.AuthCodeOption) (*oauth2.Token, error) {
	return &oauth2.Token{AccessToken: code}, nil
}

func (p *roleTestOAuthProvider) FetchAuthUser(token *oauth2.Token) (*auth.AuthUser, error) {
	return &auth.AuthUser{Id: token.AccessToken, Email: token.AccessToken + "@example.com"}, nil
}

func configureRoleTestOAuthProvider(t testing.TB, app core.App) string {
	t.Helper()
	const providerName = "beszel_role_test"
	auth.Providers[providerName] = func() auth.Provider { return &roleTestOAuthProvider{} }
	t.Cleanup(func() { delete(auth.Providers, providerName) })
	collection, err := app.FindCollectionByNameOrId("users")
	require.NoError(t, err)
	collection.OAuth2.Enabled = true
	collection.OAuth2.Providers = []core.OAuth2ProviderConfig{{
		Name: providerName, ClientId: "test-client", ClientSecret: "test-secret",
	}}
	require.NoError(t, app.Save(collection))
	return providerName
}

func TestOAuthUserCreationRoles(t *testing.T) {
	t.Setenv("USER_CREATION", "true")
	t.Setenv("BESZEL_HUB_USER_CREATION", "true")
	hub, err := beszelTests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer hub.Cleanup()
	hub.StartHub()

	bootstrap := beszelTests.ApiScenario{
		Name: "bootstrap still creates the first admin", Method: http.MethodPost,
		URL:             "/api/beszel/create-user",
		Body:            strings.NewReader(`{"email":"first-admin@example.com","password":"password123"}`),
		ExpectedStatus:  http.StatusOK,
		ExpectedContent: []string{"User created"},
		TestAppFactory:  func(t testing.TB) *pbTests.TestApp { return hub.TestApp },
		AfterTestFunc: func(t testing.TB, app *pbTests.TestApp, res *http.Response) {
			admin, err := app.FindAuthRecordByEmail("users", "first-admin@example.com")
			require.NoError(t, err)
			require.Equal(t, "admin", admin.GetString("role"))
		},
	}
	bootstrap.Test(t)

	providerName := configureRoleTestOAuthProvider(t, hub)
	oauthCreations := 0
	hub.OnRecordCreateRequest("users").BindFunc(func(e *core.RecordRequestEvent) error {
		info, err := e.RequestInfo()
		require.NoError(t, err)
		if info.Context == core.RequestInfoContextOAuth2 {
			oauthCreations++
		}
		return e.Next()
	})

	for index, tc := range []struct {
		name       string
		createData map[string]any
		status     int
	}{
		{"default role", map[string]any{}, http.StatusOK},
		{"empty role", map[string]any{"role": ""}, http.StatusOK},
		{"user role", map[string]any{"role": "user"}, http.StatusOK},
		{"admin role", map[string]any{"role": "admin"}, http.StatusBadRequest},
		{"readonly role", map[string]any{"role": "readonly"}, http.StatusBadRequest},
		{"append admin role", map[string]any{"role+": "admin"}, http.StatusBadRequest},
		{"prepend admin role", map[string]any{"+role": "admin"}, http.StatusBadRequest},
		{"array admin role", map[string]any{"role": []string{"user", "admin"}}, http.StatusBadRequest},
	} {
		code := fmt.Sprintf("oauth-role-%d", index)
		body, err := json.Marshal(map[string]any{
			"provider": providerName, "code": code,
			"redirectURL": "https://example.com/oauth2", "createData": tc.createData,
		})
		require.NoError(t, err)
		scenario := beszelTests.ApiScenario{
			Name: tc.name, Method: http.MethodPost,
			URL: "/api/collections/users/auth-with-oauth2", Body: strings.NewReader(string(body)),
			ExpectedStatus: tc.status, ExpectedContent: []string{`"message":"Failed to create record."`},
			TestAppFactory: func(t testing.TB) *pbTests.TestApp { return hub.TestApp },
			AfterTestFunc: func(t testing.TB, app *pbTests.TestApp, res *http.Response) {
				record, err := app.FindAuthRecordByEmail("users", code+"@example.com")
				if tc.status == http.StatusOK {
					require.NoError(t, err)
					require.Equal(t, "user", record.GetString("role"))
				} else {
					require.Error(t, err)
				}
			},
		}
		if tc.status == http.StatusOK {
			scenario.ExpectedContent = []string{`"role":"user"`, `"token":"`}
		}
		scenario.Test(t)
	}
	require.Equal(t, 3, oauthCreations, "OAuth signup must use PocketBase's internal create-request context")

	superuser, err := beszelTests.CreateSuperuser(hub, "role-superuser@example.com", "password123")
	require.NoError(t, err)
	token, err := superuser.NewAuthToken()
	require.NoError(t, err)
	for _, tc := range []struct {
		name   string
		token  string
		status int
	}{
		{"anonymous direct signup remains blocked", "", http.StatusBadRequest},
		{"superuser can create an admin", token, http.StatusOK},
	} {
		scenario := beszelTests.ApiScenario{
			Name: tc.name, Method: http.MethodPost, URL: "/api/collections/users/records",
			Body:    strings.NewReader(`{"email":"new-admin@example.com","password":"password123","passwordConfirm":"password123","role":"admin"}`),
			Headers: map[string]string{"Authorization": tc.token}, ExpectedStatus: tc.status,
			ExpectedContent: []string{`"message":"Failed to create record."`},
			TestAppFactory:  func(t testing.TB) *pbTests.TestApp { return hub.TestApp },
		}
		if tc.status == http.StatusOK {
			scenario.ExpectedContent = []string{`"role":"admin"`}
		}
		scenario.Test(t)
	}
}

func TestOAuthUserCreationDisabled(t *testing.T) {
	t.Setenv("USER_CREATION", "false")
	t.Setenv("BESZEL_HUB_USER_CREATION", "false")
	hub, err := beszelTests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer hub.Cleanup()
	hub.StartHub()
	providerName := configureRoleTestOAuthProvider(t, hub)
	collection, err := hub.FindCollectionByNameOrId("users")
	require.NoError(t, err)
	require.Nil(t, collection.CreateRule)

	for _, role := range []string{"admin", "user"} {
		code := "oauth-existing-" + role
		record, err := beszelTests.CreateUserWithRole(hub, code+"@example.com", "password123", role)
		require.NoError(t, err)
		submittedRole := "admin"
		if role == "admin" {
			submittedRole = "user"
		}
		for _, login := range []string{"email match", "linked account"} {
			body, err := json.Marshal(map[string]any{
				"provider": providerName, "code": code, "redirectURL": "https://example.com/oauth2",
				"createData": map[string]any{"role": submittedRole},
			})
			require.NoError(t, err)
			scenario := beszelTests.ApiScenario{
				Name:   "existing " + role + " logs in by " + login,
				Method: http.MethodPost, URL: "/api/collections/users/auth-with-oauth2",
				Body: strings.NewReader(string(body)), ExpectedStatus: http.StatusOK,
				ExpectedContent: []string{`"id":"` + record.Id + `"`, `"role":"` + role + `"`, `"token":"`},
				TestAppFactory:  func(t testing.TB) *pbTests.TestApp { return hub.TestApp },
				AfterTestFunc: func(t testing.TB, app *pbTests.TestApp, res *http.Response) {
					stored, err := app.FindRecordById("users", record.Id)
					require.NoError(t, err)
					require.Equal(t, role, stored.GetString("role"))
				},
			}
			scenario.Test(t)
		}
	}

	for _, role := range []string{"user", "admin"} {
		code := "oauth-new-" + role
		body, err := json.Marshal(map[string]any{
			"provider": providerName, "code": code, "redirectURL": "https://example.com/oauth2",
			"createData": map[string]any{"role": role},
		})
		require.NoError(t, err)
		scenario := beszelTests.ApiScenario{
			Name: "disabled signup blocks new " + role, Method: http.MethodPost,
			URL: "/api/collections/users/auth-with-oauth2", Body: strings.NewReader(string(body)),
			ExpectedStatus: http.StatusForbidden, ExpectedContent: []string{"Only superusers can perform this action."},
			TestAppFactory: func(t testing.TB) *pbTests.TestApp { return hub.TestApp },
			AfterTestFunc: func(t testing.TB, app *pbTests.TestApp, res *http.Response) {
				_, err := app.FindAuthRecordByEmail("users", code+"@example.com")
				require.Error(t, err)
			},
		}
		scenario.Test(t)
	}
	count, err := hub.CountRecords("users")
	require.NoError(t, err)
	require.EqualValues(t, 2, count)
	count, err = hub.CountRecords(core.CollectionNameExternalAuths)
	require.NoError(t, err)
	require.EqualValues(t, 2, count, "repeated logins and rejected signups must not create extra OAuth relations")
}
