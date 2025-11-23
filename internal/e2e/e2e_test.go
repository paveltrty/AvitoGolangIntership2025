package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"Avito2025/internal/config"
	"Avito2025/internal/domain"
	"Avito2025/internal/service"
	"Avito2025/internal/storage/postgres"
	httptransport "Avito2025/internal/transport/http"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestE2EFlow(t *testing.T) {
	t.Run("team lifecycle", func(t *testing.T) {
		server := newTestServer(t)
		defer server.Close()

		client := server.Client()

		createTeam(t, client, server.URL)
		assertGetTeam(t, client, server.URL)
	})

	t.Run("pull request flow", func(t *testing.T) {
		server := newTestServer(t)
		defer server.Close()

		client := server.Client()

		createTeam(t, client, server.URL)

		pr := createPR(t, client, server.URL, "pr-100", "Add login", "u1")
		if len(pr.AssignedReviewers) == 0 {
			t.Fatalf("expected reviewers to be assigned")
		}

		oldReviewer := pr.AssignedReviewers[0]
		reassignResp := reassign(t, client, server.URL, pr.ID, oldReviewer)
		if reassignResp.ReplacedBy == oldReviewer {
			t.Fatalf("reviewer should be replaced")
		}

		merged := merge(t, client, server.URL, pr.ID)
		if merged.Status != string(domain.StatusMerged) {
			t.Fatalf("expected status MERGED, got %s", merged.Status)
		}

		assertUserReviews(t, client, server.URL, reassignResp.ReplacedBy)
	})

	t.Run("health", func(t *testing.T) {
		server := newTestServer(t)
		defer server.Close()

		client := server.Client()

		resp, err := client.Get(server.URL + "/health")
		if err != nil {
			t.Fatalf("health request: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("health status: %d", resp.StatusCode)
		}
	})
}

// Helpers

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	ctx := context.Background()

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

	svc := service.New(store)
	handler := httptransport.NewHandler(svc)

	return httptest.NewServer(handler.Router())
}

type teamPayload struct {
	TeamName string `json:"team_name"`
	Members  []struct {
		UserID   string `json:"user_id"`
		Username string `json:"username"`
		IsActive bool   `json:"is_active"`
	} `json:"members"`
}

func createTeam(t *testing.T, client *http.Client, baseURL string) {
	t.Helper()

	body := map[string]any{
		"team_name": "backend",
		"members": []map[string]any{
			{"user_id": "u1", "username": "Alice", "is_active": true},
			{"user_id": "u2", "username": "Bob", "is_active": true},
			{"user_id": "u3", "username": "Cathy", "is_active": true},
			{"user_id": "u4", "username": "Dan", "is_active": true},
		},
	}

	resp := doRequest(t, client, http.MethodPost, baseURL+"/team/add", body)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create team status: %d", resp.StatusCode)
	}
}

func assertGetTeam(t *testing.T, client *http.Client, baseURL string) {
	t.Helper()

	resp, err := client.Get(baseURL + "/team/get?team_name=backend")
	if err != nil {
		t.Fatalf("get team: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get team status: %d", resp.StatusCode)
	}

	var team teamPayload
	if err := json.NewDecoder(resp.Body).Decode(&team); err != nil {
		t.Fatalf("decode team: %v", err)
	}

	if len(team.Members) != 4 {
		t.Fatalf("expected 4 members, got %d", len(team.Members))
	}
}

type pullRequestPayload struct {
	ID                string   `json:"pull_request_id"`
	Name              string   `json:"pull_request_name"`
	AuthorID          string   `json:"author_id"`
	Status            string   `json:"status"`
	AssignedReviewers []string `json:"assigned_reviewers"`
}

type prResponse struct {
	PR pullRequestPayload `json:"pr"`
}

func createPR(t *testing.T, client *http.Client, baseURL, id, name, author string) pullRequestPayload {
	t.Helper()

	payload := map[string]string{
		"pull_request_id":   id,
		"pull_request_name": name,
		"author_id":         author,
	}

	resp := doRequest(t, client, http.MethodPost, baseURL+"/pullRequest/create", payload)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create pr status: %d", resp.StatusCode)
	}

	var response prResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("decode pr: %v", err)
	}
	return response.PR
}

type reassignResponse struct {
	PR         pullRequestPayload `json:"pr"`
	ReplacedBy string             `json:"replaced_by"`
}

func reassign(t *testing.T, client *http.Client, baseURL, prID, oldReviewer string) reassignResponse {
	t.Helper()

	payload := map[string]string{
		"pull_request_id": prID,
		"old_user_id":     oldReviewer,
	}

	resp := doRequest(t, client, http.MethodPost, baseURL+"/pullRequest/reassign", payload)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reassign status: %d", resp.StatusCode)
	}

	var response reassignResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("decode reassign: %v", err)
	}

	if len(response.PR.AssignedReviewers) == 0 {
		t.Fatalf("reassign response missing reviewers")
	}
	return response
}

func merge(t *testing.T, client *http.Client, baseURL, prID string) pullRequestPayload {
	t.Helper()

	payload := map[string]string{"pull_request_id": prID}
	resp := doRequest(t, client, http.MethodPost, baseURL+"/pullRequest/merge", payload)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("merge status: %d", resp.StatusCode)
	}

	var response prResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("decode merge: %v", err)
	}
	return response.PR
}

func assertUserReviews(t *testing.T, client *http.Client, baseURL, reviewer string) {
	t.Helper()

	resp, err := client.Get(baseURL + "/users/getReview?user_id=" + reviewer)
	if err != nil {
		t.Fatalf("get reviews: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get reviews status: %d", resp.StatusCode)
	}

	var payload struct {
		UserID       string               `json:"user_id"`
		PullRequests []pullRequestPayload `json:"pull_requests"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode reviews: %v", err)
	}

	if payload.UserID != reviewer {
		t.Fatalf("unexpected user_id: %s", payload.UserID)
	}
}

func doRequest(t *testing.T, client *http.Client, method, url string, payload any) *http.Response {
	t.Helper()

	var body bytes.Buffer
	if payload != nil {
		if err := json.NewEncoder(&body).Encode(payload); err != nil {
			t.Fatalf("encode payload: %v", err)
		}
	}

	req, err := http.NewRequestWithContext(context.Background(), method, url, &body)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}

	return resp
}
