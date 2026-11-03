// Command controller serves queued GitHub Actions jobs with one fresh Proxmox
// VM per job. See README.md and docs/runner-controller-access.md.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	const usage = "usage: controller once [-config FILE] [-dry-run] | controller run [-config FILE]"
	if len(args) == 0 || (args[0] != "once" && args[0] != "run") {
		return fmt.Errorf("%s", usage)
	}
	command := args[0]

	home, _ := os.UserHomeDir()
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	configPath := flags.String("config", filepath.Join(home, ".config", "homelab", "controller.env"), "controller config file")
	dryRun := false
	if command == "once" {
		flags.BoolVar(&dryRun, "dry-run", false, "log cleanup and the planned VM, but change nothing")
	}
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}

	cfg, err := LoadConfig(*configPath)
	if err != nil {
		return err
	}
	gh, err := NewGitHub(cfg)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	c := &Controller{
		cfg:      cfg,
		github:   gh,
		proxmox:  NewProxmox(cfg),
		timeouts: defaultTimeouts,
		log:      slog.New(slog.NewTextHandler(os.Stderr, nil)),
		now:      time.Now,
	}
	if command == "run" {
		return c.Run(ctx)
	}
	return c.RunOnce(ctx, dryRun)
}
