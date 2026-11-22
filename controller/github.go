package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v93/github"
)

// defaultRunnerGroupID is the "Default" group every repository has.
const defaultRunnerGroupID = 1

// GitHub wraps the Actions API calls the controller makes as the GitHub App.
type GitHub struct {
	client *github.Client
	repos  []Repository
	label  string
}

func NewGitHub(cfg Config) (*GitHub, error) {
	transport, err := ghinstallation.NewKeyFromFile(http.DefaultTransport, cfg.GitHubAppID, cfg.GitHubInstallationID, cfg.GitHubPrivateKeyFile)
	if err != nil {
		return nil, fmt.Errorf("loading GitHub App key: %w", err)
	}
	client, err := github.NewClient(github.WithHTTPClient(&http.Client{Transport: transport, Timeout: 30 * time.Second}))
	if err != nil {
		return nil, err
	}
	return &GitHub{client: client, repos: cfg.Repositories, label: cfg.RunnerLabel}, nil
}

type QueuedJob struct {
	Repo      Repository
	RunID     int64
	JobID     int64
	Name      string
	CreatedAt time.Time
}

// OldestQueuedJob returns the queued job with the runner label that has waited
// longest across all allowlisted repositories, or nil if there is none.
func (g *GitHub) OldestQueuedJob(ctx context.Context) (*QueuedJob, error) {
	var oldest *QueuedJob
	for _, repo := range g.repos {
		// A run with one waiting job is "queued"; a run with another job already
		// running is "in_progress", so check both.
		for _, status := range []string{"queued", "in_progress"} {
			runs, _, err := g.client.Actions.ListRepositoryWorkflowRuns(ctx, repo.Owner, repo.Name, &github.ListWorkflowRunsOptions{
				Status:      status,
				ListOptions: github.ListOptions{PerPage: 100},
			})
			if err != nil {
				return nil, fmt.Errorf("listing %s runs in %s: %w", status, repo, err)
			}
			for _, run := range runs.WorkflowRuns {
				if !trustedRun(run, repo) {
					continue
				}
				jobs, _, err := g.client.Actions.ListWorkflowJobs(ctx, repo.Owner, repo.Name, run.GetID(), &github.ListWorkflowJobsOptions{
					ListOptions: github.ListOptions{PerPage: 100},
				})
				if err != nil {
					return nil, fmt.Errorf("listing jobs for run %d in %s: %w", run.GetID(), repo, err)
				}
				for _, job := range jobs.Jobs {
					if job.GetStatus() != "queued" || !slices.Contains(job.Labels, g.label) {
						continue
					}
					created := job.GetCreatedAt().Time
					if oldest == nil || created.Before(oldest.CreatedAt) {
						oldest = &QueuedJob{Repo: repo, RunID: run.GetID(), JobID: job.GetID(), Name: job.GetName(), CreatedAt: created}
					}
				}
			}
		}
	}
	return oldest, nil
}

// trustedEvents are the triggers only someone with write access can start.
// Pull request events are left out: a fork's PR runs workflow files from the
// fork, so anyone could otherwise get a runner VM on the homelab.
var trustedEvents = []string{"push", "workflow_dispatch", "schedule"}

// trustedRun reports whether a run may get a runner: a trusted event on code
// from the repository itself, never from a fork.
func trustedRun(run *github.WorkflowRun, repo Repository) bool {
	return slices.Contains(trustedEvents, run.GetEvent()) &&
		run.GetHeadRepository().GetFullName() == repo.String()
}

// CreateRunner registers a just-in-time runner and returns its ID and the
// encoded config the VM starts it with. The registration is single-use.
func (g *GitHub) CreateRunner(ctx context.Context, repo Repository, name string) (int64, string, error) {
	cfg, _, err := g.client.Actions.CreateRepoJITConfig(ctx, repo.Owner, repo.Name, github.CreateJITConfigRequest{
		Name:          name,
		RunnerGroupID: defaultRunnerGroupID,
		Labels:        []string{"self-hosted", "linux", "x64", g.label},
	})
	if err != nil {
		return 0, "", fmt.Errorf("creating runner %s in %s: %w", name, repo, err)
	}
	return cfg.GetRunner().GetID(), cfg.GetEncodedJITConfig(), nil
}

// RunnerState reports whether the runner is still registered and its status
// ("online" or "offline"). An ephemeral runner disappears after its one job.
func (g *GitHub) RunnerState(ctx context.Context, repo Repository, runnerID int64) (exists bool, status string, err error) {
	runner, _, err := g.client.Actions.GetRunner(ctx, repo.Owner, repo.Name, runnerID)
	if isNotFound(err) {
		return false, "", nil
	}
	if err != nil {
		return false, "", fmt.Errorf("getting runner %d in %s: %w", runnerID, repo, err)
	}
	return true, runner.GetStatus(), nil
}

type RegisteredRunner struct {
	Repo   Repository
	ID     int64
	Name   string
	Status string
}

// ListRunners returns the controller's runners (named with runnerNamePrefix)
// registered in the allowlisted repositories.
func (g *GitHub) ListRunners(ctx context.Context) ([]RegisteredRunner, error) {
	var out []RegisteredRunner
	for _, repo := range g.repos {
		opts := &github.ListRunnersOptions{ListOptions: github.ListOptions{PerPage: 100}}
		for {
			runners, resp, err := g.client.Actions.ListRunners(ctx, repo.Owner, repo.Name, opts)
			if err != nil {
				return nil, fmt.Errorf("listing runners in %s: %w", repo, err)
			}
			for _, r := range runners.Runners {
				if strings.HasPrefix(r.GetName(), runnerNamePrefix) {
					out = append(out, RegisteredRunner{Repo: repo, ID: r.GetID(), Name: r.GetName(), Status: r.GetStatus()})
				}
			}
			if resp.NextPage == 0 {
				break
			}
			opts.Page = resp.NextPage
		}
	}
	return out, nil
}

// RemoveRunner deletes a registration; one that is already gone is fine.
func (g *GitHub) RemoveRunner(ctx context.Context, repo Repository, runnerID int64) error {
	_, err := g.client.Actions.RemoveRunner(ctx, repo.Owner, repo.Name, runnerID)
	if err != nil && !isNotFound(err) {
		return fmt.Errorf("removing runner %d in %s: %w", runnerID, repo, err)
	}
	return nil
}

func isNotFound(err error) bool {
	var gerr *github.ErrorResponse
	return errors.As(err, &gerr) && gerr.Response != nil && gerr.Response.StatusCode == http.StatusNotFound
}
