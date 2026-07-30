package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/hxly520/sub2api/points-system/internal/config"
	"github.com/hxly520/sub2api/points-system/internal/store"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "points history backfill:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("expected activate, plan, apply, resume, or status")
	}
	switch args[0] {
	case "activate":
		return runActivate(ctx, args[1:])
	case "plan":
		return runPlan(ctx, args[1:])
	case "apply":
		return runApply(ctx, args[1:])
	case "resume":
		return runResume(ctx, args[1:])
	case "status":
		return runStatus(ctx, args[1:])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runActivate(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("activate", flag.ContinueOnError)
	actor := flags.Int64("actor-user-id", 0, "Sub2API administrator user ID")
	ratio := flags.Int64("points-per-usd-hundredths", 1000, "hundredths of a point per USD")
	if err := flags.Parse(args); err != nil {
		return err
	}
	pointsStore, cleanup, err := openStore(ctx, false)
	if err != nil {
		return err
	}
	defer cleanup()
	policy, err := pointsStore.CreateInitialActivationPolicy(ctx, *actor, *ratio, time.Now())
	if err != nil {
		return err
	}
	return emit(map[string]any{"event": "initial_policy_ready", "policy": policy})
}

func runPlan(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("plan", flag.ContinueOnError)
	from := flags.String("from", "auto", "first business date or auto")
	through := flags.String("through", "", "last completed business date; defaults to yesterday")
	policyVersion := flags.Int64("policy-version", 0, "immutable enabled policy version")
	if err := flags.Parse(args); err != nil {
		return err
	}
	pointsStore, cleanup, err := openStore(ctx, true)
	if err != nil {
		return err
	}
	defer cleanup()
	plan, err := buildPlan(ctx, pointsStore, *from, *through, *policyVersion)
	if err != nil {
		return err
	}
	return emit(map[string]any{"event": "dry_run", "plan": plan})
}

func runApply(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("apply", flag.ContinueOnError)
	from := flags.String("from", "auto", "first business date or auto")
	through := flags.String("through", "", "last completed business date; defaults to yesterday")
	policyVersion := flags.Int64("policy-version", 0, "immutable enabled policy version")
	actor := flags.Int64("actor-user-id", 0, "Sub2API administrator user ID")
	confirmation := flags.String("confirm-fingerprint", "", "fingerprint from a fresh plan")
	maxDays := flags.Int("max-days", 0, "stop cleanly after this many days; zero runs to completion")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *actor <= 0 || len(strings.TrimSpace(*confirmation)) != 64 || *maxDays < 0 {
		return errors.New("apply requires actor-user-id, a 64-character confirm-fingerprint, and nonnegative max-days")
	}
	pointsStore, cleanup, err := openStore(ctx, true)
	if err != nil {
		return err
	}
	defer cleanup()
	plan, err := buildPlan(ctx, pointsStore, *from, *through, *policyVersion)
	if err != nil {
		return err
	}
	if plan.ConfirmationFingerprint != strings.TrimSpace(*confirmation) {
		return errors.New("dry-run fingerprint changed; run plan again before applying")
	}
	job, err := pointsStore.CreateHistoryBackfillJob(ctx, plan, *actor, time.Now())
	if err != nil {
		return err
	}
	if err := emit(map[string]any{"event": "job_created", "job": job}); err != nil {
		return err
	}
	return processDays(ctx, pointsStore, job.ID, *maxDays)
}

func runResume(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("resume", flag.ContinueOnError)
	jobID := flags.String("job-id", "", "history backfill job UUID")
	actor := flags.Int64("actor-user-id", 0, "Sub2API administrator user ID")
	maxDays := flags.Int("max-days", 0, "stop cleanly after this many days; zero runs to completion")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*jobID) == "" || *actor <= 0 || *maxDays < 0 {
		return errors.New("resume requires job-id, actor-user-id, and nonnegative max-days")
	}
	pointsStore, cleanup, err := openStore(ctx, true)
	if err != nil {
		return err
	}
	defer cleanup()
	job, err := pointsStore.ResumeHistoryBackfillJob(ctx, strings.TrimSpace(*jobID), *actor)
	if err != nil {
		return err
	}
	if err := emit(map[string]any{"event": "job_resumed", "job": job}); err != nil {
		return err
	}
	return processDays(ctx, pointsStore, job.ID, *maxDays)
}

func runStatus(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("status", flag.ContinueOnError)
	jobID := flags.String("job-id", "", "history backfill job UUID")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*jobID) == "" {
		return errors.New("status requires job-id")
	}
	pointsStore, cleanup, err := openStore(ctx, false)
	if err != nil {
		return err
	}
	defer cleanup()
	job, err := pointsStore.GetHistoryBackfillJob(ctx, strings.TrimSpace(*jobID))
	if err != nil {
		return err
	}
	return emit(map[string]any{"event": "status", "job": job})
}

func buildPlan(ctx context.Context, pointsStore *store.Store, rawFrom, rawThrough string,
	policyVersion int64) (store.HistoryBackfillPlan, error) {
	if policyVersion <= 0 {
		return store.HistoryBackfillPlan{}, errors.New("policy-version must be positive")
	}
	var from, through time.Time
	var err error
	if value := strings.TrimSpace(rawFrom); value != "" && value != "auto" {
		from, err = parseDate(value, pointsStore.Location)
		if err != nil {
			return store.HistoryBackfillPlan{}, fmt.Errorf("parse from: %w", err)
		}
	}
	if value := strings.TrimSpace(rawThrough); value != "" {
		through, err = parseDate(value, pointsStore.Location)
		if err != nil {
			return store.HistoryBackfillPlan{}, fmt.Errorf("parse through: %w", err)
		}
	}
	return pointsStore.PlanHistoryBackfill(ctx, from, through, policyVersion, time.Now())
}

func processDays(ctx context.Context, pointsStore *store.Store, jobID string, maxDays int) error {
	for processed := 0; maxDays == 0 || processed < maxDays; processed++ {
		job, result, done, err := pointsStore.ProcessHistoryBackfillDay(ctx, jobID)
		if err != nil {
			return err
		}
		if result.RunID != "" {
			if err := emit(map[string]any{
				"event": "day_applied", "result": result, "job": job,
			}); err != nil {
				return err
			}
		}
		if done {
			return emit(map[string]any{"event": "job_succeeded", "job": job})
		}
	}
	job, err := pointsStore.GetHistoryBackfillJob(ctx, jobID)
	if err != nil {
		return err
	}
	return emit(map[string]any{"event": "job_paused", "job": job})
}

func openStore(ctx context.Context, withUsage bool) (*store.Store, func(), error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, err
	}
	if cfg.DatabaseMaxConns < 2 {
		return nil, nil, errors.New("history backfill requires POINTS_DATABASE_MAX_CONNS of at least 2")
	}
	// One connection holds the runner advisory lock while the second commits a
	// natural day. Do not inherit the server's larger pool during one-shot work.
	db, err := store.NewPointsPool(ctx, cfg.DatabaseURL, cfg.DatabaseSchema, 2)
	if err != nil {
		return nil, nil, err
	}
	pointsStore := store.New(db, cfg.Timezone)
	if !withUsage {
		return pointsStore, db.Close, nil
	}
	usageDB, err := store.NewReadOnlyUsagePool(ctx, cfg.UsageDatabaseURL)
	if err != nil {
		db.Close()
		return nil, nil, err
	}
	usageSource := store.NewPostgreSQLUsageSource(usageDB)
	if err := usageSource.Validate(ctx); err != nil {
		usageDB.Close()
		db.Close()
		return nil, nil, err
	}
	pointsStore.SetUsageSource(usageSource)
	return pointsStore, func() { usageDB.Close(); db.Close() }, nil
}

func parseDate(value string, location *time.Location) (time.Time, error) {
	return time.ParseInLocation("2006-01-02", value, location)
}

func emit(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}
