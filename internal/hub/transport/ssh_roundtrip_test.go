package transport

import (
	"net"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
	sshd "github.com/gliderlabs/ssh"
	"github.com/henrygd/beszel/internal/common"
	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func TestSSHTransportRequestsAfterConnectionCloses(t *testing.T) {
	// Given
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server := &sshd.Server{
		Version: "beszel_0.19.0",
		Handler: func(session sshd.Session) {
			var request common.HubRequest[common.DataRequestOptions]
			if err := cbor.NewDecoder(session).Decode(&request); err != nil {
				t.Errorf("decode SSH request: %v", err)
				return
			}
			if request.Action != common.GetData {
				t.Errorf("unexpected SSH action: %v", request.Action)
				return
			}
			response := common.AgentResponse{SystemData: &system.CombinedData{
				Stats: system.Stats{Cpu: 42},
			}}
			if err := cbor.NewEncoder(session).Encode(response); err != nil {
				t.Errorf("encode SSH response: %v", err)
				return
			}
			if err := session.Exit(0); err != nil {
				t.Errorf("exit SSH session: %v", err)
			}
		},
	}
	serverDone := make(chan error, 1)
	go func() { serverDone <- server.Serve(listener) }()
	t.Cleanup(func() {
		require.NoError(t, server.Close())
		require.ErrorIs(t, <-serverDone, sshd.ErrServerClosed)
	})
	host, port, err := net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err)
	transport := NewSSHTransport(SSHTransportConfig{
		Host: host,
		Port: port,
		Config: &ssh.ClientConfig{
			User:            "beszel",
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         5 * time.Second,
		},
	})
	t.Cleanup(transport.Close)
	var initial system.CombinedData
	require.NoError(t, transport.Request(t.Context(), common.GetData, nil, &initial))
	require.Equal(t, float64(42), initial.Stats.Cpu)
	require.NoError(t, transport.GetClient().Close())

	// When
	var recovered system.CombinedData
	err = transport.RequestWithRetry(t.Context(), common.GetData, nil, &recovered, 1)

	// Then
	require.NoError(t, err)
	require.Equal(t, float64(42), recovered.Stats.Cpu)
	require.Equal(t, "0.19.0", transport.GetAgentVersion().String())
}
