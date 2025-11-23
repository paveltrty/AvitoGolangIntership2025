package httptransport

import (
	"encoding/json"
	"net/http"

	"Avito2025/internal/domain"
	"Avito2025/internal/service"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Handler struct {
	service service.Service
}

func NewHandler(svc service.Service) *Handler {
	return &Handler{
		service: svc,
	}
}

func (h *Handler) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Logger)

	r.Route("/team", func(r chi.Router) {
		r.Post("/add", h.CreateTeam)
		r.Get("/get", h.GetTeam)
	})

	r.Route("/users", func(r chi.Router) {
		r.Post("/setIsActive", h.SetUserActive)
		r.Get("/getReview", h.GetUserReviews)
	})

	r.Route("/pullRequest", func(r chi.Router) {
		r.Post("/create", h.CreatePullRequest)
		r.Post("/merge", h.MergePullRequest)
		r.Post("/reassign", h.ReassignReviewer)
	})

	r.Get("/health", h.Health)

	return r
}

func (h *Handler) CreateTeam(w http.ResponseWriter, r *http.Request) {
	var req teamRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if err := req.validate(); err != nil {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	team := req.toDomain()
	created, err := h.service.CreateTeam(r.Context(), team)
	if err != nil {
		h.handleDomainError(w, err)
		return
	}

	respondJSON(w, http.StatusCreated, map[string]any{
		"team": mapTeam(created),
	})
}

func (h *Handler) GetTeam(w http.ResponseWriter, r *http.Request) {
	teamName := r.URL.Query().Get("team_name")
	if teamName == "" {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", "team_name is required")
		return
	}

	team, err := h.service.GetTeam(r.Context(), teamName)
	if err != nil {
		h.handleDomainError(w, err)
		return
	}

	respondJSON(w, http.StatusOK, mapTeam(team))
}

func (h *Handler) SetUserActive(w http.ResponseWriter, r *http.Request) {
	var req setUserActiveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if err := req.validate(); err != nil {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	user, err := h.service.SetUserActive(r.Context(), req.UserID, req.IsActive)
	if err != nil {
		h.handleDomainError(w, err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"user": mapUser(user),
	})
}

func (h *Handler) CreatePullRequest(w http.ResponseWriter, r *http.Request) {
	var req createPRRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if err := req.validate(); err != nil {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	pr, err := h.service.CreatePullRequest(r.Context(), domain.PullRequest{
		ID:       req.ID,
		Name:     req.Name,
		AuthorID: req.AuthorID,
	})
	if err != nil {
		h.handleDomainError(w, err)
		return
	}

	respondJSON(w, http.StatusCreated, map[string]any{
		"pr": mapPullRequest(pr),
	})
}

func (h *Handler) MergePullRequest(w http.ResponseWriter, r *http.Request) {
	var req mergePRRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if err := req.validate(); err != nil {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	pr, err := h.service.MergePullRequest(r.Context(), req.ID)
	if err != nil {
		h.handleDomainError(w, err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"pr": mapPullRequest(pr),
	})
}

func (h *Handler) ReassignReviewer(w http.ResponseWriter, r *http.Request) {
	var req reassignRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if err := req.validate(); err != nil {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	pr, replacedBy, err := h.service.ReassignReviewer(r.Context(), req.PullRequestID, req.OldUserID)
	if err != nil {
		h.handleDomainError(w, err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"pr":          mapPullRequest(pr),
		"replaced_by": replacedBy,
	})
}

func (h *Handler) GetUserReviews(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", "user_id is required")
		return
	}

	prs, err := h.service.ListUserReviews(r.Context(), userID)
	if err != nil {
		h.handleDomainError(w, err)
		return
	}

	result := make([]map[string]any, 0, len(prs))
	for _, pr := range prs {
		result = append(result, mapPullRequestShort(pr))
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"user_id":       userID,
		"pull_requests": result,
	})
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	if err := h.service.Health(r.Context()); err != nil {
		respondError(w, http.StatusInternalServerError, "UNHEALTHY", err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) handleDomainError(w http.ResponseWriter, err error) {
	switch err {
	case nil:
		return
	case domain.ErrTeamExists:
		respondError(w, http.StatusBadRequest, "TEAM_EXISTS", "team_name already exists")
	case domain.ErrPRExists:
		respondError(w, http.StatusConflict, "PR_EXISTS", "pull request already exists")
	case domain.ErrPRMerged:
		respondError(w, http.StatusConflict, "PR_MERGED", "cannot modify merged pull request")
	case domain.ErrReviewerNotFound:
		respondError(w, http.StatusConflict, "NOT_ASSIGNED", "reviewer is not assigned to this pull request")
	case domain.ErrNoReplacement:
		respondError(w, http.StatusConflict, "NO_CANDIDATE", "no active replacement candidate in team")
	case domain.ErrTeamNotFound, domain.ErrUserNotFound, domain.ErrPullRequestNotFound:
		respondError(w, http.StatusNotFound, "NOT_FOUND", "resource not found")
	default:
		respondError(w, http.StatusInternalServerError, "INTERNAL", "internal server error")
	}
}
