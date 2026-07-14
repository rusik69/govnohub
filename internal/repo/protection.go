package repo

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	gitstore "github.com/rusik69/govnohub/internal/git"
)

var ErrProtectionViolation = errors.New("branch protection requirements not met")

type ProtectedBranch struct {
	BranchName     string   `json:"branch_name"`
	RequiredChecks []string `json:"required_checks"`
	RequireReviews int      `json:"require_reviews"`
}

func (s *Service) GetProtectedBranch(ctx context.Context, repoID uuid.UUID, branch string) (*ProtectedBranch, error) {
	var pb ProtectedBranch
	err := s.pool.QueryRow(ctx, `
		SELECT branch_name, required_checks, require_reviews
		FROM protected_branches WHERE repo_id=$1 AND branch_name=$2`,
		repoID, branch,
	).Scan(&pb.BranchName, &pb.RequiredChecks, &pb.RequireReviews)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &pb, nil
}

func (s *Service) ListProtectedBranches(ctx context.Context, repoID uuid.UUID) ([]ProtectedBranch, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT branch_name, required_checks, require_reviews
		FROM protected_branches WHERE repo_id=$1 ORDER BY branch_name`, repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ProtectedBranch
	for rows.Next() {
		var pb ProtectedBranch
		if err := rows.Scan(&pb.BranchName, &pb.RequiredChecks, &pb.RequireReviews); err != nil {
			return nil, err
		}
		out = append(out, pb)
	}
	return out, rows.Err()
}

func (s *Service) CheckRequiredChecks(ctx context.Context, repoID uuid.UUID, headSHA string, checks []string) ([]string, error) {
	if len(checks) == 0 {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT wj.job_id, wj.conclusion
		FROM workflow_jobs wj
		JOIN workflow_runs wr ON wj.run_id = wr.id
		WHERE wr.repo_id=$1 AND wr.head_sha=$2 AND wj.status='completed'`,
		repoID, headSHA)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	passed := map[string]bool{}
	for rows.Next() {
		var jobID string
		var conclusion *string
		if err := rows.Scan(&jobID, &conclusion); err != nil {
			return nil, err
		}
		if conclusion != nil && *conclusion == "success" {
			passed[jobID] = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var missing []string
	for _, check := range checks {
		if !passed[check] {
			missing = append(missing, check)
		}
	}
	return missing, nil
}

func (s *Service) ValidateMergeProtection(ctx context.Context, repoID uuid.UUID, baseBranch, headSHA string, approvedReviews int) error {
	pb, err := s.GetProtectedBranch(ctx, repoID, baseBranch)
	if err != nil {
		return err
	}
	if pb == nil {
		return nil
	}
	if pb.RequireReviews > 0 && approvedReviews < pb.RequireReviews {
		return fmt.Errorf("%w: need %d approvals, have %d", ErrProtectionViolation, pb.RequireReviews, approvedReviews)
	}
	missing, err := s.CheckRequiredChecks(ctx, repoID, headSHA, pb.RequiredChecks)
	if err != nil {
		return err
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: missing or failed checks: %v", ErrProtectionViolation, missing)
	}
	return nil
}

// CheckPushProtection checks if pushing to the given git refs (from a receive-pack)
// would violate any branch protection rules. It returns an error listing all violations.
// A "deletion" push (newSHA == all-zeros) to a protected branch is also blocked.
func (s *Service) CheckPushProtection(ctx context.Context, repoID uuid.UUID, refs []gitstore.RefUpdate) error {
	var errs []string
	for _, ref := range refs {
		if !strings.HasPrefix(ref.Ref, "refs/heads/") {
			continue // only check branch pushes
		}
		branch := strings.TrimPrefix(ref.Ref, "refs/heads/")
		// Deletion (all-zero new SHA) is checked against protection too
		pb, err := s.GetProtectedBranch(ctx, repoID, branch)
		if err != nil {
			return err
		}
		if pb != nil {
			errs = append(errs, fmt.Sprintf("cannot push to protected branch %q", branch))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%w: %s", ErrProtectionViolation, strings.Join(errs, "; "))
	}
	return nil
}
