package domain

import "errors"

var (
	ErrTeamExists          = errors.New("team already exists")
	ErrPRExists            = errors.New("pull request already exists")
	ErrPRMerged            = errors.New("pull request already merged")
	ErrReviewerNotFound    = errors.New("reviewer is not assigned to this PR")
	ErrNoReplacement       = errors.New("no replacement candidate available")
	ErrTeamNotFound        = errors.New("team not found")
	ErrUserNotFound        = errors.New("user not found")
	ErrPullRequestNotFound = errors.New("pull request not found")
)
