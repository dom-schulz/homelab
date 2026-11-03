package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

const jitConfigPath = "/etc/gha-runner/jitconfig"

type githubAPI interface {
	OldestQueuedJob(ctx context.Context) (*QueuedJob, error)
	CreateRunner(ctx context.Context, repo Repository, name string) (int64, string, error)
	RunnerState(ctx context.Context, repo Repository, runnerID int64) (bool, string, error)
	RemoveRunner(ctx context.Context, repo Repository, runnerID int64) error
	ListRunners(ctx context.Context) ([]RegisteredRunner, error)
}

type proxmoxAPI interface {
	PoolVMs(ctx context.Context, pool string) ([]PoolVM, error)
	VMIDFree(ctx context.Context, vmid int) (bool, error)
	Clone(ctx context.Context, templateID, newID int, name, pool string) error
	ConfigureCloudInit(ctx context.Context, vmid int) error
	Start(ctx context.Context, vmid int) error
	Stop(ctx context.Context, vmid int) error
	Destroy(ctx context.Context, vmid int) error
	Status(ctx context.Context, vmid int) (string, error)
	AgentPing(ctx context.Context, vmid int) error
	WriteFile(ctx context.Context, vmid int, path, content string) error
}

type Timeouts struct {
	AgentReady   time.Duration // VM start until the guest agent answers
	RunnerOnline time.Duration // config written until GitHub shows the runner online
	Job          time.Duration // runner online until it deregisters after its job
	Cleanup      time.Duration // stop, destroy, and deregister after any outcome
	Poll         time.Duration
}

var defaultTimeouts = Timeouts{
	AgentReady:   5 * time.Minute,
	RunnerOnline: 5 * time.Minute,
	Job:          60 * time.Minute,
	Cleanup:      5 * time.Minute,
	Poll:         5 * time.Second,
}

// setupGrace is how long a new VM may exist before its runner must be registered.
func (t Timeouts) setupGrace() time.Duration { return t.AgentReady + t.RunnerOnline }

// maxLifetime is the longest any healthy runner VM can exist.
func (t Timeouts) maxLifetime() time.Duration {
	return t.AgentReady + t.RunnerOnline + t.Job + t.Cleanup
}

type Controller struct {
	cfg      Config
	github   githubAPI
	proxmox  proxmoxAPI
	timeouts Timeouts
	log      *slog.Logger
	now      func() time.Time
	idle     bool // already logged that there are no queued jobs
}

// Run polls until ctx is cancelled, cleaning up leftovers and serving one job
// at a time. A failed poll is logged and retried on the next interval.
func (c *Controller) Run(ctx context.Context) error {
	c.log.Info("polling for queued jobs", "interval", c.cfg.PollInterval, "repos", len(c.cfg.Repositories), "label", c.cfg.RunnerLabel)
	for {
		if err := c.RunOnce(ctx, false); err != nil && ctx.Err() == nil {
			c.log.Error("poll failed; retrying next interval", "err", err)
		}
		if sleep(ctx, c.cfg.PollInterval) != nil {
			c.log.Info("stopping")
			return nil
		}
	}
}

// RunOnce cleans up what earlier runs left behind, then serves at most one
// queued job: it clones a runner VM, hands it a just-in-time config, waits for
// the job, and always destroys the VM. With dryRun it only logs what it would do.
func (c *Controller) RunOnce(ctx context.Context, dryRun bool) error {
	if err := c.Reconcile(ctx, dryRun); err != nil {
		return err
	}
	return c.serveNext(ctx, dryRun)
}

// Reconcile removes leftovers from a crash, a sleeping laptop, or a failed
// cleanup: runner VMs whose runner is gone or that outlived any job, and
// registrations whose VM is gone. It never touches a VM that could still be
// setting up or running a job.
func (c *Controller) Reconcile(ctx context.Context, dryRun bool) error {
	vms, err := c.proxmox.PoolVMs(ctx, c.cfg.RunnerPool)
	if err != nil {
		return err
	}
	runners, err := c.github.ListRunners(ctx)
	if err != nil {
		return err
	}
	registered := map[string]RegisteredRunner{}
	for _, r := range runners {
		registered[r.Name] = r
	}

	var errs []error
	inPool := map[string]bool{}
	for _, vm := range vms {
		if !c.ownsVM(vm) {
			return fmt.Errorf("pool %s contains VM %d %q that is not a runner; refusing to continue", c.cfg.RunnerPool, vm.VMID, vm.Name)
		}
		inPool[vm.Name] = true
		created, ok := runnerCreatedAt(vm.Name)
		if !ok {
			c.log.Warn("runner VM name has no creation time; leaving it for the operator", "vmid", vm.VMID, "name", vm.Name)
			continue
		}
		age := c.now().Sub(created)
		reg, isRegistered := registered[vm.Name]
		var reason string
		switch {
		case age > c.timeouts.maxLifetime():
			reason = "outlived the maximum job lifetime"
		case !isRegistered && age > c.timeouts.setupGrace():
			reason = "its runner is no longer registered"
		default:
			if dryRun {
				keepUntil := created.Add(c.timeouts.maxLifetime())
				why := "runner registered; may still be running a job"
				if !isRegistered {
					keepUntil = created.Add(c.timeouts.setupGrace())
					why = "may still be setting up"
				}
				c.log.Info("dry run: keeping runner VM", "vmid", vm.VMID, "runner", vm.Name, "age", age.Round(time.Second), "why", why, "stale_after", keepUntil.Format(time.TimeOnly))
			}
			continue
		}

		log := c.log.With("vmid", vm.VMID, "runner", vm.Name, "age", age.Round(time.Second), "reason", reason)
		if dryRun {
			log.Info("dry run: would destroy stale runner VM")
			continue
		}
		// Destroy first: GitHub refuses to remove a runner that is still busy.
		if _, err := c.destroyVM(ctx, vm.VMID, vm.Name); err != nil {
			log.Error("destroying stale runner VM failed", "err", err)
			errs = append(errs, err)
			continue
		}
		log.Info("destroyed stale runner VM")
		if isRegistered {
			if err := c.github.RemoveRunner(ctx, reg.Repo, reg.ID); err != nil {
				errs = append(errs, err)
			}
		}
	}

	for name, reg := range registered {
		if inPool[name] {
			continue
		}
		log := c.log.With("runner", name, "repo", reg.Repo.String(), "runner_id", reg.ID)
		if dryRun {
			log.Info("dry run: would remove runner registration with no VM")
			continue
		}
		if err := c.github.RemoveRunner(ctx, reg.Repo, reg.ID); err != nil {
			errs = append(errs, err)
			continue
		}
		log.Info("removed runner registration with no VM")
	}
	return errors.Join(errs...)
}

// runnerCreatedAt reads the creation time from a gha-runner-<vmid>-<unix> name.
func runnerCreatedAt(name string) (time.Time, bool) {
	_, unix, ok := strings.Cut(strings.TrimPrefix(name, runnerNamePrefix), "-")
	if !ok {
		return time.Time{}, false
	}
	sec, err := strconv.ParseInt(unix, 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	return time.Unix(sec, 0), true
}

func (c *Controller) serveNext(ctx context.Context, dryRun bool) error {
	job, err := c.github.OldestQueuedJob(ctx)
	if err != nil {
		return err
	}
	if job == nil {
		if !c.idle {
			c.log.Info("no queued jobs; waiting", "label", c.cfg.RunnerLabel)
			c.idle = true
		}
		return nil
	}
	c.idle = false
	c.log.Info("found queued job", "repo", job.Repo.String(), "job", job.Name, "job_id", job.JobID, "queued_since", job.CreatedAt)

	vmid, err := c.pickVMID(ctx)
	if err != nil {
		return err
	}
	name := fmt.Sprintf("%s%d-%d", runnerNamePrefix, vmid, c.now().Unix())

	if dryRun {
		c.log.Info("dry run: would clone and run", "template", c.cfg.TemplateVMID, "vmid", vmid, "name", name, "pool", c.cfg.RunnerPool, "repo", job.Repo.String())
		return nil
	}
	return c.serve(ctx, job.Repo, vmid, name)
}

// pickVMID enforces the pool cap and returns the first free ID in the range.
func (c *Controller) pickVMID(ctx context.Context) (int, error) {
	vms, err := c.proxmox.PoolVMs(ctx, c.cfg.RunnerPool)
	if err != nil {
		return 0, err
	}
	inPool := map[int]bool{}
	for _, vm := range vms {
		if !c.ownsVM(vm) {
			return 0, fmt.Errorf("pool %s contains VM %d %q that is not a runner; refusing to continue", c.cfg.RunnerPool, vm.VMID, vm.Name)
		}
		inPool[vm.VMID] = true
	}
	if len(vms) >= c.cfg.MaxPoolVMs {
		return 0, fmt.Errorf("pool %s already has %d runner VMs (cap %d); destroy stale ones before starting another", c.cfg.RunnerPool, len(vms), c.cfg.MaxPoolVMs)
	}
	for id := c.cfg.VMIDMin; id <= c.cfg.VMIDMax; id++ {
		if inPool[id] {
			continue
		}
		free, err := c.proxmox.VMIDFree(ctx, id)
		if err != nil {
			return 0, err
		}
		if free {
			return id, nil
		}
	}
	return 0, fmt.Errorf("no free VM ID in %d-%d", c.cfg.VMIDMin, c.cfg.VMIDMax)
}

// ownsVM is the guardrail for anything destructive: only VMs in the ID range
// with the runner name prefix are ever touched.
func (c *Controller) ownsVM(vm PoolVM) bool {
	return vm.VMID >= c.cfg.VMIDMin && vm.VMID <= c.cfg.VMIDMax && strings.HasPrefix(vm.Name, runnerNamePrefix)
}

func (c *Controller) serve(ctx context.Context, repo Repository, vmid int, name string) (err error) {
	log := c.log.With("vmid", vmid, "runner", name, "repo", repo.String())

	// Cleanup runs on every path, including Ctrl-C, with its own deadline.
	var runnerID int64
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), c.timeouts.Cleanup)
		defer cancel()
		if runnerID != 0 {
			if rerr := c.github.RemoveRunner(cleanupCtx, repo, runnerID); rerr != nil {
				log.Error("removing runner registration failed", "err", rerr)
				err = errors.Join(err, rerr)
			}
		}
		destroyed, derr := c.destroyVM(cleanupCtx, vmid, name)
		switch {
		case derr != nil:
			log.Error("destroying VM failed; it stays in the pool and counts toward the cap", "err", derr)
			err = errors.Join(err, derr)
		case destroyed:
			log.Info("destroyed runner VM")
		default:
			log.Info("no runner VM to destroy; clone never completed")
		}
	}()

	log.Info("cloning runner VM", "template", c.cfg.TemplateVMID)
	if err := c.proxmox.Clone(ctx, c.cfg.TemplateVMID, vmid, name, c.cfg.RunnerPool); err != nil {
		return err
	}
	if err := c.proxmox.ConfigureCloudInit(ctx, vmid); err != nil {
		return err
	}
	if err := c.proxmox.Start(ctx, vmid); err != nil {
		return err
	}
	log.Info("waiting for guest agent")
	if err := c.waitFor(ctx, c.timeouts.AgentReady, "guest agent", func() (bool, error) {
		return c.proxmox.AgentPing(ctx, vmid) == nil, nil
	}); err != nil {
		return err
	}

	// Register only once the VM is ready, so a failed boot leaves nothing on GitHub.
	runnerID, jitConfig, err := c.github.CreateRunner(ctx, repo, name)
	if err != nil {
		return err
	}
	if err := c.proxmox.WriteFile(ctx, vmid, jitConfigPath, jitConfig); err != nil {
		return err
	}
	log.Info("runner config handed off; waiting for runner to come online", "runner_id", runnerID)

	var finished bool
	if err := c.waitFor(ctx, c.timeouts.RunnerOnline, "runner online", func() (bool, error) {
		exists, status, err := c.github.RunnerState(ctx, repo, runnerID)
		finished = !exists // a very short job can finish between polls
		return finished || status == "online", err
	}); err != nil {
		return err
	}
	if !finished {
		log.Info("runner online; waiting for its job to finish")
		if err := c.waitFor(ctx, c.timeouts.Job, "job", func() (bool, error) {
			exists, _, err := c.github.RunnerState(ctx, repo, runnerID)
			return !exists, err
		}); err != nil {
			return err
		}
	}
	log.Info("job finished; runner deregistered")
	runnerID = 0 // GitHub already removed the ephemeral registration
	return nil
}

// destroyVM stops and deletes the VM, but only after re-checking that it is a
// runner VM in the pool. It reports false when there was no VM to destroy.
func (c *Controller) destroyVM(ctx context.Context, vmid int, name string) (bool, error) {
	vms, err := c.proxmox.PoolVMs(ctx, c.cfg.RunnerPool)
	if err != nil {
		return false, err
	}
	idx := -1
	for i, vm := range vms {
		if vm.VMID == vmid {
			idx = i
		}
	}
	if idx < 0 {
		return false, nil
	}
	if vm := vms[idx]; !c.ownsVM(vm) || vm.Name != name {
		return false, fmt.Errorf("VM %d is %q, not %q; refusing to destroy it", vmid, vm.Name, name)
	}
	if status, err := c.proxmox.Status(ctx, vmid); err != nil {
		return false, err
	} else if status == "running" {
		if err := c.proxmox.Stop(ctx, vmid); err != nil {
			return false, err
		}
	}
	return true, c.proxmox.Destroy(ctx, vmid)
}

// waitFor polls check until it reports done, returns an error, or the timeout passes.
func (c *Controller) waitFor(ctx context.Context, timeout time.Duration, what string, check func() (bool, error)) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		done, err := check()
		if err != nil {
			return fmt.Errorf("waiting for %s: %w", what, err)
		}
		if done {
			return nil
		}
		if err := sleep(ctx, c.timeouts.Poll); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return fmt.Errorf("timed out after %s waiting for %s", timeout, what)
			}
			return fmt.Errorf("waiting for %s: %w", what, err)
		}
	}
}
