//go:build testing

package systems

import (
	"net"
	"sync"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
	sshd "github.com/gliderlabs/ssh"
	"github.com/henrygd/beszel/internal/common"
	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/henrygd/beszel/internal/hub/transport"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func TestSharedSSHClientReconnectsAfterConcurrentClose(t *testing.T) {
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
	sys := &System{
		ctx: t.Context(),
		sshTransport: transport.NewSSHTransport(transport.SSHTransportConfig{
			Host: host,
			Port: port,
			Config: &ssh.ClientConfig{
				User:            "beszel",
				HostKeyCallback: ssh.InsecureIgnoreHostKey(),
				Timeout:         5 * time.Second,
			},
		}),
	}
	t.Cleanup(sys.closeSSHConnection)
	var initial system.CombinedData
	require.NoError(t, sys.request(t.Context(), common.GetData, nil, &initial))
	originalClient := sys.client.Load()
	require.NotNil(t, originalClient)
	require.Same(t, originalClient, sys.sshTransport.GetClient())

	// When
	var closers sync.WaitGroup
	for range 2 {
		closers.Go(sys.closeSSHConnection)
	}
	closers.Wait()

	// Then
	require.Nil(t, sys.client.Load())
	require.False(t, sys.sshTransport.IsConnected())
	var recovered system.CombinedData
	require.NoError(t, sys.request(t.Context(), common.GetData, nil, &recovered))
	require.Equal(t, float64(42), recovered.Stats.Cpu)
	require.NotSame(t, originalClient, sys.client.Load())
	require.Same(t, sys.client.Load(), sys.sshTransport.GetClient())
}
