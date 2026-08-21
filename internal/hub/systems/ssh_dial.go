package systems

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

const (
	dialTimeout        = 6 * time.Second
	chainDeadline      = 20 * time.Second
	maxJumpChainLength = 8
)

// sshHostEntry is a parsed block of the user's ssh config.
type sshHostEntry struct {
	Hosts        []string // host patterns from the Host line
	HostName     string
	Port         string
	User         string
	IdentityFile string
	ProxyJump    string
}

var (
	sshDialMu       sync.Mutex
	sshDialEntries  []sshHostEntry
	sshDialModTime  time.Time
	sshDialPath     string
	sshDialLoaded   bool
	sshDialExactMap map[string]int // exact host name -> index into sshDialEntries
)

// sshConfigCandidatePaths mirrors readLocalSSHHosts' lookup order.
func sshConfigCandidatePaths() []string {
	if path := strings.TrimSpace(os.Getenv("SSH_CONFIG_PATH")); path != "" {
		return []string{path}
	}
	var paths []string
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		paths = append(paths, filepath.Join(home, ".ssh", "config"))
	}
	if p := os.Getenv("USERPROFILE"); p != "" {
		paths = append(paths, filepath.Join(p, ".ssh", "config"))
	}
	return paths
}

// loadSSHDialConfig (re)reads the ssh config when its mtime changes, so edits
// to the file take effect on the next poll without restarting the hub.
func loadSSHDialConfig() {
	path := ""
	var modTime time.Time
	for _, p := range sshConfigCandidatePaths() {
		if st, err := os.Stat(p); err == nil {
			path, modTime = p, st.ModTime()
			break
		}
	}
	sshDialMu.Lock()
	defer sshDialMu.Unlock()
	if sshDialLoaded && path == sshDialPath && !modTime.After(sshDialModTime) {
		return
	}
	entries := parseSSHConfigFile(path)
	sshDialEntries = entries
	sshDialExactMap = map[string]int{}
	for i, e := range entries {
		for _, pat := range e.Hosts {
			if !strings.ContainsAny(pat, "*?!") {
				if _, exists := sshDialExactMap[pat]; !exists {
					sshDialExactMap[pat] = i
				}
			}
		}
	}
	sshDialPath, sshDialModTime, sshDialLoaded = path, modTime, true
}

// parseSSHConfigFile parses Host blocks and the connection-relevant keywords.
// Both "Keyword value" and "Keyword=value" forms are accepted.
func parseSSHConfigFile(path string) []sshHostEntry {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var entries []sshHostEntry
	var cur *sshHostEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var keyword, val string
		if kv, value, found := strings.Cut(line, "="); found && !strings.Contains(kv, " ") {
			keyword, val = kv, value
		} else if kw, value, found := strings.Cut(line, " "); found {
			keyword, val = kw, value
		} else {
			continue
		}
		keyword, val = strings.ToLower(strings.TrimSpace(keyword)), strings.TrimSpace(val)
		switch keyword {
		case "host":
			entries = append(entries, sshHostEntry{Hosts: strings.Fields(val)})
			cur = &entries[len(entries)-1]
		case "hostname":
			if cur != nil && cur.HostName == "" {
				cur.HostName = val
			}
		case "port":
			if cur != nil && cur.Port == "" {
				cur.Port = val
			}
		case "user":
			if cur != nil && cur.User == "" {
				cur.User = val
			}
		case "identityfile":
			if cur != nil && cur.IdentityFile == "" {
				cur.IdentityFile = expandSSHPath(val)
			}
		case "proxyjump":
			if cur != nil && cur.ProxyJump == "" {
				cur.ProxyJump = val
			}
		}
	}
	return entries
}

func expandSSHPath(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

// lookupSSHHost returns the entry matching an exact host name first, then a
// pattern match. Blocks without connection-relevant fields never match.
func lookupSSHHost(host string) (sshHostEntry, bool) {
	loadSSHDialConfig()
	sshDialMu.Lock()
	defer sshDialMu.Unlock()
	if idx, ok := sshDialExactMap[host]; ok {
		return sshDialEntries[idx], true
	}
	for _, e := range sshDialEntries {
		if e.HostName == "" && e.ProxyJump == "" {
			continue
		}
		for _, pat := range e.Hosts {
			if pat == "*" || strings.HasPrefix(pat, "!") {
				continue
			}
			if sshPatternMatch(pat, host) {
				return e, true
			}
		}
	}
	return sshHostEntry{}, false
}

// sshPatternMatch supports the subset of ssh host patterns we care about
// (prefix/suffix/inline '*' segments).
func sshPatternMatch(pattern, host string) bool {
	if !strings.Contains(pattern, "*") {
		return false
	}
	parts := strings.Split(pattern, "*")
	if !strings.HasPrefix(host, parts[0]) {
		return false
	}
	host = host[len(parts[0]):]
	for i := 1; i < len(parts); i++ {
		idx := strings.Index(host, parts[i])
		if idx < 0 {
			return false
		}
		host = host[idx+len(parts[i]):]
	}
	return true
}

// sshHop is one resolved leg of a ProxyJump chain.
type sshHop struct {
	addr      string
	entry     *sshHostEntry // config block of this hop, nil for bare specs
	user      string
	entryCopy sshHostEntry
}

func userCurrentName() string {
	if u := os.Getenv("USERNAME"); u != "" {
		return u
	}
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Base(home)
	}
	return "root"
}

// hopClientConfig builds the ssh config for a jump host using its own
// IdentityFile / User from the ssh config.
func hopClientConfig(hop *sshHop) (*ssh.ClientConfig, error) {
	user := hop.user
	if user == "" && hop.entry != nil {
		user = hop.entry.User
	}
	if user == "" {
		user = userCurrentName()
	}
	var auths []ssh.AuthMethod
	var files []string
	if hop.entry != nil && hop.entry.IdentityFile != "" {
		files = append(files, hop.entry.IdentityFile)
	} else if home, err := os.UserHomeDir(); err == nil {
		files = append(files, filepath.Join(home, ".ssh", "id_ed25519"), filepath.Join(home, ".ssh", "id_rsa"))
	}
	for _, f := range files {
		if key, err := os.ReadFile(f); err == nil {
			if signer, err := ssh.ParsePrivateKey(key); err == nil {
				auths = append(auths, ssh.PublicKeys(signer))
			}
		}
	}
	if len(auths) == 0 {
		return nil, fmt.Errorf("no usable identity for jump host %s (tried %v)", hop.addr, files)
	}
	return &ssh.ClientConfig{
		User:            user,
		Auth:            auths,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         dialTimeout,
	}, nil
}

// buildJumpChain resolves the ProxyJump chain for an entry, nearest hop first.
// A hop's own ProxyJump (e.g. target -> h101 -> h100) is recursed into before
// the hop itself, so the resulting order is [h100, h101].
func buildJumpChain(entry sshHostEntry) ([]*sshHop, error) {
	var hops []*sshHop
	var build func(queue string) error
	build = func(queue string) error {
		for queue != "" {
			name, rest, _ := strings.Cut(queue, ",")
			name = strings.TrimSpace(name)
			queue = strings.TrimSpace(rest)
			if name == "" {
				continue
			}
			user := ""
			if idx := strings.LastIndex(name, "@"); idx > 0 {
				user, name = name[:idx], name[idx+1:]
			}
			hopEntry, ok := lookupSSHHost(name)
			addr := net.JoinHostPort(name, "22")
			if ok {
				hn := hopEntry.HostName
				if hn == "" {
					hn = name
				}
				port := hopEntry.Port
				if port == "" {
					port = "22"
				}
				addr = net.JoinHostPort(hn, port)
				// closer hops come first
				if hopEntry.ProxyJump != "" {
					if err := build(hopEntry.ProxyJump); err != nil {
						return err
					}
				}
			}
			hop := &sshHop{addr: addr, user: user}
			if ok {
				hop.entryCopy = hopEntry
				hop.entry = &hop.entryCopy
			}
			hops = append(hops, hop)
			if len(hops) > maxJumpChainLength {
				return fmt.Errorf("proxy jump chain exceeds %d hops", maxJumpChainLength)
			}
		}
		return nil
	}
	if err := build(entry.ProxyJump); err != nil {
		return nil, err
	}
	return hops, nil
}

// DialAgent resolves the system's host through the user's ssh config (so
// aliases, renumbered IPs, and ProxyJump chains always reflect the current
// file contents) and returns an established connection to the agent's SSH
// endpoint. Hosts not present in the ssh config are dialed directly.
func (sm *SystemManager) DialAgent(host, port string) (net.Conn, error) {
	deadline := time.Now().Add(chainDeadline)

	entry, matched := lookupSSHHost(host)
	if !matched {
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), dialTimeout)
		if err != nil {
			return nil, err
		}
		_ = conn.SetDeadline(deadline)
		return conn, nil
	}

	targetHost := entry.HostName
	if targetHost == "" {
		targetHost = host
	}
	targetPort := entry.Port
	if targetPort == "" {
		targetPort = port
	}
	targetAddr := net.JoinHostPort(targetHost, targetPort)

	hops, err := buildJumpChain(entry)
	if err != nil {
		return nil, err
	}

	// no jumps: connect straight to the (possibly renumbered) target
	if len(hops) == 0 {
		conn, err := net.DialTimeout("tcp", targetAddr, dialTimeout)
		if err != nil {
			return nil, err
		}
		_ = conn.SetDeadline(deadline)
		return conn, nil
	}

	// hop through the chain; each ssh client dials the next leg
	var hopClients []*ssh.Client
	closeAll := func() {
		for _, c := range hopClients {
			c.Close()
		}
	}
	for i, hop := range hops {
		cfg, err := hopClientConfig(hop)
		if err != nil {
			closeAll()
			return nil, err
		}
		var conn net.Conn
		if i == 0 {
			d := net.Dialer{Timeout: dialTimeout}
			conn, err = d.Dial("tcp", hop.addr)
		} else {
			conn, err = hopClients[len(hopClients)-1].Dial("tcp", hop.addr)
		}
		if err != nil {
			closeAll()
			return nil, fmt.Errorf("jump %s: %w", hop.addr, err)
		}
		_ = conn.SetDeadline(deadline)
		sshConn, chans, reqs, err := ssh.NewClientConn(conn, hop.addr, cfg)
		if err != nil {
			conn.Close()
			closeAll()
			return nil, fmt.Errorf("ssh to jump %s: %w", hop.addr, err)
		}
		hopClients = append(hopClients, ssh.NewClient(sshConn, chans, reqs))
	}

	conn, err := hopClients[len(hopClients)-1].Dial("tcp", targetAddr)
	if err != nil {
		closeAll()
		return nil, fmt.Errorf("dial agent %s via jumps: %w", targetAddr, err)
	}
	_ = conn.SetDeadline(deadline)
	// wrap so closing the agent connection also tears down the jump chain
	return &chainedConn{Conn: conn, clients: hopClients}, nil
}

// chainedConn ties the final agent connection to the jump chain behind it:
// closing the agent connection closes every ssh client in the chain.
type chainedConn struct {
	net.Conn
	clients []*ssh.Client
}

func (c *chainedConn) Close() error {
	err := c.Conn.Close()
	for _, cl := range c.clients {
		cl.Close()
	}
	return err
}
