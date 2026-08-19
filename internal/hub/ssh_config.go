package hub

import (
	"bufio"
	"io"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

type sshHost struct {
	Name     string `json:"name"`
	HostName string `json:"hostName"`
}

func getSSHHosts(e *core.RequestEvent) error {
	hosts, err := readLocalSSHHosts()
	if err != nil {
		return e.InternalServerError("Unable to read the local SSH config.", nil)
	}
	return e.JSON(http.StatusOK, map[string][]sshHost{"hosts": hosts})
}

func readLocalSSHHosts() ([]sshHost, error) {
	// explicit override for deployments where the config lives elsewhere
	if path := strings.TrimSpace(os.Getenv("SSH_CONFIG_PATH")); path != "" {
		return readSSHHosts(path)
	}
	homes := make([]string, 0, 5)
	if home, err := os.UserHomeDir(); err == nil {
		homes = append(homes, home)
	}
	if currentUser, err := user.Current(); err == nil {
		homes = append(homes, currentUser.HomeDir)
	}
	homes = append(homes, os.Getenv("USERPROFILE"))
	if runtime.GOOS == "windows" {
		systemDrive := os.Getenv("SystemDrive")
		if systemDrive == "" {
			systemDrive = "C:"
		}
		if username := os.Getenv("USERNAME"); username != "" {
			homes = append(homes, filepath.Join(systemDrive+string(os.PathSeparator), "Users", username))
		}
	}

	seen := make(map[string]struct{})
	for _, home := range homes {
		if home == "" {
			continue
		}
		path := filepath.Join(home, ".ssh", "config")
		lookup := strings.ToLower(filepath.Clean(path))
		if _, exists := seen[lookup]; exists {
			continue
		}
		seen[lookup] = struct{}{}
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			return readSSHHosts(path)
		}
	}
	return []sshHost{}, nil
}

func readSSHHosts(path string) ([]sshHost, error) {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return []sshHost{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return parseSSHHosts(file)
}

func parseSSHHosts(reader io.Reader) ([]sshHost, error) {
	hosts := make([]sshHost, 0)
	byName := make(map[string]int)
	current := make([]int, 0)

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		key, value := splitSSHDirective(scanner.Text())
		switch strings.ToLower(key) {
		case "host":
			current = current[:0]
			for _, alias := range splitSSHWords(value) {
				if alias == "" || strings.ContainsAny(alias, "*?!") {
					continue
				}
				lookup := strings.ToLower(alias)
				index, exists := byName[lookup]
				if !exists {
					index = len(hosts)
					byName[lookup] = index
					hosts = append(hosts, sshHost{Name: alias})
				}
				current = append(current, index)
			}
		case "hostname":
			valueWords := splitSSHWords(value)
			if len(valueWords) == 0 {
				continue
			}
			for _, index := range current {
				if hosts[index].HostName == "" {
					hosts[index].HostName = expandSSHHostName(valueWords[0], hosts[index].Name)
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	for index := range hosts {
		if hosts[index].HostName == "" {
			hosts[index].HostName = hosts[index].Name
		}
	}
	sort.Slice(hosts, func(i, j int) bool {
		return strings.ToLower(hosts[i].Name) < strings.ToLower(hosts[j].Name)
	})
	return hosts, nil
}

func splitSSHDirective(line string) (string, string) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", ""
	}
	separator := strings.IndexAny(line, " \t=")
	if separator < 0 {
		return line, ""
	}
	key := line[:separator]
	value := strings.TrimLeft(line[separator:], " \t=")
	return key, value
}

func splitSSHWords(value string) []string {
	words := make([]string, 0)
	var word strings.Builder
	var quote rune
	escaped := false
	flush := func() {
		if word.Len() > 0 {
			words = append(words, word.String())
			word.Reset()
		}
	}
	for _, char := range value {
		if escaped {
			word.WriteRune(char)
			escaped = false
			continue
		}
		if char == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if char == quote {
				quote = 0
			} else {
				word.WriteRune(char)
			}
			continue
		}
		switch char {
		case '\'', '"':
			quote = char
		case '#':
			flush()
			return words
		case ' ', '\t':
			flush()
		default:
			word.WriteRune(char)
		}
	}
	if escaped {
		word.WriteRune('\\')
	}
	flush()
	return words
}

func expandSSHHostName(hostName, alias string) string {
	hostName = strings.ReplaceAll(hostName, "%h", alias)
	hostName = strings.ReplaceAll(hostName, "%n", alias)
	return strings.ReplaceAll(hostName, "%%", "%")
}
