package httptransport

import (
	"encoding/json"
	"net/http"
	"time"

	"Avito2025/internal/domain"
)

type errorResponse struct {
	Error errorPayload `json:"error"`
}

type errorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type teamPayload struct {
	TeamName string              `json:"team_name"`
	Members  []teamMemberPayload `json:"members"`
}

type teamMemberPayload struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	IsActive bool   `json:"is_active"`
}

type userPayload struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	TeamName string `json:"team_name"`
	IsActive bool   `json:"is_active"`
}

type pullRequestPayload struct {
	ID                string     `json:"pull_request_id"`
	Name              string     `json:"pull_request_name"`
	AuthorID          string     `json:"author_id"`
	Status            string     `json:"status"`
	AssignedReviewers []string   `json:"assigned_reviewers"`
	CreatedAt         *time.Time `json:"createdAt,omitempty"`
	MergedAt          *time.Time `json:"mergedAt,omitempty"`
}

type pullRequestShortPayload struct {
	ID       string `json:"pull_request_id"`
	Name     string `json:"pull_request_name"`
	AuthorID string `json:"author_id"`
	Status   string `json:"status"`
}

func respondJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func respondError(w http.ResponseWriter, status int, code, message string) {
	respondJSON(w, status, errorResponse{
		Error: errorPayload{
			Code:    code,
			Message: message,
		},
	})
}

func mapTeam(team domain.Team) teamPayload {
	members := make([]teamMemberPayload, 0, len(team.Members))
	for _, member := range team.Members {
		members = append(members, teamMemberPayload{
			UserID:   member.ID,
			Username: member.Username,
			IsActive: member.IsActive,
		})
	}

	return teamPayload{
		TeamName: team.Name,
		Members:  members,
	}
}

func mapUser(user domain.User) userPayload {
	return userPayload{
		UserID:   user.ID,
		Username: user.Username,
		TeamName: user.TeamName,
		IsActive: user.IsActive,
	}
}

func mapPullRequest(pr domain.PullRequest) pullRequestPayload {
	var createdAt *time.Time
	if !pr.CreatedAt.IsZero() {
		ts := pr.CreatedAt
		createdAt = &ts
	}

	return pullRequestPayload{
		ID:                pr.ID,
		Name:              pr.Name,
		AuthorID:          pr.AuthorID,
		Status:            string(pr.Status),
		AssignedReviewers: append([]string(nil), pr.AssignedReviewers...),
		CreatedAt:         createdAt,
		MergedAt:          pr.MergedAt,
	}
}

func mapPullRequestShort(pr domain.PullRequest) map[string]any {
	return map[string]any{
		"pull_request_id":   pr.ID,
		"pull_request_name": pr.Name,
		"author_id":         pr.AuthorID,
		"status":            string(pr.Status),
	}
}
