package httptransport

import (
	"errors"
	"fmt"

	"Avito2025/internal/domain"
)

type teamRequest struct {
	TeamName string              `json:"team_name"`
	Members  []teamMemberRequest `json:"members"`
}

type teamMemberRequest struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	IsActive bool   `json:"is_active"`
}

func (t teamRequest) validate() error {
	if t.TeamName == "" {
		return errors.New("team_name is required")
	}
	if len(t.Members) == 0 {
		return errors.New("members are required")
	}
	for i, member := range t.Members {
		if member.UserID == "" {
			return fmt.Errorf("members[%d].user_id is required", i)
		}
		if member.Username == "" {
			return fmt.Errorf("members[%d].username is required", i)
		}
	}
	return nil
}

func (t teamRequest) toDomain() domain.Team {
	members := make([]domain.User, 0, len(t.Members))
	for _, member := range t.Members {
		members = append(members, domain.User{
			ID:       member.UserID,
			Username: member.Username,
			TeamName: t.TeamName,
			IsActive: member.IsActive,
		})
	}

	return domain.Team{
		Name:    t.TeamName,
		Members: members,
	}
}

type setUserActiveRequest struct {
	UserID   string `json:"user_id"`
	IsActive bool   `json:"is_active"`
}

func (r setUserActiveRequest) validate() error {
	if r.UserID == "" {
		return errors.New("user_id is required")
	}
	return nil
}

type createPRRequest struct {
	ID       string `json:"pull_request_id"`
	Name     string `json:"pull_request_name"`
	AuthorID string `json:"author_id"`
}

func (r createPRRequest) validate() error {
	if r.ID == "" {
		return errors.New("pull_request_id is required")
	}
	if r.Name == "" {
		return errors.New("pull_request_name is required")
	}
	if r.AuthorID == "" {
		return errors.New("author_id is required")
	}
	return nil
}

type mergePRRequest struct {
	ID string `json:"pull_request_id"`
}

func (r mergePRRequest) validate() error {
	if r.ID == "" {
		return errors.New("pull_request_id is required")
	}
	return nil
}

type reassignRequest struct {
	PullRequestID string `json:"pull_request_id"`
	OldUserID     string `json:"old_user_id"`
}

func (r reassignRequest) validate() error {
	if r.PullRequestID == "" {
		return errors.New("pull_request_id is required")
	}
	if r.OldUserID == "" {
		return errors.New("old_user_id is required")
	}
	return nil
}
