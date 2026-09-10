package agent

import (
	"fmt"
	"strings"
)

func parseTokenFile(contents, path string) (string, error) {
	var token string
	for line := range strings.Lines(contents) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if token != "" {
			return "", fmt.Errorf("%s must contain a single token", path)
		}
		token = line
	}
	return token, nil
}
