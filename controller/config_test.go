package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const validConfig = `# controller config
GITHUB_APP_ID=123
GITHUB_APP_PRIVATE_KEY_FILE=/home/me/.config/homelab/gha-runners.pem
GITHUB_INSTALLATION_ID=456
GITHUB_REPOSITORIES=dom-schulz/homelab, dom-schulz/drydock
PROXMOX_API_URL=https://192.0.2.10:8006/
PROXMOX_API_TOKEN="gha-controller@pve!controller=secret"
PROXMOX_TLS_INSECURE=true
PROXMOX_RUNNER_POOL=gha-runners
PROXMOX_RUNNER_TEMPLATE_VMID=910
RUNNER_LABEL=homelab-ephemeral
MAX_CONCURRENT_RUNNERS=1
POLL_INTERVAL_SECONDS=20
`

func writeConfig(t *testing.T, body string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "controller.env")
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadConfig(t *testing.T) {
	cfg, err := LoadConfig(writeConfig(t, validConfig, 0o600))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.GitHubAppID != 123 || cfg.GitHubInstallationID != 456 {
		t.Errorf("app IDs = %d, %d", cfg.GitHubAppID, cfg.GitHubInstallationID)
	}
	if len(cfg.Repositories) != 2 || cfg.Repositories[1].String() != "dom-schulz/drydock" {
		t.Errorf("repositories = %v", cfg.Repositories)
	}
	if cfg.ProxmoxURL != "https://192.0.2.10:8006" || cfg.ProxmoxToken != "gha-controller@pve!controller=secret" {
		t.Errorf("proxmox = %q %q", cfg.ProxmoxURL, cfg.ProxmoxToken)
	}
	if cfg.ProxmoxNode != "pve" || !cfg.ProxmoxTLSInsecure {
		t.Errorf("node/tls = %q %v", cfg.ProxmoxNode, cfg.ProxmoxTLSInsecure)
	}
	if cfg.PollInterval != 20*time.Second {
		t.Errorf("poll interval = %v", cfg.PollInterval)
	}
	if cfg.VMIDMin != 9200 || cfg.VMIDMax != 9209 || cfg.MaxPoolVMs != 2 {
		t.Errorf("defaults = %d-%d cap %d", cfg.VMIDMin, cfg.VMIDMax, cfg.MaxPoolVMs)
	}
}

func TestLoadConfigRejectsReadableFile(t *testing.T) {
	_, err := LoadConfig(writeConfig(t, validConfig, 0o644))
	if err == nil || !strings.Contains(err.Error(), "chmod 600") {
		t.Fatalf("want permission error, got %v", err)
	}
}

func TestLoadConfigReportsEveryProblem(t *testing.T) {
	body := strings.NewReplacer(
		"GITHUB_REPOSITORIES=dom-schulz/homelab, dom-schulz/drydock", "GITHUB_REPOSITORIES=a/1,a/2,a/3,a/4,a/5,a/6",
		"MAX_CONCURRENT_RUNNERS=1", "MAX_CONCURRENT_RUNNERS=3",
		"PROXMOX_RUNNER_TEMPLATE_VMID=910", "PROXMOX_RUNNER_TEMPLATE_VMID=9205",
		"RUNNER_LABEL=homelab-ephemeral", "RUNER_LABEL=typo",
		"POLL_INTERVAL_SECONDS=20", "POLL_INTERVAL_SECONDS=2",
	).Replace(validConfig)

	_, err := LoadConfig(writeConfig(t, body, 0o600))
	if err == nil {
		t.Fatal("want error")
	}
	for _, want := range []string{"limit is 5", "MAX_CONCURRENT_RUNNERS must be 1", "inside the runner VM ID range", "RUNNER_LABEL is required", "unknown key RUNER_LABEL", "POLL_INTERVAL_SECONDS must be at least 10"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %q:\n%v", want, err)
		}
	}
}
