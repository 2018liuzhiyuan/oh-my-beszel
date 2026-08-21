package systems

import "testing"

func TestIsDirectHost(t *testing.T) {
	direct := []string{"", "localhost", "LOCALHOST", "127.0.0.1", "100.73.31.115", "::1", "0:0:0:0:0:0:0:1", "/tmp/sock"}
	for _, h := range direct {
		if !isDirectHost(h) {
			t.Errorf("expected %q to be direct", h)
		}
	}
	aliases := []string{"h101", "beszel-meta-006", "some-host.example.com"}
	for _, h := range aliases {
		if isDirectHost(h) {
			t.Errorf("expected %q to go through ssh", h)
		}
	}
}
