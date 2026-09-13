package systems

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFormatStatusInfo(t *testing.T) {
	require.Equal(t, "", FormatStatusInfo(nil))
	require.Equal(t, "dial tcp 127.0.0.1:45876: connect: connection refused", FormatStatusInfo(errors.New("dial tcp 127.0.0.1:45876: connect: connection refused")))
	// only the first line is kept, whitespace-normalized
	require.Equal(
		t,
		"ssh: handshake failed: EOF; ssh: Could not resolve hostname gpu-node",
		FormatStatusInfo(errors.New("ssh: handshake failed: EOF; ssh: Could not resolve hostname gpu-node\r\nssh exited with status 255")),
	)
	// long errors are truncated to the storage bound
	long := strings.Repeat("x", maxStatusInfoLength+50)
	require.Len(t, FormatStatusInfo(errors.New(long)), maxStatusInfoLength)
}
