package hub_test

import (
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
