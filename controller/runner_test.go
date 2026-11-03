package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"
)

// fakeProxmox records calls and keeps a tiny in-memory pool.
type fakeProxmox struct {
	pool      []PoolVM
	taken     map[int]bool
	calls     []string
	agentUp   bool
	failClone error
	onClone   func()
	written   map[string]string
}

func (f *fakeProxmox) record(format string, args ...any) {
	f.calls = append(f.calls, fmt.Sprintf(format, args...))
}

func (f *fakeProxmox) PoolVMs(ctx context.Context, pool string) ([]PoolVM, error) {
	return slices.Clone(f.pool), nil
}

func (f *fakeProxmox) VMIDFree(ctx context.Context, vmid int) (bool, error) {
	return !f.taken[vmid], nil
}

func (f *fakeProxmox) Clone(ctx context.Context, templateID, newID int, name, pool string) error {
	f.record("clone %d->%d %s", templateID, newID, name)
	if f.onClone != nil {
		f.onClone()
	}
	if f.failClone != nil {
		return f.failClone
	}
	f.pool = append(f.pool, PoolVM{VMID: newID, Name: name, Status: "stopped", Type: "qemu"})
	return nil
}

func (f *fakeProxmox) ConfigureCloudInit(ctx context.Context, vmid int) error {
	f.record("cloudinit %d", vmid)
	return nil
}

func (f *fakeProxmox) Start(ctx context.Context, vmid int) error {
	f.record("start %d", vmid)
	f.setStatus(vmid, "running")
	return nil
}

func (f *fakeProxmox) Stop(ctx context.Context, vmid int) error {
	f.record("stop %d", vmid)
	f.setStatus(vmid, "stopped")
	return nil
}

func (f *fakeProxmox) Destroy(ctx context.Context, vmid int) error {
	f.record("destroy %d", vmid)
	f.pool = slices.DeleteFunc(f.pool, func(vm PoolVM) bool { return vm.VMID == vmid })
	return nil
}

func (f *fakeProxmox) Status(ctx context.Context, vmid int) (string, error) {
	for _, vm := range f.pool {
		if vm.VMID == vmid {
			return vm.Status, nil
		}
	}
	return "", errors.New("no such VM")
}

func (f *fakeProxmox) AgentPing(ctx context.Context, vmid int) error {
	if !f.agentUp {
		return errors.New("agent not running")
	}
	return nil
}

func (f *fakeProxmox) WriteFile(ctx context.Context, vmid int, path, content string) error {
	f.record("write %d %s", vmid, path)
	if f.written == nil {
		f.written = map[string]string{}
	}
	f.written[path] = content
	return nil
}

func (f *fakeProxmox) setStatus(vmid int, status string) {
	for i := range f.pool {
		if f.pool[i].VMID == vmid {
			f.pool[i].Status = status
		}
	}
}

// fakeGitHub serves one queued job; the runner's states are scripted per poll.
type fakeGitHub struct {
	job        *QueuedJob
	states     []string // "offline", "online", or "gone"; the last one repeats
	polls      int
	created    []string
	removed    []int64
	onCreate   func()
	registered []RegisteredRunner
	oneShot    bool   // stop offering the job once a runner was created
	onIdle     func() // called whenever there is no job to offer
}

func (f *fakeGitHub) OldestQueuedJob(ctx context.Context) (*QueuedJob, error) {
	if f.job == nil || (f.oneShot && len(f.created) > 0) {
		if f.onIdle != nil {
			f.onIdle()
		}
		return nil, nil
	}
	return f.job, nil
}

func (f *fakeGitHub) ListRunners(ctx context.Context) ([]RegisteredRunner, error) {
	return slices.Clone(f.registered), nil
}

func (f *fakeGitHub) CreateRunner(ctx context.Context, repo Repository, name string) (int64, string, error) {
	f.created = append(f.created, name)
	if f.onCreate != nil {
		f.onCreate()
	}
	return 42, "encoded-jit-config", nil
}

func (f *fakeGitHub) RunnerState(ctx context.Context, repo Repository, runnerID int64) (bool, string, error) {
	state := f.states[min(f.polls, len(f.states)-1)]
	f.polls++
	if state == "gone" {
		return false, "", nil
	}
	return true, state, nil
}

func (f *fakeGitHub) RemoveRunner(ctx context.Context, repo Repository, runnerID int64) error {
	f.removed = append(f.removed, runnerID)
	f.registered = slices.DeleteFunc(f.registered, func(r RegisteredRunner) bool { return r.ID == runnerID })
	return nil
}

func newTestController(gh *fakeGitHub, pve *fakeProxmox) *Controller {
	return &Controller{
		cfg: Config{
			RunnerLabel:  "homelab-ephemeral",
			RunnerPool:   "gha-runners",
			TemplateVMID: 910,
			VMIDMin:      9200,
			VMIDMax:      9202,
			MaxPoolVMs:   2,
			PollInterval: time.Millisecond,
		},
		github:  gh,
		proxmox: pve,
		timeouts: Timeouts{
			AgentReady:   50 * time.Millisecond,
			RunnerOnline: 50 * time.Millisecond,
			Job:          50 * time.Millisecond,
			Cleanup:      time.Second,
			Poll:         time.Millisecond,
		},
		log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		now: func() time.Time { return time.Unix(1700000000, 0) },
	}
}

var homelabJob = &QueuedJob{Repo: Repository{Owner: "dom-schulz", Name: "homelab"}, JobID: 7, Name: "smoke"}

func TestRunOnceServesJobAndDestroysVM(t *testing.T) {
	gh := &fakeGitHub{job: homelabJob, states: []string{"offline", "online", "online", "gone"}}
	pve := &fakeProxmox{agentUp: true}

	if err := newTestController(gh, pve).RunOnce(context.Background(), false); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	want := []string{
		"clone 910->9200 gha-runner-9200-1700000000",
		"cloudinit 9200",
		"start 9200",
		"write 9200 /etc/gha-runner/jitconfig",
		"stop 9200",
		"destroy 9200",
	}
	if !slices.Equal(pve.calls, want) {
		t.Errorf("proxmox calls\n got %q\nwant %q", pve.calls, want)
	}
	if got := pve.written[jitConfigPath]; got != "encoded-jit-config" {
		t.Errorf("jitconfig written = %q", got)
	}
	if len(gh.removed) != 0 {
		t.Errorf("finished runner should not need removal, removed %v", gh.removed)
	}
	if len(pve.pool) != 0 {
		t.Errorf("pool not empty after job: %v", pve.pool)
	}
}

func TestRunOnceNoJobDoesNothing(t *testing.T) {
	gh := &fakeGitHub{}
	pve := &fakeProxmox{}
	if err := newTestController(gh, pve).RunOnce(context.Background(), false); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if len(pve.calls) != 0 {
		t.Errorf("unexpected proxmox calls: %q", pve.calls)
	}
}

func TestRunOnceDryRunChangesNothing(t *testing.T) {
	gh := &fakeGitHub{job: homelabJob, states: []string{"online"}}
	pve := &fakeProxmox{agentUp: true}
	if err := newTestController(gh, pve).RunOnce(context.Background(), true); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if len(pve.calls) != 0 || len(gh.created) != 0 {
		t.Errorf("dry run made changes: proxmox %q, runners %q", pve.calls, gh.created)
	}
}

func TestAgentTimeoutDestroysVMWithoutRegistering(t *testing.T) {
	gh := &fakeGitHub{job: homelabJob, states: []string{"online"}}
	pve := &fakeProxmox{agentUp: false}

	err := newTestController(gh, pve).RunOnce(context.Background(), false)
	if err == nil || !strings.Contains(err.Error(), "guest agent") {
		t.Fatalf("want guest agent timeout, got %v", err)
	}
	if len(gh.created) != 0 {
		t.Errorf("runner registered before the VM was ready: %q", gh.created)
	}
	if !slices.Contains(pve.calls, "destroy 9200") || len(pve.pool) != 0 {
		t.Errorf("VM not destroyed: calls %q, pool %v", pve.calls, pve.pool)
	}
}

func TestRunnerNeverOnlineRemovesRegistrationAndVM(t *testing.T) {
	gh := &fakeGitHub{job: homelabJob, states: []string{"offline"}}
	pve := &fakeProxmox{agentUp: true}

	err := newTestController(gh, pve).RunOnce(context.Background(), false)
	if err == nil || !strings.Contains(err.Error(), "runner online") {
		t.Fatalf("want runner online timeout, got %v", err)
	}
	if !slices.Equal(gh.removed, []int64{42}) {
		t.Errorf("registration not removed: %v", gh.removed)
	}
	if len(pve.pool) != 0 {
		t.Errorf("VM not destroyed: %v", pve.pool)
	}
}

func TestJobTimeoutRemovesRegistrationAndVM(t *testing.T) {
	gh := &fakeGitHub{job: homelabJob, states: []string{"online"}}
	pve := &fakeProxmox{agentUp: true}

	err := newTestController(gh, pve).RunOnce(context.Background(), false)
	if err == nil || !strings.Contains(err.Error(), "job") {
		t.Fatalf("want job timeout, got %v", err)
	}
	if !slices.Equal(gh.removed, []int64{42}) || len(pve.pool) != 0 {
		t.Errorf("cleanup incomplete: removed %v, pool %v", gh.removed, pve.pool)
	}
}

func TestCloneFailureLeavesNothingBehind(t *testing.T) {
	gh := &fakeGitHub{job: homelabJob, states: []string{"online"}}
	pve := &fakeProxmox{agentUp: true, failClone: errors.New("storage full")}

	if err := newTestController(gh, pve).RunOnce(context.Background(), false); err == nil {
		t.Fatal("want clone error")
	}
	if len(gh.created) != 0 || slices.Contains(pve.calls, "destroy 9200") {
		t.Errorf("unexpected follow-up: runners %q, calls %q", gh.created, pve.calls)
	}
}

func TestPoolCapRefusesToClone(t *testing.T) {
	gh := &fakeGitHub{job: homelabJob, states: []string{"online"}}
	pve := &fakeProxmox{agentUp: true, pool: []PoolVM{
		{VMID: 9200, Name: "gha-runner-9200-1700000000", Type: "qemu"},
		{VMID: 9201, Name: "gha-runner-9201-1700000000", Type: "qemu"},
	}}

	err := newTestController(gh, pve).RunOnce(context.Background(), false)
	if err == nil || !strings.Contains(err.Error(), "cap 2") {
		t.Fatalf("want pool cap error, got %v", err)
	}
	if len(pve.calls) != 0 {
		t.Errorf("cloned despite cap: %q", pve.calls)
	}
}

func TestForeignVMInPoolStopsEverything(t *testing.T) {
	gh := &fakeGitHub{job: homelabJob, states: []string{"online"}}
	pve := &fakeProxmox{agentUp: true, pool: []PoolVM{{VMID: 105, Name: "k3s-control", Type: "qemu"}}}

	err := newTestController(gh, pve).RunOnce(context.Background(), false)
	if err == nil || !strings.Contains(err.Error(), "not a runner") {
		t.Fatalf("want foreign VM error, got %v", err)
	}
	if len(pve.calls) != 0 {
		t.Errorf("acted despite foreign VM: %q", pve.calls)
	}
}

func TestSkipsVMIDsInUseElsewhere(t *testing.T) {
	gh := &fakeGitHub{job: homelabJob, states: []string{"gone"}}
	pve := &fakeProxmox{agentUp: true, taken: map[int]bool{9200: true}}

	if err := newTestController(gh, pve).RunOnce(context.Background(), false); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if pve.calls[0] != "clone 910->9201 gha-runner-9201-1700000000" {
		t.Errorf("first call = %q, want clone into 9201", pve.calls[0])
	}
}

func TestCancelledRunStillCleansUp(t *testing.T) {
	gh := &fakeGitHub{job: homelabJob, states: []string{"online"}}
	pve := &fakeProxmox{agentUp: true}
	c := newTestController(gh, pve)
	c.timeouts.Job = time.Minute

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	gh.onCreate = cancel // simulates Ctrl-C once the runner is registered

	if err := c.RunOnce(ctx, false); !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
	if !slices.Equal(gh.removed, []int64{42}) || len(pve.pool) != 0 {
		t.Errorf("cleanup after cancel incomplete: removed %v, pool %v", gh.removed, pve.pool)
	}
}

// runnerName builds a controller VM name created secondsAgo before the test clock.
func runnerName(vmid int, secondsAgo int64) string {
	return fmt.Sprintf("%s%d-%d", runnerNamePrefix, vmid, 1700000000-secondsAgo)
}

func TestReconcile(t *testing.T) {
	// Test timeouts: setup grace is 100ms, max lifetime about 1.15s.
	repo := Repository{Owner: "dom-schulz", Name: "homelab"}
	young := runnerName(9200, 0)        // still setting up: keep
	unregistered := runnerName(9201, 1) // past setup grace, runner gone: destroy
	busy := runnerName(9202, 1)         // registered, within lifetime: keep
	ancient := runnerName(9203, 5)      // past max lifetime: destroy and deregister
	pve := &fakeProxmox{pool: []PoolVM{
		{VMID: 9200, Name: young, Type: "qemu", Status: "running"},
		{VMID: 9201, Name: unregistered, Type: "qemu", Status: "running"},
		{VMID: 9202, Name: busy, Type: "qemu", Status: "running"},
		{VMID: 9203, Name: ancient, Type: "qemu", Status: "running"},
		{VMID: 9204, Name: "gha-runner-9204", Type: "qemu", Status: "stopped"}, // no timestamp: keep
	}}
	gh := &fakeGitHub{registered: []RegisteredRunner{
		{Repo: repo, ID: 2, Name: busy, Status: "online"},
		{Repo: repo, ID: 3, Name: ancient, Status: "online"},
		{Repo: repo, ID: 9, Name: runnerName(9205, 1), Status: "offline"}, // VM gone: deregister
	}}
	c := newTestController(gh, pve)
	c.cfg.VMIDMax = 9209

	if err := c.Reconcile(context.Background(), false); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	var left []int
	for _, vm := range pve.pool {
		left = append(left, vm.VMID)
	}
	if !slices.Equal(left, []int{9200, 9202, 9204}) {
		t.Errorf("VMs left = %v, want [9200 9202 9204]", left)
	}
	slices.Sort(gh.removed)
	if !slices.Equal(gh.removed, []int64{3, 9}) {
		t.Errorf("registrations removed = %v, want [3 9]", gh.removed)
	}
}

func TestReconcileDryRunChangesNothing(t *testing.T) {
	repo := Repository{Owner: "dom-schulz", Name: "homelab"}
	pve := &fakeProxmox{pool: []PoolVM{{VMID: 9200, Name: runnerName(9200, 5), Type: "qemu", Status: "running"}}}
	gh := &fakeGitHub{registered: []RegisteredRunner{{Repo: repo, ID: 9, Name: runnerName(9201, 1)}}}

	if err := newTestController(gh, pve).Reconcile(context.Background(), true); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if len(pve.calls) != 0 || len(gh.removed) != 0 {
		t.Errorf("dry run changed things: calls %q, removed %v", pve.calls, gh.removed)
	}
}

func TestRunOnceCleansUpBeforeServing(t *testing.T) {
	// A VM leaked by a crash fills one of two slots; reconcile frees it so the cap allows a new clone.
	pve := &fakeProxmox{agentUp: true, pool: []PoolVM{
		{VMID: 9200, Name: runnerName(9200, 5), Type: "qemu", Status: "running"},
		{VMID: 9201, Name: runnerName(9201, 5), Type: "qemu", Status: "running"},
	}}
	gh := &fakeGitHub{job: homelabJob, states: []string{"gone"}}

	if err := newTestController(gh, pve).RunOnce(context.Background(), false); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if !slices.Contains(pve.calls, "clone 910->9200 gha-runner-9200-1700000000") {
		t.Errorf("did not clone after cleanup: %q", pve.calls)
	}
	if len(pve.pool) != 0 {
		t.Errorf("pool not empty: %v", pve.pool)
	}
}

func TestRunServesJobsUntilCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	gh := &fakeGitHub{job: homelabJob, states: []string{"online", "gone"}, oneShot: true}
	pve := &fakeProxmox{agentUp: true}
	idle := 0
	gh.onIdle = func() {
		if idle++; idle == 2 {
			cancel() // stop after a couple of empty polls
		}
	}

	if err := newTestController(gh, pve).Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(gh.created) != 1 || len(pve.pool) != 0 {
		t.Errorf("served %d jobs, pool %v; want 1 job and an empty pool", len(gh.created), pve.pool)
	}
}

func TestRunKeepsPollingAfterErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pve := &fakeProxmox{agentUp: true, failClone: errors.New("storage full")}
	gh := &fakeGitHub{job: homelabJob, states: []string{"online"}}
	attempts := 0
	pve.onClone = func() {
		if attempts++; attempts == 3 {
			cancel()
		}
	}

	if err := newTestController(gh, pve).Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if attempts != 3 {
		t.Errorf("clone attempts = %d, want 3", attempts)
	}
}
