package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"Avito2025/internal/config"
	"Avito2025/internal/domain"
	"Avito2025/internal/storage"
	"Avito2025/internal/storage/postgres/migrations"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ storage.Repository = (*Store)(nil)

type Store struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, cfg config.PostgresConfig) (*Store, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("parse postgres dsn: %w", err)
	}
	if cfg.MaxConns > 0 {
		poolCfg.MaxConns = cfg.MaxConns
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}

	store := &Store{pool: pool}
	if err := store.applyMigrations(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return store, nil
}

func (s *Store) Close() {
	s.pool.Close()
}

func (s *Store) applyMigrations(ctx context.Context) error {
	entries, err := migrations.Files.ReadDir(".")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		sqlBytes, err := fs.ReadFile(migrations.Files, entry.Name())
		if err != nil {
			return fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}

		if _, err := s.pool.Exec(ctx, string(sqlBytes)); err != nil {
			return fmt.Errorf("apply migration %s: %w", entry.Name(), err)
		}
	}
	return nil
}

func (s *Store) CreateTeam(ctx context.Context, team domain.Team) (domain.Team, error) {
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		var name string
		err := tx.QueryRow(ctx, `SELECT name FROM teams WHERE name = $1`, team.Name).Scan(&name)
		if err == nil {
			return domain.ErrTeamExists
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		if _, err := tx.Exec(ctx, `INSERT INTO teams (name) VALUES ($1)`, team.Name); err != nil {
			return err
		}

		for _, member := range team.Members {
			if _, err := tx.Exec(ctx, `
				INSERT INTO users (user_id, username, team_name, is_active)
				VALUES ($1, $2, $3, $4)
				ON CONFLICT (user_id) DO UPDATE
				SET username = EXCLUDED.username,
				    team_name = EXCLUDED.team_name,
				    is_active = EXCLUDED.is_active,
				    updated_at = NOW()
			`, member.ID, member.Username, team.Name, member.IsActive); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return domain.Team{}, translateError(err)
	}

	return s.GetTeam(ctx, team.Name)
}

func (s *Store) GetTeam(ctx context.Context, name string) (domain.Team, error) {
	var teamName string
	err := s.pool.QueryRow(ctx, `SELECT name FROM teams WHERE name = $1`, name).Scan(&teamName)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Team{}, domain.ErrTeamNotFound
		}
		return domain.Team{}, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT user_id, username, is_active
		FROM users
		WHERE team_name = $1
		ORDER BY user_id`, name)
	if err != nil {
		return domain.Team{}, err
	}
	defer rows.Close()

	var members []domain.User
	for rows.Next() {
		var u domain.User
		u.TeamName = name
		if err := rows.Scan(&u.ID, &u.Username, &u.IsActive); err != nil {
			return domain.Team{}, err
		}
		members = append(members, u)
	}
	if rows.Err() != nil {
		return domain.Team{}, rows.Err()
	}

	return domain.Team{
		Name:    teamName,
		Members: members,
	}, nil
}

func (s *Store) GetUser(ctx context.Context, userID string) (domain.User, error) {
	var user domain.User
	err := s.pool.QueryRow(ctx, `
		SELECT user_id, username, team_name, is_active
		FROM users
		WHERE user_id = $1`, userID).Scan(&user.ID, &user.Username, &user.TeamName, &user.IsActive)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.User{}, domain.ErrUserNotFound
		}
		return domain.User{}, err
	}
	return user, nil
}

func (s *Store) SetUserActive(ctx context.Context, userID string, isActive bool) (domain.User, error) {
	var user domain.User
	err := s.pool.QueryRow(ctx, `
		UPDATE users
		SET is_active = $2,
		    updated_at = NOW()
		WHERE user_id = $1
		RETURNING user_id, username, team_name, is_active
	`, userID, isActive).Scan(&user.ID, &user.Username, &user.TeamName, &user.IsActive)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.User{}, domain.ErrUserNotFound
		}
		return domain.User{}, err
	}
	return user, nil
}

func (s *Store) ListUsersByTeam(ctx context.Context, teamName string) ([]domain.User, error) {
	var name string
	if err := s.pool.QueryRow(ctx, `SELECT name FROM teams WHERE name = $1`, teamName).Scan(&name); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrTeamNotFound
		}
		return nil, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT user_id, username, team_name, is_active
		FROM users
		WHERE team_name = $1`, teamName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []domain.User
	for rows.Next() {
		var user domain.User
		if err := rows.Scan(&user.ID, &user.Username, &user.TeamName, &user.IsActive); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return users, nil
}

func (s *Store) CreatePullRequest(ctx context.Context, pr domain.PullRequest) (domain.PullRequest, error) {
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO pull_requests (pull_request_id, pull_request_name, author_id, status, created_at, merged_at)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, pr.ID, pr.Name, pr.AuthorID, string(pr.Status), pr.CreatedAt, pr.MergedAt)
		if err != nil {
			return err
		}

		for _, reviewer := range pr.AssignedReviewers {
			if _, err := tx.Exec(ctx, `
				INSERT INTO pull_request_reviewers (pull_request_id, reviewer_id)
				VALUES ($1, $2)
			`, pr.ID, reviewer); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return domain.PullRequest{}, translateError(err)
	}

	return s.GetPullRequest(ctx, pr.ID)
}

func (s *Store) UpdatePullRequest(ctx context.Context, pr domain.PullRequest) (domain.PullRequest, error) {
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		commandTag, err := tx.Exec(ctx, `
			UPDATE pull_requests
			SET pull_request_name = $2,
			    author_id = $3,
			    status = $4,
			    created_at = $5,
			    merged_at = $6
			WHERE pull_request_id = $1
		`, pr.ID, pr.Name, pr.AuthorID, string(pr.Status), pr.CreatedAt, pr.MergedAt)
		if err != nil {
			return err
		}
		if commandTag.RowsAffected() == 0 {
			return domain.ErrPullRequestNotFound
		}

		if _, err := tx.Exec(ctx, `DELETE FROM pull_request_reviewers WHERE pull_request_id = $1`, pr.ID); err != nil {
			return err
		}
		for _, reviewer := range pr.AssignedReviewers {
			if _, err := tx.Exec(ctx, `
				INSERT INTO pull_request_reviewers (pull_request_id, reviewer_id)
				VALUES ($1, $2)
			`, pr.ID, reviewer); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return domain.PullRequest{}, translateError(err)
	}

	return s.GetPullRequest(ctx, pr.ID)
}

func (s *Store) GetPullRequest(ctx context.Context, id string) (domain.PullRequest, error) {
	var pr domain.PullRequest
	var mergedAt sql.NullTime
	err := s.pool.QueryRow(ctx, `
		SELECT pull_request_id, pull_request_name, author_id, status, created_at, merged_at
		FROM pull_requests
		WHERE pull_request_id = $1
	`, id).Scan(&pr.ID, &pr.Name, &pr.AuthorID, &pr.Status, &pr.CreatedAt, &mergedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.PullRequest{}, domain.ErrPullRequestNotFound
		}
		return domain.PullRequest{}, err
	}
	if mergedAt.Valid {
		pr.MergedAt = &mergedAt.Time
	}

	rows, err := s.pool.Query(ctx, `
		SELECT reviewer_id
		FROM pull_request_reviewers
		WHERE pull_request_id = $1
		ORDER BY reviewer_id
	`, id)
	if err != nil {
		return domain.PullRequest{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var reviewer string
		if err := rows.Scan(&reviewer); err != nil {
			return domain.PullRequest{}, err
		}
		pr.AssignedReviewers = append(pr.AssignedReviewers, reviewer)
	}
	if rows.Err() != nil {
		return domain.PullRequest{}, rows.Err()
	}

	return pr, nil
}

func (s *Store) ListPullRequestsByReviewer(ctx context.Context, userID string) ([]domain.PullRequest, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT pr.pull_request_id, pr.pull_request_name, pr.author_id, pr.status, pr.created_at, pr.merged_at
		FROM pull_requests pr
		JOIN pull_request_reviewers r ON r.pull_request_id = pr.pull_request_id
		WHERE r.reviewer_id = $1
		ORDER BY pr.created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []domain.PullRequest
	for rows.Next() {
		var pr domain.PullRequest
		var mergedAt sql.NullTime
		if err := rows.Scan(&pr.ID, &pr.Name, &pr.AuthorID, &pr.Status, &pr.CreatedAt, &mergedAt); err != nil {
			return nil, err
		}
		if mergedAt.Valid {
			pr.MergedAt = &mergedAt.Time
		}
		result = append(result, pr)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return result, nil
}

func (s *Store) Health(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

func (s *Store) withTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if err := fn(tx); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func translateError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == "23505" {
			switch {
			case pgErr.ConstraintName == "teams_pkey":
				return domain.ErrTeamExists
			case pgErr.ConstraintName == "pull_requests_pkey":
				return domain.ErrPRExists
			}
		}
	}
	return err
}
