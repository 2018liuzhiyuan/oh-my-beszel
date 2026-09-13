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

type sshHostsResponse struct {
	Path  string    `json:"path"`
	Hosts []sshHost `json:"hosts"`
}

func getSSHHosts(e *core.RequestEvent) error {
	requestedPath := strings.TrimSpace(e.Request.URL.Query().Get("path"))
	path := resolveSSHConfigPath(requestedPath)
	hosts, err := readSSHHosts(path)
	if err != nil {
		e.App.Logger().Warn("Unable to read the local SSH config", "path", path, "err", err)
		return e.InternalServerError("Unable to read the local SSH config.", nil)
	}
	return e.JSON(http.StatusOK, sshHostsResponse{Path: path, Hosts: hosts})
}

func readLocalSSHHosts() ([]sshHost, error) {
	return readSSHHosts(resolveSSHConfigPath(""))
}

func resolveSSHConfigPath(requestedPath string) string {
	homes := sshHomeCandidates()
	home := ""
	for _, candidate := range homes {
		if strings.TrimSpace(candidate) != "" {
			home = candidate
			break
		}
	}
	if requestedPath != "" {
		return expandSSHConfigPath(requestedPath, home)
	}
	if configuredPath := strings.TrimSpace(os.Getenv("SSH_CONFIG_PATH")); configuredPath != "" {
		return expandSSHConfigPath(configuredPath, home)
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
			return path
		}
	}
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".ssh", "config")
}

// sshHomeCandidates lists home directories in priority order for resolving
// `~` and the default config location. On Windows the list tolerates service
// accounts where USERPROFILE is empty by falling back to the profile implied
// by SystemDrive and USERNAME.
func sshHomeCandidates() []string {
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
	return homes
}

func expandSSHConfigPath(path, home string) string {
	path = os.ExpandEnv(strings.TrimSpace(path))
	if path == "~" {
		path = home
	} else if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		path = filepath.Join(home, path[2:])
	}
	return filepath.Clean(path)
}

// maxSSHConfigIncludeDepth bounds recursive Include processing so a cyclic
// set of config files cannot stall the hub.
const maxSSHConfigIncludeDepth = 8

func readSSHHosts(path string) ([]sshHost, error) {
	parser := newSSHConfigParser()
	if err := parser.parseFile(path, 0, true); err != nil {
		return nil, err
	}
	return parser.finish(), nil
}

// parseSSHHosts parses a single stream. Includes with relative paths are
// skipped because the containing file's directory is unknown; file-based
// parsing in readSSHHosts resolves them like OpenSSH does.
func parseSSHHosts(reader io.Reader) ([]sshHost, error) {
	parser := newSSHConfigParser()
	if err := parser.parseReader(reader, "", 0); err != nil {
		return nil, err
	}
	return parser.finish(), nil
}

type sshConfigParser struct {
	hosts   []sshHost
	byName  map[string]int
	current []int
	home    string
	seen    map[string]struct{}
}

func newSSHConfigParser() *sshConfigParser {
	home := ""
	for _, candidate := range sshHomeCandidates() {
		if strings.TrimSpace(candidate) != "" {
			home = candidate
			break
		}
	}
	return &sshConfigParser{
		byName: make(map[string]int),
		home:   home,
		seen:   make(map[string]struct{}),
	}
}

// parseFile reads one config file. Missing files and unreadable includes are
// skipped like OpenSSH; when strict is true an existing-but-unopenable top
// level file is reported so users can distinguish a permissions problem from
// an absent config.
func (p *sshConfigParser) parseFile(path string, depth int, strict bool) error {
	if path == "" || depth > maxSSHConfigIncludeDepth {
		return nil
	}
	lookup := strings.ToLower(filepath.Clean(path))
	if _, included := p.seen[lookup]; included {
		return nil
	}
	p.seen[lookup] = struct{}{}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil
	}
	file, err := os.Open(path)
	if err != nil {
		if strict {
			return err
		}
		return nil
	}
	defer file.Close()
	return p.parseReader(file, filepath.Dir(path), depth)
}

func (p *sshConfigParser) parseReader(reader io.Reader, baseDir string, depth int) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		key, value := splitSSHDirective(scanner.Text())
		switch strings.ToLower(key) {
		case "host":
			p.current = p.current[:0]
			for _, alias := range splitSSHWords(value) {
				if alias == "" || strings.ContainsAny(alias, "*?!") {
					continue
				}
				lookup := strings.ToLower(alias)
				index, exists := p.byName[lookup]
				if !exists {
					index = len(p.hosts)
					p.byName[lookup] = index
					p.hosts = append(p.hosts, sshHost{Name: alias})
				}
				p.current = append(p.current, index)
			}
		case "hostname":
			valueWords := splitSSHWords(value)
			if len(valueWords) == 0 {
				continue
			}
			for _, index := range p.current {
				if p.hosts[index].HostName == "" {
					p.hosts[index].HostName = expandSSHHostName(valueWords[0], p.hosts[index].Name)
				}
			}
		case "include":
			if err := p.parseIncludes(value, baseDir, depth); err != nil {
				return err
			}
		}
	}
	return scanner.Err()
}

// parseIncludes expands one Include directive at the point it appears, so the
// first-wins precedence across the merged files matches OpenSSH. Each word may
// be an absolute path, a `~` path, an environment-variable path, or a relative
// path resolved against the including file's directory; wildcards follow
// filepath.Glob and patterns without matches are ignored.
func (p *sshConfigParser) parseIncludes(value, baseDir string, depth int) error {
	for _, word := range splitSSHWords(value) {
		if word == "" {
			continue
		}
		path := expandSSHConfigPath(word, p.home)
		if !filepath.IsAbs(path) {
			if baseDir == "" {
				continue
			}
			path = filepath.Join(baseDir, path)
		}
		matches, err := filepath.Glob(path)
		if err != nil {
			continue
		}
		for _, match := range matches {
			if err := p.parseFile(match, depth+1, false); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *sshConfigParser) finish() []sshHost {
	if p.hosts == nil {
		return []sshHost{}
	}
	for index := range p.hosts {
		if p.hosts[index].HostName == "" {
			p.hosts[index].HostName = p.hosts[index].Name
		}
	}
	sort.Slice(p.hosts, func(i, j int) bool {
		return strings.ToLower(p.hosts[i].Name) < strings.ToLower(p.hosts[j].Name)
	})
	return p.hosts
}

func splitSSHDirective(line string) (string, string) {
	// tolerate a UTF-8 BOM, which Windows editors commonly prepend
	line = strings.TrimPrefix(strings.TrimSpace(line), string(rune(0xfeff)))
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
