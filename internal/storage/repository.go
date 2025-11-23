package storage

import (
	"context"

	"Avito2025/internal/domain"
)

type Repository interface {
	CreateTeam(ctx context.Context, team domain.Team) (domain.Team, error)
	GetTeam(ctx context.Context, name string) (domain.Team, error)
	GetUser(ctx context.Context, userID string) (domain.User, error)
	SetUserActive(ctx context.Context, userID string, isActive bool) (domain.User, error)
	ListUsersByTeam(ctx context.Context, teamName string) ([]domain.User, error)

	CreatePullRequest(ctx context.Context, pr domain.PullRequest) (domain.PullRequest, error)
	UpdatePullRequest(ctx context.Context, pr domain.PullRequest) (domain.PullRequest, error)
	GetPullRequest(ctx context.Context, id string) (domain.PullRequest, error)
	ListPullRequestsByReviewer(ctx context.Context, userID string) ([]domain.PullRequest, error)

	Health(ctx context.Context) error
}
