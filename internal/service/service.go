package service

import (
	"context"
	"math/rand"
	"time"

	"Avito2025/internal/domain"
	"Avito2025/internal/storage"
)

type Service interface {
	CreateTeam(ctx context.Context, team domain.Team) (domain.Team, error)
	GetTeam(ctx context.Context, name string) (domain.Team, error)
	SetUserActive(ctx context.Context, userID string, isActive bool) (domain.User, error)

	CreatePullRequest(ctx context.Context, pr domain.PullRequest) (domain.PullRequest, error)
	MergePullRequest(ctx context.Context, prID string) (domain.PullRequest, error)
	ReassignReviewer(ctx context.Context, prID, oldReviewerID string) (domain.PullRequest, string, error)
	ListUserReviews(ctx context.Context, userID string) ([]domain.PullRequest, error)
	Health(ctx context.Context) error
}

type ReviewerService struct {
	repo storage.Repository
	rnd  *rand.Rand
}

func New(repo storage.Repository) *ReviewerService {
	return &ReviewerService{
		repo: repo,
		rnd:  rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (s *ReviewerService) CreateTeam(ctx context.Context, team domain.Team) (domain.Team, error) {
	return s.repo.CreateTeam(ctx, team)
}

func (s *ReviewerService) GetTeam(ctx context.Context, name string) (domain.Team, error) {
	return s.repo.GetTeam(ctx, name)
}

func (s *ReviewerService) SetUserActive(ctx context.Context, userID string, isActive bool) (domain.User, error) {
	return s.repo.SetUserActive(ctx, userID, isActive)
}

func (s *ReviewerService) CreatePullRequest(ctx context.Context, pr domain.PullRequest) (domain.PullRequest, error) {
	author, err := s.repo.GetUser(ctx, pr.AuthorID)
	if err != nil {
		return domain.PullRequest{}, err
	}

	members, err := s.repo.ListUsersByTeam(ctx, author.TeamName)
	if err != nil {
		return domain.PullRequest{}, err
	}

	candidates := filterReviewers(members, pr.AuthorID)
	pr.AssignedReviewers = pickReviewers(s.rnd, candidates, 2)
	pr.Status = domain.StatusOpen
	pr.CreatedAt = time.Now().UTC()

	return s.repo.CreatePullRequest(ctx, pr)
}

func (s *ReviewerService) MergePullRequest(ctx context.Context, prID string) (domain.PullRequest, error) {
	pr, err := s.repo.GetPullRequest(ctx, prID)
	if err != nil {
		return domain.PullRequest{}, err
	}

	if pr.Status == domain.StatusMerged {
		return pr, nil
	}

	now := time.Now().UTC()
	pr.Status = domain.StatusMerged
	pr.MergedAt = &now

	return s.repo.UpdatePullRequest(ctx, pr)
}

func (s *ReviewerService) ReassignReviewer(ctx context.Context, prID, oldReviewerID string) (domain.PullRequest, string, error) {
	pr, err := s.repo.GetPullRequest(ctx, prID)
	if err != nil {
		return domain.PullRequest{}, "", err
	}

	if pr.Status == domain.StatusMerged {
		return domain.PullRequest{}, "", domain.ErrPRMerged
	}

	index := reviewerIndex(pr.AssignedReviewers, oldReviewerID)
	if index == -1 {
		return domain.PullRequest{}, "", domain.ErrReviewerNotFound
	}

	oldReviewer, err := s.repo.GetUser(ctx, oldReviewerID)
	if err != nil {
		return domain.PullRequest{}, "", err
	}

	members, err := s.repo.ListUsersByTeam(ctx, oldReviewer.TeamName)
	if err != nil {
		return domain.PullRequest{}, "", err
	}

	candidates := filterForReplacement(members, oldReviewerID, pr.AssignedReviewers)
	if len(candidates) == 0 {
		return domain.PullRequest{}, "", domain.ErrNoReplacement
	}

	replacement := pickReviewers(s.rnd, candidates, 1)
	if len(replacement) == 0 {
		return domain.PullRequest{}, "", domain.ErrNoReplacement
	}

	pr.AssignedReviewers[index] = replacement[0]
	updatedPR, err := s.repo.UpdatePullRequest(ctx, pr)
	if err != nil {
		return domain.PullRequest{}, "", err
	}

	return updatedPR, replacement[0], nil
}

func (s *ReviewerService) ListUserReviews(ctx context.Context, userID string) ([]domain.PullRequest, error) {
	return s.repo.ListPullRequestsByReviewer(ctx, userID)
}

func (s *ReviewerService) Health(ctx context.Context) error {
	return s.repo.Health(ctx)
}

func filterReviewers(users []domain.User, authorID string) []domain.User {
	candidates := make([]domain.User, 0, len(users))
	for _, user := range users {
		if user.ID == authorID {
			continue
		}
		if !user.IsActive {
			continue
		}
		candidates = append(candidates, user)
	}
	return candidates
}

func filterForReplacement(users []domain.User, oldReviewerID string, assigned []string) []domain.User {
	candidates := make([]domain.User, 0, len(users))
	for _, user := range users {
		if user.ID == oldReviewerID {
			continue
		}
		if !user.IsActive {
			continue
		}
		if contains(assigned, user.ID) {
			continue
		}
		candidates = append(candidates, user)
	}
	return candidates
}

func pickReviewers(rnd *rand.Rand, users []domain.User, limit int) []string {
	if len(users) == 0 || limit <= 0 {
		return nil
	}

	copyUsers := append([]domain.User(nil), users...)
	rnd.Shuffle(len(copyUsers), func(i, j int) {
		copyUsers[i], copyUsers[j] = copyUsers[j], copyUsers[i]
	})

	if len(copyUsers) < limit {
		limit = len(copyUsers)
	}

	result := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		result = append(result, copyUsers[i].ID)
	}
	return result
}

func reviewerIndex(reviewers []string, target string) int {
	for i, reviewer := range reviewers {
		if reviewer == target {
			return i
		}
	}
	return -1
}

func contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
