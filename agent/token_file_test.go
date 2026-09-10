//go:build testing

package agent

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/lxzan/gws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetTokenParsesTokenFileLines(t *testing.T) {
	cases := []struct {
		name      string
		contents  string
		want      string
		wantError bool
	}{
		{"comments and CRLF", " # explanation\r\n\r\n file-token \r\n# trailing comment\r\n", "file-token", false},
		{"multiple tokens", "first-token\n# explanation\nsecond-token\n", "", true},
		{"empty file", "", "", false},
		{"only comments", "\n # comment\n\t", "", false},
		{"inline hash remains token data", "token#suffix\n", "token#suffix", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Given token file configuration without an environment token.
			path := filepath.Join(t.TempDir(), "token")
			require.NoError(t, os.WriteFile(path, []byte(tc.contents), 0600))
			t.Setenv("BESZEL_AGENT_TOKEN", "")
			t.Setenv("BESZEL_AGENT_TOKEN_FILE", path)

			// When configuration reads the token file.
			got, err := getToken()

			// Then only a single non-comment line is accepted.
			if tc.wantError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), path)
				assert.NotContains(t, err.Error(), "first-token")
				assert.NotContains(t, err.Error(), "second-token")
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestGetTokenEnvironmentPrecedesMalformedTokenFile(t *testing.T) {
	// Given an environment token and a malformed file.
	path := filepath.Join(t.TempDir(), "token")
	require.NoError(t, os.WriteFile(path, []byte("first-token\nsecond-token"), 0600))
	t.Setenv("BESZEL_AGENT_TOKEN", "environment-token")
	t.Setenv("BESZEL_AGENT_TOKEN_FILE", path)

	// When the client resolves its token.
	got, err := getToken()

	// Then the existing environment precedence is preserved.
	require.NoError(t, err)
	assert.Equal(t, "environment-token", got)
}

func TestTokenFileCommentsProduceValidWebSocketHandshakeHeader(t *testing.T) {
	// Given a commented token file and a local hub HTTP endpoint.
	path := filepath.Join(t.TempDir(), "token")
	require.NoError(t, os.WriteFile(path, []byte("# registration token\nwire-token\n"), 0600))
	t.Setenv("BESZEL_AGENT_TOKEN", "")
	t.Setenv("BESZEL_AGENT_TOKEN_FILE", path)
	requests := make(chan http.Header, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Header.Clone()
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(server.Close)
	t.Setenv("BESZEL_AGENT_HUB_URL", server.URL)
	client, err := newWebSocketClient(createTestAgent(t))
	require.NoError(t, err)
	client.getOptions().NewDialer = func() (gws.Dialer, error) { return &net.Dialer{}, nil }

	// When the real WebSocket client sends its HTTP upgrade handshake.
	err = client.Connect()

	// Then the endpoint receives a valid token header before rejecting the upgrade.
	require.Error(t, err)
	select {
	case headers := <-requests:
		assert.Equal(t, "wire-token", headers.Get("X-Token"))
		assert.Equal(t, "websocket", headers.Get("Upgrade"))
	default:
		t.Fatal("hub did not receive a handshake")
	}
}
