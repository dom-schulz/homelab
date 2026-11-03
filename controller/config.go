package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	runnerNamePrefix = "gha-runner-"
	maxRepositories  = 5
)

// Config is loaded from an env-style file kept outside the repository.
type Config struct {
	GitHubAppID          int64
	GitHubPrivateKeyFile string
	GitHubInstallationID int64
	Repositories         []Repository
	RunnerLabel          string

	ProxmoxURL         string
	ProxmoxToken       string
	ProxmoxNode        string
	ProxmoxTLSInsecure bool
	RunnerPool         string
	TemplateVMID       int
	VMIDMin            int
	VMIDMax            int
	MaxPoolVMs         int

	PollInterval time.Duration
}

type Repository struct {
	Owner string
	Name  string
}

func (r Repository) String() string { return r.Owner + "/" + r.Name }

// LoadConfig reads KEY=VALUE lines. The file holds credentials, so it must not
// be readable by group or others.
func LoadConfig(path string) (Config, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Config{}, err
	}
	if info.Mode().Perm()&0o077 != 0 {
		return Config{}, fmt.Errorf("%s is readable by other users (mode %v); run chmod 600 on it", path, info.Mode().Perm())
	}

	f, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer f.Close()

	values := map[string]string{}
	scanner := bufio.NewScanner(f)
	for line := 1; scanner.Scan(); line++ {
		text := strings.TrimSpace(scanner.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		key, value, ok := strings.Cut(text, "=")
		if !ok {
			return Config{}, fmt.Errorf("%s:%d: expected KEY=VALUE", path, line)
		}
		values[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"'`)
	}
	if err := scanner.Err(); err != nil {
		return Config{}, err
	}
	return parseConfig(values)
}

func parseConfig(values map[string]string) (Config, error) {
	p := configParser{values: values}
	cfg := Config{
		GitHubAppID:          p.int64("GITHUB_APP_ID"),
		GitHubPrivateKeyFile: p.required("GITHUB_APP_PRIVATE_KEY_FILE"),
		GitHubInstallationID: p.int64("GITHUB_INSTALLATION_ID"),
		RunnerLabel:          p.required("RUNNER_LABEL"),
		ProxmoxURL:           strings.TrimSuffix(p.required("PROXMOX_API_URL"), "/"),
		ProxmoxToken:         p.required("PROXMOX_API_TOKEN"),
		ProxmoxNode:          p.optional("PROXMOX_NODE", "pve"),
		ProxmoxTLSInsecure:   p.optional("PROXMOX_TLS_INSECURE", "false") == "true",
		RunnerPool:           p.required("PROXMOX_RUNNER_POOL"),
		TemplateVMID:         p.int("PROXMOX_RUNNER_TEMPLATE_VMID", ""),
		MaxPoolVMs:           p.int("PROXMOX_MAX_POOL_VMS", "2"),
	}
	cfg.Repositories = p.repositories("GITHUB_REPOSITORIES")
	cfg.VMIDMin, cfg.VMIDMax = p.vmidRange("PROXMOX_RUNNER_VMID_RANGE", "9200-9209")

	// The controller runs one runner at a time; reject configs that expect more.
	if n := p.int("MAX_CONCURRENT_RUNNERS", "1"); n != 1 {
		p.fail("MAX_CONCURRENT_RUNNERS must be 1, got %d", n)
	}
	cfg.PollInterval = time.Duration(p.int("POLL_INTERVAL_SECONDS", "20")) * time.Second
	if cfg.PollInterval < 10*time.Second {
		p.fail("POLL_INTERVAL_SECONDS must be at least 10")
	}

	if cfg.MaxPoolVMs < 1 {
		p.fail("PROXMOX_MAX_POOL_VMS must be at least 1")
	}
	if cfg.TemplateVMID >= cfg.VMIDMin && cfg.TemplateVMID <= cfg.VMIDMax {
		p.fail("PROXMOX_RUNNER_TEMPLATE_VMID %d is inside the runner VM ID range", cfg.TemplateVMID)
	}
	for key := range values {
		if !p.seen[key] {
			p.fail("unknown key %s", key)
		}
	}
	if len(p.errs) > 0 {
		return Config{}, fmt.Errorf("invalid config:\n  %s", strings.Join(p.errs, "\n  "))
	}
	return cfg, nil
}

// configParser collects every problem so one run reports them all.
type configParser struct {
	values map[string]string
	seen   map[string]bool
	errs   []string
}

func (p *configParser) fail(format string, args ...any) {
	p.errs = append(p.errs, fmt.Sprintf(format, args...))
}

func (p *configParser) optional(key, fallback string) string {
	if p.seen == nil {
		p.seen = map[string]bool{}
	}
	p.seen[key] = true
	if v, ok := p.values[key]; ok && v != "" {
		return v
	}
	return fallback
}

func (p *configParser) required(key string) string {
	v := p.optional(key, "")
	if v == "" {
		p.fail("%s is required", key)
	}
	return v
}

func (p *configParser) int(key, fallback string) int {
	raw := p.optional(key, fallback)
	if raw == "" {
		p.fail("%s is required", key)
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		p.fail("%s must be an integer, got %q", key, raw)
	}
	return n
}

func (p *configParser) int64(key string) int64 {
	raw := p.required(key)
	if raw == "" {
		return 0
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		p.fail("%s must be an integer, got %q", key, raw)
	}
	return n
}

func (p *configParser) repositories(key string) []Repository {
	raw := p.required(key)
	if raw == "" {
		return nil
	}
	var repos []Repository
	for _, item := range strings.Split(raw, ",") {
		owner, name, ok := strings.Cut(strings.TrimSpace(item), "/")
		if !ok || owner == "" || name == "" || strings.Contains(name, "/") {
			p.fail("%s entry %q must be owner/repo", key, item)
			continue
		}
		repos = append(repos, Repository{Owner: owner, Name: name})
	}
	if len(repos) > maxRepositories {
		p.fail("%s lists %d repositories; the limit is %d", key, len(repos), maxRepositories)
	}
	return repos
}

func (p *configParser) vmidRange(key, fallback string) (int, int) {
	raw := p.optional(key, fallback)
	lo, hi, ok := strings.Cut(raw, "-")
	min, err1 := strconv.Atoi(lo)
	max, err2 := strconv.Atoi(hi)
	if !ok || err1 != nil || err2 != nil || min < 100 || max < min {
		p.fail("%s must look like 9200-9209, got %q", key, raw)
		return 0, -1
	}
	return min, max
}
