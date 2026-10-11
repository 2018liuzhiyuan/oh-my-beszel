//go:build testing

package agent

import (
	"bytes"
	"crypto/ed25519"
	"crypto/tls"
	"crypto/x509"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/henrygd/beszel/internal/beszel"

	"github.com/henrygd/beszel/internal/common"

	"github.com/fxamacker/cbor/v2"
	"github.com/lxzan/gws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

// TestNewWebSocketClient tests WebSocket client creation
func TestNewWebSocketClient(t *testing.T) {
	agent := createTestAgent(t)

	testCases := []struct {
		name        string
		hubURL      string
		token       string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid configuration",
			hubURL:      "http://localhost:8080",
			token:       "test-token-123",
			expectError: false,
		},
		{
			name:        "valid https URL",
			hubURL:      "https://hub.example.com",
			token:       "secure-token",
			expectError: false,
		},
		{
			name:        "missing hub URL",
			hubURL:      "",
			token:       "test-token",
			expectError: true,
			errorMsg:    "HUB_URL environment variable not set",
		},
		{
			name:        "malformed URL",
			hubURL:      "ht\ttp://invalid",
			token:       "test-token",
			expectError: true,
			errorMsg:    "invalid HUB_URL",
		},
		{
			name:        "URL without host",
			hubURL:      "http:/api",
			token:       "test-token",
			expectError: true,
			errorMsg:    "invalid HUB_URL",
		},
		{
			name:        "missing token",
			hubURL:      "http://localhost:8080",
			token:       "",
			expectError: true,
			errorMsg:    "must set TOKEN or TOKEN_FILE",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up environment
			if tc.hubURL != "" {
				t.Setenv("BESZEL_AGENT_HUB_URL", tc.hubURL)
			}
			if tc.token != "" {
				t.Setenv("BESZEL_AGENT_TOKEN", tc.token)
			}

			client, err := newWebSocketClient(agent)

			if tc.expectError {
				assert.Error(t, err)
				if err != nil && tc.errorMsg != "" {
					assert.Contains(t, err.Error(), tc.errorMsg)
				}
				assert.Nil(t, client)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, client)
				assert.Equal(t, agent, client.agent)
				assert.Equal(t, tc.token, client.token)
				assert.Equal(t, tc.hubURL, client.hubURL.String())
				assert.NotEmpty(t, client.fingerprint)
			}
		})
	}
}

// TestWebSocketClient_GetOptions tests WebSocket client options configuration
func TestWebSocketClient_GetOptions(t *testing.T) {
	agent := createTestAgent(t)

	testCases := []struct {
		name           string
		inputURL       string
		expectedScheme string
		expectedPath   string
	}{
		{
			name:           "http to ws conversion",
			inputURL:       "http://localhost:8080",
			expectedScheme: "ws",
			expectedPath:   "/api/beszel/agent-connect",
		},
		{
			name:           "https to wss conversion",
			inputURL:       "https://hub.example.com",
			expectedScheme: "wss",
			expectedPath:   "/api/beszel/agent-connect",
		},
		{
			name:           "existing path preservation",
			inputURL:       "http://localhost:8080/custom/path",
			expectedScheme: "ws",
			expectedPath:   "/custom/path/api/beszel/agent-connect",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up environment
			t.Setenv("BESZEL_AGENT_HUB_URL", tc.inputURL)
			t.Setenv("BESZEL_AGENT_TOKEN", "test-token")

			client, err := newWebSocketClient(agent)
			require.NoError(t, err)

			options := client.getOptions()

			// Parse the WebSocket URL
			wsURL, err := url.Parse(options.Addr)
			require.NoError(t, err)

			assert.Equal(t, tc.expectedScheme, wsURL.Scheme)
			assert.Equal(t, tc.expectedPath, wsURL.Path)

			// Check headers
			assert.Equal(t, "test-token", options.RequestHeader.Get("X-Token"))
			assert.Equal(t, beszel.Version, options.RequestHeader.Get("X-Beszel"))
			assert.Contains(t, options.RequestHeader.Get("User-Agent"), "Mozilla/5.0")
			assert.False(t, options.TlsConfig.InsecureSkipVerify)

			// Test options caching
			options2 := client.getOptions()
			assert.Same(t, options, options2, "Options should be cached")
		})
	}
}

// TestWebSocketClient_VerifySignature tests signature verification
func TestWebSocketClient_VerifySignature(t *testing.T) {
	agent := createTestAgent(t)

	// Generate test key pairs
	_, goodPrivKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	goodPubKey, err := ssh.NewPublicKey(goodPrivKey.Public().(ed25519.PublicKey))
	require.NoError(t, err)

	_, badPrivKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	badPubKey, err := ssh.NewPublicKey(badPrivKey.Public().(ed25519.PublicKey))
	require.NoError(t, err)

	// Set up environment
	t.Setenv("BESZEL_AGENT_HUB_URL", "http://localhost:8080")
	t.Setenv("BESZEL_AGENT_TOKEN", "test-token")

	client, err := newWebSocketClient(agent)
	require.NoError(t, err)

	testCases := []struct {
		name        string
		keys        []ssh.PublicKey
		token       string
		signWith    ed25519.PrivateKey
		expectError bool
	}{
		{
			name:        "valid signature with correct key",
			keys:        []ssh.PublicKey{goodPubKey},
			token:       "test-token",
			signWith:    goodPrivKey,
			expectError: false,
		},
		{
			name:        "invalid signature with wrong key",
			keys:        []ssh.PublicKey{goodPubKey},
			token:       "test-token",
			signWith:    badPrivKey,
			expectError: true,
		},
		{
			name:        "valid signature with multiple keys",
			keys:        []ssh.PublicKey{badPubKey, goodPubKey},
			token:       "test-token",
			signWith:    goodPrivKey,
			expectError: false,
		},
		{
			name:        "no valid keys",
			keys:        []ssh.PublicKey{badPubKey},
			token:       "test-token",
			signWith:    goodPrivKey,
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up agent with test keys
			agent.keys = tc.keys
			client.token = tc.token

			// Create signature
			signature := ed25519.Sign(tc.signWith, []byte(tc.token))

			err := client.verifySignature(signature)

			if tc.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "invalid signature")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestWebSocketClient_HandleHubRequest tests hub request routing (basic verification logic)
func TestWebSocketClient_HandleHubRequest(t *testing.T) {
	agent := createTestAgent(t)

	// Set up environment
	t.Setenv("BESZEL_AGENT_HUB_URL", "http://localhost:8080")
	t.Setenv("BESZEL_AGENT_TOKEN", "test-token")

	client, err := newWebSocketClient(agent)
	require.NoError(t, err)
	conn := &gws.Conn{}
	client.Conn = conn

	testCases := []struct {
		name        string
		action      common.WebSocketAction
		hubVerified bool
		expectError bool
		errorMsg    string
	}{
		{
			name:        "CheckFingerprint without verification",
			action:      common.CheckFingerprint,
			hubVerified: false,
			expectError: false, // CheckFingerprint is allowed without verification
		},
		{
			name:        "GetData without verification",
			action:      common.GetData,
			hubVerified: false,
			expectError: true,
			errorMsg:    "hub not verified",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client.hubVerified = tc.hubVerified

			// Create minimal request
			hubRequest := &common.HubRequest[cbor.RawMessage]{
				Action: tc.action,
				Data:   cbor.RawMessage{},
			}

			err := client.handleHubRequest(conn, hubRequest, nil)

			if tc.expectError {
				assert.Error(t, err)
				if tc.errorMsg != "" {
					assert.Contains(t, err.Error(), tc.errorMsg)
				}
			} else {
				// For CheckFingerprint, we expect a decode error since we're not providing valid data,
				// but it shouldn't be the "hub not verified" error
				if err != nil && tc.errorMsg != "" {
					assert.NotContains(t, err.Error(), tc.errorMsg)
				}
			}
		})
	}
}

type testHubEventHandler struct {
	gws.BuiltinEventHandler
	messages chan []byte
}

func (h *testHubEventHandler) OnMessage(conn *gws.Conn, message *gws.Message) {
	defer message.Close()
	h.messages <- bytes.Clone(message.Data.Bytes())
}

func newWebSocketTestHub(t *testing.T) (*httptest.Server, <-chan *gws.Conn, <-chan http.Header, <-chan []byte) {
	t.Helper()
	connections := make(chan *gws.Conn, 16)
	headers := make(chan http.Header, 16)
	handler := &testHubEventHandler{messages: make(chan []byte, 16)}
	upgrader := gws.NewUpgrader(handler, nil)
	var mu sync.Mutex
	var sockets []*gws.Conn
	var readLoops sync.WaitGroup
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers <- r.Header.Clone()
		conn, err := upgrader.Upgrade(w, r)
		if err != nil {
			return
		}
		mu.Lock()
		sockets = append(sockets, conn)
		readLoops.Add(1)
		mu.Unlock()
		connections <- conn
		go func() {
			defer readLoops.Done()
			conn.ReadLoop()
		}()
	}))
	t.Cleanup(func() {
		server.Close()
		mu.Lock()
		for _, conn := range sockets {
			_ = conn.WriteClose(1000, nil)
		}
		mu.Unlock()
		readLoops.Wait()
	})
	return server, connections, headers, handler.messages
}

func newWebSocketTestClient(t *testing.T, server *httptest.Server) *WebSocketClient {
	t.Helper()
	t.Setenv("BESZEL_AGENT_HUB_URL", server.URL)
	t.Setenv("BESZEL_AGENT_TOKEN", "test-registration-token")
	client, err := newWebSocketClient(createTestAgent(t))
	require.NoError(t, err)
	client.agent.connectionManager.eventChan = make(chan ConnectionEvent, 16)
	options := client.getOptions()
	options.HandshakeTimeout = 2 * time.Second
	options.NewDialer = func() (gws.Dialer, error) { return &net.Dialer{Timeout: time.Second}, nil }
	t.Cleanup(client.Close)
	return client
}

func newTrustedWebSocketTestClient(t *testing.T, server *httptest.Server) *WebSocketClient {
	t.Helper()
	client := newWebSocketTestClient(t, server)
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	client.getOptions().TlsConfig.RootCAs = roots
	return client
}

func newWebSocketTestAuthRequest(t *testing.T, client *WebSocketClient, id uint32) *common.HubRequest[cbor.RawMessage] {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	publicKey, err := ssh.NewPublicKey(privateKey.Public().(ed25519.PublicKey))
	require.NoError(t, err)
	client.agent.keys = []ssh.PublicKey{publicKey}
	data, err := cbor.Marshal(common.FingerprintRequest{Signature: ed25519.Sign(privateKey, []byte(client.token))})
	require.NoError(t, err)
	return &common.HubRequest[cbor.RawMessage]{Action: common.CheckFingerprint, Data: data, Id: &id}
}

func sendWebSocketTestRequest(t *testing.T, conn *gws.Conn, request *common.HubRequest[cbor.RawMessage]) {
	t.Helper()
	data, err := cbor.Marshal(request)
	require.NoError(t, err)
	require.NoError(t, conn.WriteMessage(gws.OpcodeBinary, data))
}

type webSocketTestDialer struct {
	addr string
}

func (d webSocketTestDialer) Dial(network, _ string) (net.Conn, error) {
	return net.DialTimeout(network, d.addr, time.Second)
}

func receiveWebSocketTestValue[T any](t *testing.T, values <-chan T) T {
	t.Helper()
	select {
	case value := <-values:
		return value
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for WebSocket event")
		var zero T
		return zero
	}
}

func TestWebSocketClientRejectsUntrustedTLSBeforeSendingToken(t *testing.T) {
	server, _, headers, _ := newWebSocketTestHub(t)
	client := newWebSocketTestClient(t, server)

	err := client.Connect()
	var certificateError *tls.CertificateVerificationError
	require.ErrorAs(t, err, &certificateError)
	select {
	case <-headers:
		t.Fatal("untrusted hub received HTTP headers containing the registration token")
	default:
	}
	assert.Nil(t, client.Conn)
	assert.False(t, client.hubVerified)
}

func TestWebSocketClientRejectsWrongTLSHostnameBeforeSendingToken(t *testing.T) {
	server, _, headers, _ := newWebSocketTestHub(t)
	client := newTrustedWebSocketTestClient(t, server)
	options := client.getOptions()
	_, err := server.Certificate().Verify(x509.VerifyOptions{Roots: options.TlsConfig.RootCAs, DNSName: "127.0.0.1"})
	require.NoError(t, err)
	hubURL, err := url.Parse(options.Addr)
	require.NoError(t, err)
	hubURL.Host = net.JoinHostPort("wrong-hub.invalid", hubURL.Port())
	options.Addr = hubURL.String()
	options.NewDialer = func() (gws.Dialer, error) {
		return webSocketTestDialer{addr: server.Listener.Addr().String()}, nil
	}

	err = client.Connect()
	var hostnameError x509.HostnameError
	require.ErrorAs(t, err, &hostnameError)
	assert.Equal(t, "wrong-hub.invalid", hostnameError.Host)
	select {
	case <-headers:
		t.Fatal("hub with mismatched TLS hostname received the registration token")
	default:
	}
	client.mu.Lock()
	assert.Nil(t, client.Conn)
	assert.False(t, client.hubVerified)
	client.mu.Unlock()
}

func TestWebSocketClientRejectsInvalidAuthChallenges(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	publicKey, err := ssh.NewPublicKey(privateKey.Public().(ed25519.PublicKey))
	require.NoError(t, err)
	_, wrongKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testCases := []struct {
		name      string
		challenge any
	}{
		{name: "missing signature", challenge: common.FingerprintRequest{}},
		{name: "truncated signature", challenge: common.FingerprintRequest{Signature: []byte{1, 2, 3}}},
		{name: "wrong key", challenge: common.FingerprintRequest{Signature: ed25519.Sign(wrongKey, []byte("test-registration-token"))}},
		{name: "wrong token", challenge: common.FingerprintRequest{Signature: ed25519.Sign(privateKey, []byte("other-token"))}},
		{name: "malformed challenge", challenge: "not a fingerprint request"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server, connections, _, messages := newWebSocketTestHub(t)
			client := newTrustedWebSocketTestClient(t, server)
			client.agent.keys = []ssh.PublicKey{publicKey}
			var protectedCalls atomic.Int32
			client.agent.handlerRegistry.Register(common.GetData, &MockHandler{handleFunc: func(ctx *HandlerContext) error {
				protectedCalls.Add(1)
				return ctx.SendResponse("protected", ctx.RequestID)
			}})
			invalidData, err := cbor.Marshal(tc.challenge)
			require.NoError(t, err)
			validData, err := cbor.Marshal(common.FingerprintRequest{Signature: ed25519.Sign(privateKey, []byte(client.token))})
			require.NoError(t, err)
			invalidID, protectedID, validID := uint32(1), uint32(2), uint32(3)
			require.NoError(t, client.Connect())
			conn := receiveWebSocketTestValue(t, connections)
			sendWebSocketTestRequest(t, conn, &common.HubRequest[cbor.RawMessage]{Action: common.CheckFingerprint, Data: invalidData, Id: &invalidID})
			sendWebSocketTestRequest(t, conn, &common.HubRequest[cbor.RawMessage]{Action: common.GetData, Id: &protectedID})
			sendWebSocketTestRequest(t, conn, &common.HubRequest[cbor.RawMessage]{Action: common.CheckFingerprint, Data: validData, Id: &validID})

			var response common.AgentResponse
			require.NoError(t, cbor.Unmarshal(receiveWebSocketTestValue(t, messages), &response))
			assert.Equal(t, &validID, response.Id)
			require.NotNil(t, response.Fingerprint)
			assert.Equal(t, client.fingerprint, response.Fingerprint.Fingerprint)
			assert.Zero(t, protectedCalls.Load(), "invalid challenge must not authorize the following protected request")
			assert.Equal(t, WebSocketConnect, receiveWebSocketTestValue(t, client.agent.connectionManager.eventChan))
			select {
			case event := <-client.agent.connectionManager.eventChan:
				t.Fatalf("invalid challenge emitted an extra connection event: %v", event)
			default:
			}
		})
	}
}

func TestWebSocketClientFailedReconnectClearsAuthentication(t *testing.T) {
	server, connections, headers, messages := newWebSocketTestHub(t)
	client := newTrustedWebSocketTestClient(t, server)
	auth := newWebSocketTestAuthRequest(t, client, 1)
	require.NoError(t, client.Connect())
	conn := receiveWebSocketTestValue(t, connections)
	receiveWebSocketTestValue(t, headers)
	sendWebSocketTestRequest(t, conn, auth)
	var response common.AgentResponse
	require.NoError(t, cbor.Unmarshal(receiveWebSocketTestValue(t, messages), &response))
	assert.Equal(t, auth.Id, response.Id)
	require.NotNil(t, response.Fingerprint)
	assert.Equal(t, WebSocketConnect, receiveWebSocketTestValue(t, client.agent.connectionManager.eventChan))
	client.mu.Lock()
	oldConn := client.Conn
	verified := client.hubVerified
	client.mu.Unlock()
	require.True(t, verified)
	options := client.getOptions()
	options.TlsConfig = options.TlsConfig.Clone()
	options.TlsConfig.RootCAs = x509.NewCertPool()

	err := client.Connect()
	var certificateError *tls.CertificateVerificationError
	require.ErrorAs(t, err, &certificateError)
	client.mu.Lock()
	assert.Nil(t, client.Conn)
	assert.Nil(t, client.closingConn)
	assert.False(t, client.hubVerified)
	client.mu.Unlock()
	require.ErrorIs(t, client.handleAuthChallenge(oldConn, auth, auth.Id), gws.ErrConnClosed)
	client.OnClose(oldConn, nil)
	select {
	case <-headers:
		t.Fatal("failed TLS reconnect exposed the registration token")
	default:
	}
	select {
	case event := <-client.agent.connectionManager.eventChan:
		t.Fatalf("failed reconnect emitted a stale connection event: %v", event)
	default:
	}
}

func TestWebSocketClientDuplicateChallengesAndPeerCloseNotifyOnce(t *testing.T) {
	server, connections, _, messages := newWebSocketTestHub(t)
	client := newTrustedWebSocketTestClient(t, server)
	auth := newWebSocketTestAuthRequest(t, client, 1)
	require.NoError(t, client.Connect())
	conn := receiveWebSocketTestValue(t, connections)
	for id := uint32(1); id <= 3; id++ {
		auth.Id = &id
		sendWebSocketTestRequest(t, conn, auth)
		var response common.AgentResponse
		require.NoError(t, cbor.Unmarshal(receiveWebSocketTestValue(t, messages), &response))
		assert.Equal(t, &id, response.Id)
		require.NotNil(t, response.Fingerprint)
		assert.Equal(t, client.fingerprint, response.Fingerprint.Fingerprint)
	}
	assert.Equal(t, WebSocketConnect, receiveWebSocketTestValue(t, client.agent.connectionManager.eventChan))
	select {
	case event := <-client.agent.connectionManager.eventChan:
		t.Fatalf("repeated authentication emitted a duplicate event: %v", event)
	default:
	}
	client.mu.Lock()
	closedConn := client.Conn
	client.mu.Unlock()

	require.NoError(t, conn.WriteClose(1000, nil))
	assert.Equal(t, WebSocketDisconnect, receiveWebSocketTestValue(t, client.agent.connectionManager.eventChan))
	client.mu.Lock()
	assert.Nil(t, client.Conn)
	assert.Nil(t, client.closingConn)
	assert.False(t, client.hubVerified)
	client.mu.Unlock()
	require.ErrorIs(t, client.handleAuthChallenge(closedConn, auth, auth.Id), gws.ErrConnClosed)
	client.OnClose(closedConn, nil)
	client.Close()
	client.Close()
	select {
	case event := <-client.agent.connectionManager.eventChan:
		t.Fatalf("repeated closure emitted a duplicate event: %v", event)
	default:
	}
}

func TestWebSocketClientReauthenticatesEachConnection(t *testing.T) {
	server, connections, headers, messages := newWebSocketTestHub(t)
	client := newWebSocketTestClient(t, server)
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	client.getOptions().TlsConfig.RootCAs = roots
	_, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	publicKey, err := ssh.NewPublicKey(privateKey.Public().(ed25519.PublicKey))
	require.NoError(t, err)
	client.agent.keys = []ssh.PublicKey{publicKey}
	var protectedCalls atomic.Int32
	client.agent.handlerRegistry.Register(common.GetData, &MockHandler{handleFunc: func(ctx *HandlerContext) error {
		protectedCalls.Add(1)
		return ctx.SendResponse("protected", ctx.RequestID)
	}})
	authData, err := cbor.Marshal(common.FingerprintRequest{Signature: ed25519.Sign(privateKey, []byte(client.token))})
	require.NoError(t, err)
	auth := &common.HubRequest[cbor.RawMessage]{Action: common.CheckFingerprint, Data: authData}
	request := &common.HubRequest[cbor.RawMessage]{Action: common.GetData}
	send := func(conn *gws.Conn, request *common.HubRequest[cbor.RawMessage]) {
		t.Helper()
		data, err := cbor.Marshal(request)
		require.NoError(t, err)
		require.NoError(t, conn.WriteMessage(gws.OpcodeBinary, data))
	}
	authenticate := func(conn *gws.Conn) {
		t.Helper()
		send(conn, auth)
		data := receiveWebSocketTestValue(t, messages)
		var response common.FingerprintResponse
		if auth.Id == nil {
			require.NoError(t, cbor.Unmarshal(data, &response))
		} else {
			var envelope common.AgentResponse
			require.NoError(t, cbor.Unmarshal(data, &envelope))
			assert.Equal(t, auth.Id, envelope.Id)
			require.NotNil(t, envelope.Fingerprint)
			response = *envelope.Fingerprint
		}
		assert.Equal(t, client.fingerprint, response.Fingerprint)
		assert.Equal(t, WebSocketConnect, receiveWebSocketTestValue(t, client.agent.connectionManager.eventChan))
	}

	require.NoError(t, client.Connect())
	firstHubConn := receiveWebSocketTestValue(t, connections)
	assert.Equal(t, client.token, receiveWebSocketTestValue(t, headers).Get("X-Token"))
	authenticate(firstHubConn)
	send(firstHubConn, request)
	var protectedResponse string
	require.NoError(t, cbor.Unmarshal(receiveWebSocketTestValue(t, messages), &protectedResponse))
	assert.Equal(t, "protected", protectedResponse)
	assert.EqualValues(t, 1, protectedCalls.Load())
	client.mu.Lock()
	oldConn := client.Conn
	client.mu.Unlock()

	require.NoError(t, client.Connect())
	secondHubConn := receiveWebSocketTestValue(t, connections)
	receiveWebSocketTestValue(t, headers)
	client.OnClose(oldConn, nil)
	require.ErrorIs(t, client.handleAuthChallenge(oldConn, auth, nil), gws.ErrConnClosed)
	require.ErrorIs(t, client.handleHubRequest(oldConn, request, nil), gws.ErrConnClosed)
	require.Error(t, client.sendResponse(oldConn, "stale response", nil))
	send(secondHubConn, request)
	authID := uint32(6)
	auth.Id = &authID
	authenticate(secondHubConn)
	assert.EqualValues(t, 1, protectedCalls.Load(), "request before the new challenge must be rejected")

	id := uint32(7)
	request.Id = &id
	send(secondHubConn, request)
	var response common.AgentResponse
	require.NoError(t, cbor.Unmarshal(receiveWebSocketTestValue(t, messages), &response))
	assert.Equal(t, &id, response.Id)
	assert.Equal(t, &protectedResponse, response.String)
	assert.EqualValues(t, 2, protectedCalls.Load())

	client.mu.Lock()
	closedConn := client.Conn
	client.mu.Unlock()
	client.Close()
	require.ErrorIs(t, client.handleAuthChallenge(closedConn, auth, nil), gws.ErrConnClosed)
	assert.Equal(t, WebSocketDisconnect, receiveWebSocketTestValue(t, client.agent.connectionManager.eventChan))
	client.mu.Lock()
	assert.Nil(t, client.Conn)
	assert.False(t, client.hubVerified)
	client.mu.Unlock()
}

func TestWebSocketClientSerializesConcurrentReconnects(t *testing.T) {
	server, connections, _, _ := newWebSocketTestHub(t)
	client := newWebSocketTestClient(t, server)
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	client.getOptions().TlsConfig.RootCAs = roots
	results := make(chan error, 2)
	for range 2 {
		go func() { results <- client.Connect() }()
	}
	for range 2 {
		require.NoError(t, receiveWebSocketTestValue(t, results))
		receiveWebSocketTestValue(t, connections)
	}
	select {
	case event := <-client.agent.connectionManager.eventChan:
		t.Fatalf("replaced connection emitted a stale event: %v", event)
	default:
	}
	client.Close()
	assert.Equal(t, WebSocketDisconnect, receiveWebSocketTestValue(t, client.agent.connectionManager.eventChan))
}

func TestWebSocketClientCloseWithFullEventChannel(t *testing.T) {
	server, connections, _, _ := newWebSocketTestHub(t)
	client := newWebSocketTestClient(t, server)
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	client.getOptions().TlsConfig.RootCAs = roots
	events := make(chan ConnectionEvent, 1)
	client.agent.connectionManager.eventChan = events
	events <- SSHConnect
	require.NoError(t, client.Connect())
	receiveWebSocketTestValue(t, connections)
	closed := make(chan struct{})
	go func() {
		client.Close()
		close(closed)
	}()
	receiveWebSocketTestValue(t, closed)
	assert.Equal(t, SSHConnect, receiveWebSocketTestValue(t, events))
	assert.Equal(t, WebSocketDisconnect, receiveWebSocketTestValue(t, events))
}

// TestWebSocketClient_GetUserAgent tests user agent generation
func TestGetUserAgent(t *testing.T) {
	// Run multiple times to check both variants
	userAgents := make(map[string]bool)

	for range 20 {
		ua := getUserAgent()
		userAgents[ua] = true

		// Check that it's a valid Mozilla user agent
		assert.Contains(t, ua, "Mozilla/5.0")
		assert.Contains(t, ua, "AppleWebKit/537.36")
		assert.Contains(t, ua, "Chrome/124.0.0.0")
		assert.Contains(t, ua, "Safari/537.36")

		// Should contain either Windows or Mac
		isWindows := strings.Contains(ua, "Windows NT 11.0")
		isMac := strings.Contains(ua, "Macintosh; Intel Mac OS X 14_0_0")
		assert.True(t, isWindows || isMac, "User agent should contain either Windows or Mac identifier")
	}

	// With enough iterations, we should see both variants
	// though this might occasionally fail
	if len(userAgents) == 1 {
		t.Log("Note: Only one user agent variant was generated in this test run")
	}
}

// TestWebSocketClient_Close tests connection closing
func TestWebSocketClient_Close(t *testing.T) {
	agent := createTestAgent(t)

	t.Setenv("BESZEL_AGENT_HUB_URL", "http://localhost:8080")
	t.Setenv("BESZEL_AGENT_TOKEN", "test-token")

	client, err := newWebSocketClient(agent)
	require.NoError(t, err)

	// Test closing with nil connection (should not panic)
	assert.NotPanics(t, func() {
		client.Close()
	})
}

// TestWebSocketClient_ConnectRateLimit tests connection rate limiting
func TestWebSocketClient_ConnectRateLimit(t *testing.T) {
	agent := createTestAgent(t)

	t.Setenv("BESZEL_AGENT_HUB_URL", "http://localhost:8080")
	t.Setenv("BESZEL_AGENT_TOKEN", "test-token")

	client, err := newWebSocketClient(agent)
	require.NoError(t, err)

	// Set recent connection attempt
	client.lastConnectAttempt = time.Now()

	// Test that connection fails quickly due to rate limiting
	// This won't actually connect but should fail fast
	err = client.Connect()
	assert.Error(t, err, "Connection should fail but not hang")
}

// TestGetToken tests the getToken function with various scenarios
func TestGetToken(t *testing.T) {
	t.Run("token from TOKEN environment variable", func(t *testing.T) {
		// Set TOKEN env var
		expectedToken := "test-token-from-env"
		t.Setenv("TOKEN", expectedToken)

		token, err := getToken()
		assert.NoError(t, err)
		assert.Equal(t, expectedToken, token)
	})

	t.Run("token from BESZEL_AGENT_TOKEN environment variable", func(t *testing.T) {
		// Set BESZEL_AGENT_TOKEN env var (should take precedence)
		expectedToken := "test-token-from-beszel-env"
		t.Setenv("BESZEL_AGENT_TOKEN", expectedToken)

		token, err := getToken()
		assert.NoError(t, err)
		assert.Equal(t, expectedToken, token)
	})

	t.Run("token from TOKEN_FILE", func(t *testing.T) {
		// Create a temporary token file
		expectedToken := "test-token-from-file"
		tokenFile, err := os.CreateTemp("", "token-test-*.txt")
		require.NoError(t, err)
		defer os.Remove(tokenFile.Name())

		_, err = tokenFile.WriteString(expectedToken)
		require.NoError(t, err)
		tokenFile.Close()

		// Set TOKEN_FILE env var
		t.Setenv("TOKEN_FILE", tokenFile.Name())

		token, err := getToken()
		assert.NoError(t, err)
		assert.Equal(t, expectedToken, token)
	})

	t.Run("token from BESZEL_AGENT_TOKEN_FILE", func(t *testing.T) {
		// Create a temporary token file
		expectedToken := "test-token-from-beszel-file"
		tokenFile, err := os.CreateTemp("", "token-test-*.txt")
		require.NoError(t, err)
		defer os.Remove(tokenFile.Name())

		_, err = tokenFile.WriteString(expectedToken)
		require.NoError(t, err)
		tokenFile.Close()

		// Set BESZEL_AGENT_TOKEN_FILE env var (should take precedence)
		t.Setenv("BESZEL_AGENT_TOKEN_FILE", tokenFile.Name())

		token, err := getToken()
		assert.NoError(t, err)
		assert.Equal(t, expectedToken, token)
	})

	t.Run("TOKEN takes precedence over TOKEN_FILE", func(t *testing.T) {
		// Create a temporary token file
		fileToken := "token-from-file"
		tokenFile, err := os.CreateTemp("", "token-test-*.txt")
		require.NoError(t, err)
		defer os.Remove(tokenFile.Name())

		_, err = tokenFile.WriteString(fileToken)
		require.NoError(t, err)
		tokenFile.Close()

		// Set both TOKEN and TOKEN_FILE
		envToken := "token-from-env"
		t.Setenv("TOKEN", envToken)
		t.Setenv("TOKEN_FILE", tokenFile.Name())

		token, err := getToken()
		assert.NoError(t, err)
		assert.Equal(t, envToken, token, "TOKEN should take precedence over TOKEN_FILE")
	})

	t.Run("error when neither TOKEN nor TOKEN_FILE is set", func(t *testing.T) {
		t.Setenv("BESZEL_AGENT_TOKEN", "")
		t.Setenv("TOKEN", "")
		t.Setenv("BESZEL_AGENT_TOKEN_FILE", "")
		t.Setenv("TOKEN_FILE", "")

		token, err := getToken()
		assert.Error(t, err)
		assert.Equal(t, "", token)
		assert.Contains(t, err.Error(), "must set TOKEN or TOKEN_FILE")
	})

	t.Run("error when TOKEN_FILE points to non-existent file", func(t *testing.T) {
		t.Setenv("TOKEN_FILE", filepath.Join(t.TempDir(), "missing-token.txt"))

		token, err := getToken()
		assert.Error(t, err)
		assert.Equal(t, "", token)
		assert.ErrorIs(t, err, fs.ErrNotExist)
	})

	t.Run("handles empty token file", func(t *testing.T) {
		// Create an empty token file
		tokenFile, err := os.CreateTemp("", "token-test-*.txt")
		require.NoError(t, err)
		defer os.Remove(tokenFile.Name())
		tokenFile.Close()

		// Set TOKEN_FILE env var
		t.Setenv("TOKEN_FILE", tokenFile.Name())

		token, err := getToken()
		assert.NoError(t, err)
		assert.Equal(t, "", token, "Empty file should return empty string")
	})

	t.Run("strips whitespace from TOKEN_FILE", func(t *testing.T) {
		tokenWithWhitespace := "  test-token-with-whitespace  \n\t"
		expectedToken := "test-token-with-whitespace"
		tokenFile, err := os.CreateTemp("", "token-test-*.txt")
		require.NoError(t, err)
		defer os.Remove(tokenFile.Name())

		_, err = tokenFile.WriteString(tokenWithWhitespace)
		require.NoError(t, err)
		tokenFile.Close()

		t.Setenv("TOKEN_FILE", tokenFile.Name())

		token, err := getToken()
		assert.NoError(t, err)
		assert.Equal(t, expectedToken, token, "Whitespace should be stripped from token file content")
	})
}
