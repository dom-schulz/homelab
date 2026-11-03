package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-github/v93/github"
)

func fakeGitHubAPI(t *testing.T, routes map[string]string) *GitHub {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		if status := r.URL.Query().Get("status"); status != "" {
			key += "?status=" + status
		}
		body, ok := routes[key]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)

	base := srv.URL + "/"
	client, err := github.NewClient(github.WithURLs(&base, &base))
	if err != nil {
		t.Fatal(err)
	}
	return &GitHub{
		client: client,
		repos:  []Repository{{Owner: "me", Name: "a"}, {Owner: "me", Name: "b"}},
		label:  "homelab-ephemeral",
	}
}

func TestOldestQueuedJobAcrossRepositories(t *testing.T) {
	g := fakeGitHubAPI(t, map[string]string{
		"GET /repos/me/a/actions/runs?status=queued":      `{"workflow_runs":[{"id":1}]}`,
		"GET /repos/me/a/actions/runs?status=in_progress": `{"workflow_runs":[]}`,
		"GET /repos/me/a/actions/runs/1/jobs": `{"jobs":[
			{"id":11,"name":"newer","status":"queued","labels":["self-hosted","homelab-ephemeral"],"created_at":"2026-10-27T10:05:00Z"},
			{"id":12,"name":"other label","status":"queued","labels":["ubuntu-latest"],"created_at":"2026-10-27T09:00:00Z"}]}`,
		"GET /repos/me/b/actions/runs?status=queued":      `{"workflow_runs":[]}`,
		"GET /repos/me/b/actions/runs?status=in_progress": `{"workflow_runs":[{"id":2}]}`,
		"GET /repos/me/b/actions/runs/2/jobs": `{"jobs":[
			{"id":21,"name":"running","status":"in_progress","labels":["homelab-ephemeral"],"created_at":"2026-10-27T08:00:00Z"},
			{"id":22,"name":"oldest","status":"queued","labels":["homelab-ephemeral"],"created_at":"2026-10-27T10:00:00Z"}]}`,
	})

	job, err := g.OldestQueuedJob(context.Background())
	if err != nil {
		t.Fatalf("OldestQueuedJob: %v", err)
	}
	if job == nil || job.JobID != 22 || job.Repo.Name != "b" {
		t.Fatalf("job = %+v, want job 22 in me/b", job)
	}
}

func TestRunnerStateTreatsNotFoundAsGone(t *testing.T) {
	g := fakeGitHubAPI(t, map[string]string{
		"GET /repos/me/a/actions/runners/5": `{"id":5,"name":"gha-runner-9200-1","status":"online"}`,
	})
	exists, status, err := g.RunnerState(context.Background(), g.repos[0], 5)
	if err != nil || !exists || status != "online" {
		t.Errorf("runner 5 = %v %q %v", exists, status, err)
	}
	exists, _, err = g.RunnerState(context.Background(), g.repos[0], 6)
	if err != nil || exists {
		t.Errorf("missing runner = %v %v, want gone", exists, err)
	}
	if err := g.RemoveRunner(context.Background(), g.repos[0], 6); err != nil {
		t.Errorf("removing a gone runner: %v", err)
	}
}

func TestListRunnersKeepsOnlyControllerRunnersAcrossPages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/repos/me/a/actions/runners" && r.URL.Query().Get("page") == "":
			w.Header().Set("Link", `<`+"http://"+r.Host+`/repos/me/a/actions/runners?page=2>; rel="next"`)
			fmt.Fprint(w, `{"runners":[{"id":1,"name":"gha-runner-9200-1","status":"online"},{"id":2,"name":"laptop","status":"online"}]}`)
		case r.URL.Path == "/repos/me/a/actions/runners":
			fmt.Fprint(w, `{"runners":[{"id":3,"name":"gha-runner-9201-1","status":"offline"}]}`)
		default:
			fmt.Fprint(w, `{"runners":[]}`)
		}
	}))
	defer srv.Close()
	base := srv.URL + "/"
	client, _ := github.NewClient(github.WithURLs(&base, &base))
	g := &GitHub{client: client, repos: []Repository{{Owner: "me", Name: "a"}, {Owner: "me", Name: "b"}}}

	runners, err := g.ListRunners(context.Background())
	if err != nil {
		t.Fatalf("ListRunners: %v", err)
	}
	if len(runners) != 2 || runners[0].ID != 1 || runners[1].ID != 3 || runners[1].Status != "offline" {
		t.Errorf("runners = %+v, want IDs 1 and 3", runners)
	}
}
