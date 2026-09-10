//go:build testing

package systems

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

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

func TestCreateSessionReturnsErrorDuringClose(t *testing.T) {
	for range 500 {
		// Given
		sys := &System{ctx: t.Context()}
		sys.client.Store(&ssh.Client{Conn: &closedSSHConn{}})

		// When
		var workers sync.WaitGroup
		workers.Add(2)
		go func() {
			defer workers.Done()
			session, err := sys.createSessionWithTimeout(time.Second)
			// Then
			assert.Nil(t, session)
			assert.Error(t, err)
		}()
		go func() {
			defer workers.Done()
			sys.closeSSHConnection()
		}()
		workers.Wait()
	}
}

func TestCloseSSHConnectionClosesOnceDuringConcurrentShutdown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Given
		release := make(chan struct{})
		conn := &closedSSHConn{release: release}
		sys := &System{ctx: t.Context()}
		sys.client.Store(&ssh.Client{Conn: conn})

		// When
		go sys.closeSSHConnection()
		go sys.closeSSHConnection()
		synctest.Wait()
		close(release)
		synctest.Wait()

		// Then
		require.EqualValues(t, 1, conn.closes.Load())
	})
}
