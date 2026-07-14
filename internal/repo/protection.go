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

// CheckResult describes the aggregate state of required status checks.
// It separates checks into three buckets:
//   - Missing: no workflow job was ever created for this check name
//   - Pending: a matching job exists but has not completed yet
//   - Failed:  a matching job completed with a conclusion other than "success"
type CheckResult struct {
	Missing []string `json:"missing"`
	Pending []string `json:"pending"`
	Failed  []string `json:"failed"`
}

// HasBlockers returns true when at least one required check is not passing.
func (r CheckResult) HasBlockers() bool {
	return len(r.Failed)+len(r.Pending)+len(r.Missing) > 0
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

// CheckRequiredChecks evaluates the required status checks for a given commit.
// It queries all workflow jobs associated with headSHA and classifies each
// required check as missing (no job exists), pending (job still running), or
// failed (job completed with a non-success conclusion). A check that has
// completed successfully is not included in any return bucket.
func (s *Service) CheckRequiredChecks(ctx context.Context, repoID uuid.UUID, headSHA string, checks []string) (*CheckResult, error) {
	res := &CheckResult{}
	if len(checks) == 0 {
		return res, nil
	}

	rows, err := s.pool.Query(ctx, `
		SELECT wj.job_id, wj.status, wj.conclusion
		FROM workflow_jobs wj
		JOIN workflow_runs wr ON wj.run_id = wr.id
		WHERE wr.repo_id=$1 AND wr.head_sha=$2`,
		repoID, headSHA)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// statusByCheck maps check names to their state:
	//   "passed"  – completed with conclusion="success"
	//   "failed"  – completed with conclusion != "success"
	//   "pending" – still running (not completed)
	statusByCheck := map[string]string{}
	for rows.Next() {
		var jobID, status string
		var conclusion *string
		if err := rows.Scan(&jobID, &status, &conclusion); err != nil {
			return nil, err
		}
		if status != "completed" {
			statusByCheck[jobID] = "pending"
		} else if conclusion != nil && *conclusion == "success" {
			statusByCheck[jobID] = "passed"
		} else {
			statusByCheck[jobID] = "failed"
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, check := range checks {
		switch statusByCheck[check] {
		case "passed":
			// all good
		case "failed":
			res.Failed = append(res.Failed, check)
		case "pending":
			res.Pending = append(res.Pending, check)
		default:
			res.Missing = append(res.Missing, check)
		}
	}
	return res, nil
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
	res, err := s.CheckRequiredChecks(ctx, repoID, headSHA, pb.RequiredChecks)
	if err != nil {
		return err
	}
	if !res.HasBlockers() {
		return nil
	}
	var parts []string
	if len(res.Failed) > 0 {
		parts = append(parts, fmt.Sprintf("failed checks: %v", res.Failed))
	}
	if len(res.Pending) > 0 {
		parts = append(parts, fmt.Sprintf("pending checks: %v", res.Pending))
	}
	if len(res.Missing) > 0 {
		parts = append(parts, fmt.Sprintf("missing checks: %v", res.Missing))
	}
	return fmt.Errorf("%w: %s", ErrProtectionViolation, strings.Join(parts, "; "))
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
