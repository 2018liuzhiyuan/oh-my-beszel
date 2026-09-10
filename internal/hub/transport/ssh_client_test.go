package transport

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/henrygd/beszel/internal/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

type closedSSHConn struct {
	ssh.Conn
	closes  atomic.Int32
	release <-chan struct{}
}

func (*closedSSHConn) OpenChannel(string, []byte) (ssh.Channel, <-chan *ssh.Request, error) {
	return nil, nil, errors.New("use of closed network connection")
}

func (conn *closedSSHConn) Close() error {
	conn.closes.Add(1)
	if conn.release != nil {
		<-conn.release
	}
	return nil
}

func TestSSHTransportRequestReturnsErrorDuringClose(t *testing.T) {
	for range 500 {
		// Given
		transport := NewSSHTransport(SSHTransportConfig{Timeout: time.Second})
		transport.SetClient(&ssh.Client{Conn: &closedSSHConn{}})

		// When
		var workers sync.WaitGroup
		workers.Add(2)
		go func() {
			defer workers.Done()
			var response common.AgentResponse
			err := transport.Request(t.Context(), common.GetData, nil, &response)
			// Then
			assert.Error(t, err)
		}()
		go func() {
			defer workers.Done()
			transport.Close()
		}()
		workers.Wait()
	}
}

func TestSSHTransportClosesOnceDuringConcurrentShutdown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Given
		release := make(chan struct{})
		conn := &closedSSHConn{release: release}
		transport := NewSSHTransport(SSHTransportConfig{})
		transport.SetClient(&ssh.Client{Conn: conn})

		// When
		go transport.Close()
		go transport.Close()
		synctest.Wait()
		close(release)
		synctest.Wait()

		// Then
		require.EqualValues(t, 1, conn.closes.Load())
		require.False(t, transport.IsConnected())
	})
}
