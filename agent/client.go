package agent

import (
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/henrygd/beszel/agent/utils"
	"github.com/henrygd/beszel/internal/beszel"
	"github.com/henrygd/beszel/internal/common"

	"github.com/fxamacker/cbor/v2"
	"github.com/lxzan/gws"
	"golang.org/x/crypto/ssh"
	"golang.org/x/net/proxy"
)

const (
	wsDeadline = 70 * time.Second
)

// WebSocketClient manages the WebSocket connection between the agent and hub.
// It handles authentication, message routing, and connection lifecycle management.
type WebSocketClient struct {
	gws.BuiltinEventHandler
	options            *gws.ClientOption // WebSocket client configuration options
	agent              *Agent            // Reference to the parent agent
	Conn               *gws.Conn         // Active WebSocket connection
	closingConn        *gws.Conn
	hubURL             *url.URL   // Parsed hub URL for connection
	token              string     // Authentication token for hub registration
	fingerprint        string     // System fingerprint for identification
	lastConnectAttempt time.Time  // Timestamp of last connection attempt
	hubVerified        bool       // Whether the hub has been cryptographically verified
	connectMu          sync.Mutex // Serializes connection replacement and explicit closure
	mu                 sync.Mutex // Protects Conn, closingConn, hubVerified, and lastConnectAttempt
}

// newWebSocketClient creates a new WebSocket client for the given agent.
// It reads configuration from environment variables and validates the hub URL.
func newWebSocketClient(agent *Agent) (client *WebSocketClient, err error) {
	hubURLStr, exists := utils.GetEnv("HUB_URL")
	if !exists {
		return nil, errors.New("HUB_URL environment variable not set")
	}

	client = &WebSocketClient{}

	client.hubURL, err = url.Parse(hubURLStr)
	if err != nil || client.hubURL.Host == "" {
		return nil, fmt.Errorf("invalid HUB_URL %q: must include scheme and host (e.g. http://hub.example.com:8090)", hubURLStr)
	}
	// get registration token
	client.token, err = getToken()
	if err != nil {
		return nil, err
	}

	client.agent = agent
	client.fingerprint = agent.getFingerprint()

	return client, nil
}

// getToken returns the token for the WebSocket client.
// It first checks the TOKEN environment variable, then the TOKEN_FILE environment variable.
// If neither is set, it returns an error.
func getToken() (string, error) {
	// get token from env var
	token, _ := utils.GetEnv("TOKEN")
	if token != "" {
		return token, nil
	}
	// get token from file
	tokenFile, _ := utils.GetEnv("TOKEN_FILE")
	if tokenFile == "" {
		return "", errors.New("must set TOKEN or TOKEN_FILE")
	}
	tokenBytes, err := os.ReadFile(tokenFile)
	if err != nil {
		return "", err
	}
	return parseTokenFile(string(tokenBytes), tokenFile)
}

// getOptions returns the WebSocket client options, creating them if necessary.
// It configures the connection URL, TLS settings, and authentication headers.
func (client *WebSocketClient) getOptions() *gws.ClientOption {
	if client.options != nil {
		return client.options
	}

	// update the hub url to use websocket scheme and api path
	if client.hubURL.Scheme == "https" {
		client.hubURL.Scheme = "wss"
	} else {
		client.hubURL.Scheme = "ws"
	}
	client.hubURL.Path = path.Join(client.hubURL.Path, "api/beszel/agent-connect")

	// make sure BESZEL_AGENT_ALL_PROXY works (GWS only checks ALL_PROXY)
	if val := os.Getenv("BESZEL_AGENT_ALL_PROXY"); val != "" {
		os.Setenv("ALL_PROXY", val)
	}

	client.options = &gws.ClientOption{
		Addr:      client.hubURL.String(),
		TlsConfig: &tls.Config{},
		RequestHeader: http.Header{
			"User-Agent": []string{getUserAgent()},
			"X-Token":    []string{client.token},
			"X-Beszel":   []string{beszel.Version},
		},
		NewDialer: func() (gws.Dialer, error) {
			return proxy.FromEnvironment(), nil
		},
	}
	return client.options
}

// Connect establishes a WebSocket connection to the hub.
// It closes any existing connection before attempting to reconnect.
func (client *WebSocketClient) Connect() (err error) {
	client.connectMu.Lock()
	defer client.connectMu.Unlock()

	client.mu.Lock()
	client.lastConnectAttempt = time.Now()
	previousConn := client.Conn
	client.Conn = nil
	client.closingConn = nil
	client.hubVerified = false
	client.mu.Unlock()

	// make sure previous connection is closed
	if previousConn != nil {
		_ = previousConn.WriteClose(1000, nil)
	}

	conn, _, err := gws.NewClient(client, client.getOptions())
	if err != nil {
		return err
	}

	client.mu.Lock()
	client.Conn = conn
	client.mu.Unlock()
	go conn.ReadLoop()

	return nil
}

func (client *WebSocketClient) timeSinceLastConnectAttempt() time.Duration {
	client.mu.Lock()
	defer client.mu.Unlock()
	return time.Since(client.lastConnectAttempt)
}

// OnOpen handles WebSocket connection establishment.
// It sets a deadline for the connection to prevent hanging.
func (client *WebSocketClient) OnOpen(conn *gws.Conn) {
	conn.SetDeadline(time.Now().Add(wsDeadline))
}

// OnClose handles WebSocket connection closure.
// It logs the closure reason and notifies the connection manager.
func (client *WebSocketClient) OnClose(conn *gws.Conn, err error) {
	client.mu.Lock()
	if conn == nil || (client.Conn != conn && client.closingConn != conn) {
		client.mu.Unlock()
		return
	}
	client.Conn = nil
	client.closingConn = nil
	client.hubVerified = false
	client.mu.Unlock()

	if err != nil {
		slog.Warn("Connection closed", "err", strings.TrimPrefix(err.Error(), "gws: "))
	}
	client.agent.connectionManager.eventChan <- WebSocketDisconnect
}

// OnMessage handles incoming WebSocket messages from the hub.
// It decodes CBOR messages and routes them to appropriate handlers.
func (client *WebSocketClient) OnMessage(conn *gws.Conn, message *gws.Message) {
	defer message.Close()
	conn.SetDeadline(time.Now().Add(wsDeadline))

	if message.Opcode != gws.OpcodeBinary {
		return
	}

	var HubRequest common.HubRequest[cbor.RawMessage]

	err := cbor.Unmarshal(message.Data.Bytes(), &HubRequest)
	if err != nil {
		slog.Error("Error parsing message", "err", err)
		return
	}

	if err := client.handleHubRequest(conn, &HubRequest, HubRequest.Id); err != nil {
		slog.Error("Error handling message", "err", err)
	}
}

// OnPing handles WebSocket ping frames.
// It responds with a pong and updates the connection deadline.
func (client *WebSocketClient) OnPing(conn *gws.Conn, message []byte) {
	conn.SetDeadline(time.Now().Add(wsDeadline))
	conn.WritePong(message)
}

// handleAuthChallenge verifies the authenticity of the hub and returns the system's fingerprint.
func (client *WebSocketClient) handleAuthChallenge(conn *gws.Conn, msg *common.HubRequest[cbor.RawMessage], requestID *uint32) (err error) {
	var authRequest common.FingerprintRequest
	if err := cbor.Unmarshal(msg.Data, &authRequest); err != nil {
		return err
	}

	if err := client.verifySignature(authRequest.Signature); err != nil {
		return err
	}

	client.mu.Lock()
	if conn == nil || client.Conn != conn {
		client.mu.Unlock()
		return gws.ErrConnClosed
	}
	wasVerified := client.hubVerified
	client.hubVerified = true
	client.mu.Unlock()
	if !wasVerified {
		client.agent.connectionManager.eventChan <- WebSocketConnect
	}

	response := &common.FingerprintResponse{
		Fingerprint: client.fingerprint,
	}

	if authRequest.NeedSysInfo {
		response.Name, _ = utils.GetEnv("SYSTEM_NAME")
		response.Hostname = client.agent.systemDetails.Hostname
		serverAddr := client.agent.connectionManager.serverOptions.Addr
		_, response.Port, _ = net.SplitHostPort(serverAddr)
	}

	return client.sendResponse(conn, response, requestID)
}

// verifySignature verifies the signature of the token using the public keys.
func (client *WebSocketClient) verifySignature(signature []byte) (err error) {
	for _, pubKey := range client.agent.keys {
		sig := ssh.Signature{
			Format: pubKey.Type(),
			Blob:   signature,
		}
		if err = pubKey.Verify([]byte(client.token), &sig); err == nil {
			return nil
		}
	}
	return errors.New("invalid signature - check KEY value")
}

// Close closes the WebSocket connection gracefully.
// This method is safe to call multiple times.
func (client *WebSocketClient) Close() {
	client.connectMu.Lock()
	defer client.connectMu.Unlock()
	client.mu.Lock()
	conn := client.Conn
	if conn != nil {
		client.closingConn = conn
	}
	client.Conn = nil
	client.hubVerified = false
	client.mu.Unlock()
	if conn != nil {
		_ = conn.WriteClose(1000, nil)
	}
}

// handleHubRequest routes the request to the appropriate handler using the handler registry.
func (client *WebSocketClient) handleHubRequest(conn *gws.Conn, msg *common.HubRequest[cbor.RawMessage], requestID *uint32) error {
	client.mu.Lock()
	if conn == nil || client.Conn != conn {
		client.mu.Unlock()
		return gws.ErrConnClosed
	}
	hubVerified := client.hubVerified
	client.mu.Unlock()

	// Authentication and replies belong to the originating connection, including during reconnects.
	ctx := &HandlerContext{
		Client:      client,
		Conn:        conn,
		Agent:       client.agent,
		Request:     msg,
		RequestID:   requestID,
		HubVerified: hubVerified,
		SendResponse: func(data any, requestID *uint32) error {
			return client.sendResponse(conn, data, requestID)
		},
	}
	return client.agent.handlerRegistry.Handle(ctx)
}

// sendMessage encodes the given data to CBOR and sends it as a binary message over the WebSocket connection to the hub.
func (client *WebSocketClient) sendMessage(conn *gws.Conn, data any) error {
	bytes, err := cbor.Marshal(data)
	if err != nil {
		return err
	}
	err = conn.WriteMessage(gws.OpcodeBinary, bytes)
	if err != nil {
		client.OnClose(conn, err)
	}
	return err
}

// sendResponse sends a response with optional request ID.
// For ID-based requests, we must populate legacy typed fields for backward
// compatibility with older hubs (<= 0.17) that don't read the generic Data field.
func (client *WebSocketClient) sendResponse(conn *gws.Conn, data any, requestID *uint32) error {
	if requestID != nil {
		response := newAgentResponse(data, requestID)
		return client.sendMessage(conn, response)
	}
	// Legacy format - send data directly
	return client.sendMessage(conn, data)
}

// getUserAgent returns one of two User-Agent strings based on current time.
// This is used to avoid being blocked by Cloudflare or other anti-bot measures.
func getUserAgent() string {
	const (
		uaBase    = "Mozilla/5.0 (%s) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
		uaWindows = "Windows NT 11.0; Win64; x64"
		uaMac     = "Macintosh; Intel Mac OS X 14_0_0"
	)
	if time.Now().UnixNano()%2 == 0 {
		return fmt.Sprintf(uaBase, uaWindows)
	}
	return fmt.Sprintf(uaBase, uaMac)
}
