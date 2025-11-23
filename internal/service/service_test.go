package service_test

import (
	"context"
	"testing"
	"time"

	"Avito2025/internal/config"
	"Avito2025/internal/domain"
	"Avito2025/internal/service"
	"Avito2025/internal/storage/postgres"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestCreatePullRequestAssignsReviewers(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t, ctx)
	defer store.Close()
	svc := service.New(store)

	createTeam(t, ctx, svc, domain.Team{
		Name: "backend",
		Members: []domain.User{
			{ID: "u1", Username: "Alice", IsActive: true},
			{ID: "u2", Username: "Bob", IsActive: true},
			{ID: "u3", Username: "Charlie", IsActive: true},
		},
	})

	pr, err := svc.CreatePullRequest(ctx, domain.PullRequest{
		ID:       "pr-1",
		Name:     "Initial",
		AuthorID: "u1",
	})
	if err != nil {
		t.Fatalf("CreatePullRequest: %v", err)
	}

	if got := len(pr.AssignedReviewers); got != 2 {
		t.Fatalf("expected 2 reviewers, got %d: %+v", got, pr.AssignedReviewers)
	}
	for _, reviewer := range pr.AssignedReviewers {
		if reviewer == "u1" {
			t.Fatalf("author should not be reviewer: %+v", pr.AssignedReviewers)
		}
	}
}

func TestReassignReviewer(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t, ctx)
	defer store.Close()
	svc := service.New(store)

	createTeam(t, ctx, svc, domain.Team{
		Name: "backend",
		Members: []domain.User{
			{ID: "u1", Username: "Alice", IsActive: true},
			{ID: "u2", Username: "Bob", IsActive: true},
			{ID: "u3", Username: "Charlie", IsActive: true},
			{ID: "u4", Username: "Dora", IsActive: true},
		},
	})

	pr, err := svc.CreatePullRequest(ctx, domain.PullRequest{
		ID:       "pr-2",
		Name:     "Replace reviewer",
		AuthorID: "u1",
	})
	if err != nil {
		t.Fatalf("CreatePullRequest: %v", err)
	}

	var oldReviewer string
	for _, r := range pr.AssignedReviewers {
		if r != "u3" {
			oldReviewer = r
			break
		}
	}
	if oldReviewer == "" {
		oldReviewer = pr.AssignedReviewers[0]
	}

	updatedPR, replacedBy, err := svc.ReassignReviewer(ctx, pr.ID, oldReviewer)
	if err != nil {
		t.Fatalf("ReassignReviewer: %v", err)
	}
	if replacedBy == oldReviewer {
		t.Fatalf("reviewer was not replaced: %s", replacedBy)
	}
	if !contains(updatedPR.AssignedReviewers, replacedBy) {
		t.Fatalf("new reviewer not assigned: %s", replacedBy)
	}
	if contains(updatedPR.AssignedReviewers, oldReviewer) {
		t.Fatalf("old reviewer still assigned: %s", oldReviewer)
	}
}

func TestMergePullRequestIdempotent(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t, ctx)
	defer store.Close()
	svc := service.New(store)

	createTeam(t, ctx, svc, domain.Team{
		Name: "backend",
		Members: []domain.User{
			{ID: "u1", Username: "Alice", IsActive: true},
			{ID: "u2", Username: "Bob", IsActive: true},
		},
	})

	pr, err := svc.CreatePullRequest(ctx, domain.PullRequest{
		ID:       "pr-3",
		Name:     "Merge twice",
		AuthorID: "u1",
	})
	if err != nil {
		t.Fatalf("CreatePullRequest: %v", err)
	}

	first, err := svc.MergePullRequest(ctx, pr.ID)
	if err != nil {
		t.Fatalf("MergePullRequest first: %v", err)
	}
	second, err := svc.MergePullRequest(ctx, pr.ID)
	if err != nil {
		t.Fatalf("MergePullRequest second: %v", err)
	}

	if first.Status != domain.StatusMerged || second.Status != domain.StatusMerged {
		t.Fatalf("status not merged: %s / %s", first.Status, second.Status)
	}
	if first.MergedAt == nil || second.MergedAt == nil {
		t.Fatalf("mergedAt not set")
	}
	if !first.MergedAt.Equal(*second.MergedAt) {
		t.Fatalf("mergedAt differs between idempotent calls")
	}
}

func newTestStore(t *testing.T, ctx context.Context) *postgres.Store {
	t.Helper()

	postgresContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "postgres:15-alpine",
			ExposedPorts: []string{"5432/tcp"},
			Env: map[string]string{
				"POSTGRES_USER":     "test",
				"POSTGRES_PASSWORD": "test",
				"POSTGRES_DB":       "test",
			},
			WaitingFor: wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}

	t.Cleanup(func() {
		if err := postgresContainer.Terminate(ctx); err != nil {
			t.Logf("failed to terminate postgres container: %v", err)
		}
	})

	host, err := postgresContainer.Host(ctx)
	if err != nil {
		t.Fatalf("failed to get postgres host: %v", err)
	}

	port, err := postgresContainer.MappedPort(ctx, "5432")
	if err != nil {
		t.Fatalf("failed to get postgres port: %v", err)
	}

	pgConfig := config.PostgresConfig{
		Host:     host,
		Port:     port.Port(),
		User:     "test",
		Password: "test",
		DBName:   "test",
		SSLMode:  "disable",
		MaxConns: 4,
	}

	store, err := postgres.New(ctx, pgConfig)
	if err != nil {
		t.Fatalf("failed to create postgres store: %v", err)
	}

	t.Cleanup(func() {
		store.Close()
	})

	return store
}

func createTeam(t *testing.T, ctx context.Context, svc service.Service, team domain.Team) {
	t.Helper()
	if _, err := svc.CreateTeam(ctx, team); err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
}

func contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
