package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Proxmox is a minimal client for the handful of VE API calls the controller
// makes. Every call needs only the privileges in docs/runner-controller-access.md.
type Proxmox struct {
	apiURL       string
	token        string
	node         string
	http         *http.Client
	taskInterval time.Duration
}

func NewProxmox(cfg Config) *Proxmox {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if cfg.ProxmoxTLSInsecure {
		// Proxmox ships a self-signed certificate; only used on the private network.
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return &Proxmox{
		apiURL:       cfg.ProxmoxURL + "/api2/json",
		token:        cfg.ProxmoxToken,
		node:         cfg.ProxmoxNode,
		http:         &http.Client{Transport: transport, Timeout: 60 * time.Second},
		taskInterval: 2 * time.Second,
	}
}

// ProxmoxError is a non-2xx API response.
type ProxmoxError struct {
	Method, Path string
	StatusCode   int
	Message      string
}

func (e *ProxmoxError) Error() string {
	return fmt.Sprintf("proxmox %s %s: %d %s", e.Method, e.Path, e.StatusCode, e.Message)
}

func (p *Proxmox) do(ctx context.Context, method, path string, params url.Values, out any) error {
	target := p.apiURL + path
	var body io.Reader
	if method == http.MethodGet || method == http.MethodDelete {
		if len(params) > 0 {
			target += "?" + params.Encode()
		}
	} else {
		body = strings.NewReader(params.Encode())
	}

	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "PVEAPIToken="+p.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	resp, err := p.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		// Proxmox puts the reason in the status line; the body may add field errors.
		msg := strings.TrimSpace(strings.TrimPrefix(resp.Status, strconv.Itoa(resp.StatusCode)))
		if len(raw) > 0 && string(raw) != "null" {
			msg += " " + strings.TrimSpace(string(raw))
		}
		return &ProxmoxError{Method: method, Path: path, StatusCode: resp.StatusCode, Message: msg}
	}
	if out == nil {
		return nil
	}
	envelope := struct {
		Data json.RawMessage `json:"data"`
	}{}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("proxmox %s %s: decoding response: %w", method, path, err)
	}
	if err := json.Unmarshal(envelope.Data, out); err != nil {
		return fmt.Errorf("proxmox %s %s: decoding data: %w", method, path, err)
	}
	return nil
}

func (p *Proxmox) vmPath(vmid int, suffix string) string {
	return fmt.Sprintf("/nodes/%s/qemu/%d%s", url.PathEscape(p.node), vmid, suffix)
}

// runTask issues a call that returns a task ID and waits for the task to finish.
func (p *Proxmox) runTask(ctx context.Context, method, path string, params url.Values) error {
	var upid string
	if err := p.do(ctx, method, path, params, &upid); err != nil {
		return err
	}
	return p.waitTask(ctx, upid)
}

func (p *Proxmox) waitTask(ctx context.Context, upid string) error {
	path := fmt.Sprintf("/nodes/%s/tasks/%s/status", url.PathEscape(p.node), url.PathEscape(upid))
	for {
		var status struct {
			Status     string `json:"status"`
			ExitStatus string `json:"exitstatus"`
		}
		if err := p.do(ctx, http.MethodGet, path, nil, &status); err != nil {
			return err
		}
		if status.Status == "stopped" {
			if status.ExitStatus != "OK" {
				return fmt.Errorf("proxmox task %s failed: %s", upid, status.ExitStatus)
			}
			return nil
		}
		if err := sleep(ctx, p.taskInterval); err != nil {
			return fmt.Errorf("waiting for proxmox task %s: %w", upid, err)
		}
	}
}

type PoolVM struct {
	VMID   int    `json:"vmid"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Type   string `json:"type"`
}

// PoolVMs lists the VMs in the runner pool. The token can see nothing else.
func (p *Proxmox) PoolVMs(ctx context.Context, pool string) ([]PoolVM, error) {
	var data struct {
		Members []PoolVM `json:"members"`
	}
	if err := p.do(ctx, http.MethodGet, "/pools/"+url.PathEscape(pool), nil, &data); err != nil {
		return nil, err
	}
	var vms []PoolVM
	for _, m := range data.Members {
		if m.Type == "qemu" {
			vms = append(vms, m)
		}
	}
	return vms, nil
}

// VMIDFree reports whether no VM or container uses vmid anywhere on the cluster.
func (p *Proxmox) VMIDFree(ctx context.Context, vmid int) (bool, error) {
	var got json.Number
	err := p.do(ctx, http.MethodGet, "/cluster/nextid", url.Values{"vmid": {strconv.Itoa(vmid)}}, &got)
	var perr *ProxmoxError
	if errors.As(err, &perr) && perr.StatusCode == http.StatusBadRequest {
		return false, nil
	}
	return err == nil, err
}

// Clone makes a linked clone of the template inside the runner pool.
func (p *Proxmox) Clone(ctx context.Context, templateID, newID int, name, pool string) error {
	return p.runTask(ctx, http.MethodPost, p.vmPath(templateID, "/clone"), url.Values{
		"newid": {strconv.Itoa(newID)},
		"name":  {name},
		"pool":  {pool},
		"full":  {"0"},
	})
}

// ConfigureCloudInit drops the builder's custom user data inherited from the
// template, so Proxmox generates defaults (hostname = VM name), and skips the
// first-boot package upgrade.
func (p *Proxmox) ConfigureCloudInit(ctx context.Context, vmid int) error {
	return p.do(ctx, http.MethodPut, p.vmPath(vmid, "/config"), url.Values{
		"delete":    {"cicustom"},
		"ciupgrade": {"0"},
	}, nil)
}

func (p *Proxmox) Start(ctx context.Context, vmid int) error {
	return p.runTask(ctx, http.MethodPost, p.vmPath(vmid, "/status/start"), nil)
}

func (p *Proxmox) Stop(ctx context.Context, vmid int) error {
	return p.runTask(ctx, http.MethodPost, p.vmPath(vmid, "/status/stop"), nil)
}

func (p *Proxmox) Destroy(ctx context.Context, vmid int) error {
	return p.runTask(ctx, http.MethodDelete, p.vmPath(vmid, ""), nil)
}

func (p *Proxmox) Status(ctx context.Context, vmid int) (string, error) {
	var data struct {
		Status string `json:"status"`
	}
	err := p.do(ctx, http.MethodGet, p.vmPath(vmid, "/status/current"), nil, &data)
	return data.Status, err
}

func (p *Proxmox) AgentPing(ctx context.Context, vmid int) error {
	return p.do(ctx, http.MethodPost, p.vmPath(vmid, "/agent/ping"), nil, nil)
}

// WriteFile writes content into the guest through the QEMU guest agent.
// Proxmox base64-encodes it for the agent (encode defaults to on).
func (p *Proxmox) WriteFile(ctx context.Context, vmid int, path, content string) error {
	return p.do(ctx, http.MethodPost, p.vmPath(vmid, "/agent/file-write"), url.Values{
		"file":    {path},
		"content": {content},
	}, nil)
}

// sleep waits for d or until ctx is done.
func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
