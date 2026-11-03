package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

type pveRequest struct {
	Method, Path string
	Form         url.Values
}

// fakePVE answers with canned bodies keyed by "METHOD /path" and records requests.
func fakePVE(t *testing.T, responses map[string]func(w http.ResponseWriter)) (*Proxmox, *[]pveRequest) {
	t.Helper()
	var mu sync.Mutex
	var seen []pveRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "PVEAPIToken=gha-controller@pve!controller=secret" {
			t.Errorf("Authorization header = %q", got)
		}
		r.ParseForm()
		path := strings.TrimPrefix(r.URL.EscapedPath(), "/api2/json")
		mu.Lock()
		seen = append(seen, pveRequest{Method: r.Method, Path: path, Form: r.Form})
		mu.Unlock()
		respond, ok := responses[r.Method+" "+path]
		if !ok {
			t.Errorf("unexpected request %s %s", r.Method, path)
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		respond(w)
	}))
	t.Cleanup(srv.Close)

	p := NewProxmox(Config{ProxmoxURL: srv.URL, ProxmoxToken: "gha-controller@pve!controller=secret", ProxmoxNode: "pve"})
	p.taskInterval = 0
	return p, &seen
}

func data(body string) func(w http.ResponseWriter) {
	return func(w http.ResponseWriter) { fmt.Fprintf(w, `{"data":%s}`, body) }
}

const upid = "UPID:pve:0001:0002:0003:qmclone:910:gha-controller@pve!controller:"

func taskPath() string {
	return "/nodes/pve/tasks/" + url.PathEscape(upid) + "/status"
}

func TestCloneWaitsForTask(t *testing.T) {
	polls := 0
	p, seen := fakePVE(t, map[string]func(http.ResponseWriter){
		"POST /nodes/pve/qemu/910/clone": data(`"` + upid + `"`),
		"GET " + taskPath(): func(w http.ResponseWriter) {
			polls++
			if polls < 3 {
				data(`{"status":"running"}`)(w)
				return
			}
			data(`{"status":"stopped","exitstatus":"OK"}`)(w)
		},
	})

	if err := p.Clone(context.Background(), 910, 9200, "gha-runner-9200-1", "gha-runners"); err != nil {
		t.Fatalf("Clone: %v", err)
	}
	form := (*seen)[0].Form
	for key, want := range map[string]string{"newid": "9200", "name": "gha-runner-9200-1", "pool": "gha-runners", "full": "0"} {
		if got := form.Get(key); got != want {
			t.Errorf("clone %s = %q, want %q", key, got, want)
		}
	}
	if polls != 3 {
		t.Errorf("task polled %d times, want 3", polls)
	}
}

func TestFailedTaskIsAnError(t *testing.T) {
	p, _ := fakePVE(t, map[string]func(http.ResponseWriter){
		"POST /nodes/pve/qemu/9200/status/start": data(`"` + upid + `"`),
		"GET " + taskPath():                      data(`{"status":"stopped","exitstatus":"start failed: not enough memory"}`),
	})
	err := p.Start(context.Background(), 9200)
	if err == nil || !strings.Contains(err.Error(), "not enough memory") {
		t.Fatalf("want task failure, got %v", err)
	}
}

func TestPoolVMsListsOnlyQemu(t *testing.T) {
	p, _ := fakePVE(t, map[string]func(http.ResponseWriter){
		"GET /pools/gha-runners": data(`{"members":[
			{"vmid":9200,"name":"gha-runner-9200-1","status":"running","type":"qemu"},
			{"storage":"local-lvm","type":"storage"}]}`),
	})
	vms, err := p.PoolVMs(context.Background(), "gha-runners")
	if err != nil {
		t.Fatalf("PoolVMs: %v", err)
	}
	if len(vms) != 1 || vms[0].VMID != 9200 || vms[0].Status != "running" {
		t.Errorf("PoolVMs = %+v", vms)
	}
}
