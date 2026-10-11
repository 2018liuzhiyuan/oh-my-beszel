package hub_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	beszelTests "github.com/henrygd/beszel/internal/tests"
	pbTests "github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/require"
)

func TestSSHHostsCustomPathPermissions(t *testing.T) {
	testHub, user := beszelTests.GetHubWithUser(t)
	defer testHub.Cleanup()

	userToken, err := user.NewAuthToken()
	require.NoError(t, err)
	readOnlyUser, err := beszelTests.CreateUserWithRole(testHub, "readonly-ssh@example.com", "password123", "readonly")
	require.NoError(t, err)
	readOnlyToken, err := readOnlyUser.NewAuthToken()
	require.NoError(t, err)

	configPath := filepath.Join(t.TempDir(), "ssh-config")
	require.NoError(t, os.WriteFile(configPath, []byte("Host qa-custom\n    HostName 192.0.2.10\n"), 0o600))
	requestURL := "/api/beszel/ssh-hosts?path=" + url.QueryEscape(configPath)
	testAppFactory := func(t testing.TB) *pbTests.TestApp {
		return testHub.TestApp
	}

	scenarios := []beszelTests.ApiScenario{
		{
			Name:               "unauthenticated user cannot load a custom SSH config path",
			Method:             http.MethodGet,
			URL:                requestURL,
			ExpectedStatus:     http.StatusUnauthorized,
			ExpectedContent:    []string{"requires valid"},
			NotExpectedContent: []string{"qa-custom", "192.0.2.10"},
			TestAppFactory:     testAppFactory,
		},
		{
			Name:   "editable user can load a custom SSH config path",
			Method: http.MethodGet,
			URL:    requestURL,
			Headers: map[string]string{
				"Authorization": userToken,
			},
			ExpectedStatus:  200,
			ExpectedContent: []string{"qa-custom", "192.0.2.10"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "readonly user cannot load a custom SSH config path",
			Method: http.MethodGet,
			URL:    requestURL,
			Headers: map[string]string{
				"Authorization": readOnlyToken,
			},
			ExpectedStatus:  403,
			ExpectedContent: []string{"not allowed"},
			TestAppFactory:  testAppFactory,
		},
	}

	for index := range scenarios {
		scenarios[index].Test(t)
	}
}

func TestSSHHostsCustomPathDoesNotExpandEnvironment(t *testing.T) {
	testHub, user := beszelTests.GetHubWithUser(t)
	defer testHub.Cleanup()
	token, err := user.NewAuthToken()
	require.NoError(t, err)

	dir := t.TempDir()
	literalPath := filepath.Join(dir, "${BESZEL_TEST_SSH_SECRET}")
	secretPath := filepath.Join(dir, "private-config")
	require.NoError(t, os.WriteFile(literalPath, []byte("Host literal-host\n"), 0o600))
	require.NoError(t, os.WriteFile(secretPath, []byte("Host private-host\n"), 0o600))
	t.Setenv("BESZEL_TEST_SSH_SECRET", "private-config")

	scenario := beszelTests.ApiScenario{
		Name:               "custom SSH path is read literally without disclosing server environment",
		Method:             http.MethodGet,
		URL:                "/api/beszel/ssh-hosts?path=" + url.QueryEscape(literalPath),
		Headers:            map[string]string{"Authorization": token},
		ExpectedStatus:     http.StatusOK,
		ExpectedContent:    []string{"${BESZEL_TEST_SSH_SECRET}", "literal-host"},
		NotExpectedContent: []string{"private-config", "private-host"},
		TestAppFactory: func(t testing.TB) *pbTests.TestApp {
			return testHub.TestApp
		},
	}
	scenario.Test(t)
}

func TestSSHHostsRequestPathEnvironmentSyntax(t *testing.T) {
	testHub, user := beszelTests.GetHubWithUser(t)
	defer testHub.Cleanup()
	token, err := user.NewAuthToken()
	require.NoError(t, err)

	dir := t.TempDir()
	secretPath := filepath.Join(dir, "server-private-config")
	require.NoError(t, os.WriteFile(secretPath, []byte("Host server-private-host\n"), 0o600))
	t.Setenv("BESZEL_TEST_SSH_SECRET_PATH", secretPath)
	t.Setenv("BESZEL_TEST_SSH_UNSET_PATH", "")
	require.NoError(t, os.Unsetenv("BESZEL_TEST_SSH_UNSET_PATH"))

	for _, tc := range []struct {
		name string
		path string
	}{
		{"bare variable", "$BESZEL_TEST_SSH_SECRET_PATH"},
		{"braced variable", "${BESZEL_TEST_SSH_SECRET_PATH}"},
		{"embedded variable", filepath.Join(dir, "prefix-${BESZEL_TEST_SSH_SECRET_PATH}-suffix")},
		{"nonexistent path", filepath.Join(dir, "missing", "$BESZEL_TEST_SSH_SECRET_PATH")},
		{"unset bare variable", "$BESZEL_TEST_SSH_UNSET_PATH"},
		{"unset braced variable", "${BESZEL_TEST_SSH_UNSET_PATH}"},
	} {
		scenario := beszelTests.ApiScenario{
			Name: tc.name, Method: http.MethodGet,
			URL:                "/api/beszel/ssh-hosts?path=" + url.QueryEscape(tc.path),
			Headers:            map[string]string{"Authorization": token},
			ExpectedStatus:     http.StatusOK,
			ExpectedContent:    []string{`"hosts":[]`},
			NotExpectedContent: []string{"server-private-config", "server-private-host"},
			TestAppFactory:     func(t testing.TB) *pbTests.TestApp { return testHub.TestApp },
			AfterTestFunc: func(t testing.TB, app *pbTests.TestApp, res *http.Response) {
				var body struct {
					Path string `json:"path"`
				}
				require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
				require.Equal(t, filepath.Clean(tc.path), body.Path)
			},
		}
		scenario.Test(t)
	}
}

func TestSSHHostsTrustedEnvironmentConfigAndInclude(t *testing.T) {
	testHub, user := beszelTests.GetHubWithUser(t)
	defer testHub.Cleanup()
	token, err := user.NewAuthToken()
	require.NoError(t, err)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")
	includePath := filepath.Join(dir, "included-config")
	require.NoError(t, os.WriteFile(configPath, []byte("Include ${BESZEL_TEST_SSH_API_INCLUDE}\n"), 0o600))
	require.NoError(t, os.WriteFile(includePath, []byte("Host trusted-included-host\n    HostName 192.0.2.20\n"), 0o600))
	t.Setenv("BESZEL_TEST_SSH_API_CONFIG", configPath)
	t.Setenv("BESZEL_TEST_SSH_API_INCLUDE", includePath)
	t.Setenv("SSH_CONFIG_PATH", "${BESZEL_TEST_SSH_API_CONFIG}")

	scenario := beszelTests.ApiScenario{
		Name: "default SSH config and Include expand trusted environment paths", Method: http.MethodGet,
		URL:             "/api/beszel/ssh-hosts",
		Headers:         map[string]string{"Authorization": token},
		ExpectedStatus:  http.StatusOK,
		ExpectedContent: []string{"trusted-included-host", "192.0.2.20"},
		TestAppFactory:  func(t testing.TB) *pbTests.TestApp { return testHub.TestApp },
		AfterTestFunc: func(t testing.TB, app *pbTests.TestApp, res *http.Response) {
			var body struct {
				Path string `json:"path"`
			}
			require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
			require.Equal(t, configPath, body.Path)
		},
	}
	scenario.Test(t)
}
