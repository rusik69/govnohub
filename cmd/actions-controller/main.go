package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/rusik69/govnohub/internal/actions"
	"github.com/rusik69/govnohub/internal/config"
	"github.com/rusik69/govnohub/internal/db"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()
	pool, err := db.Connect(ctx, cfg.DatabaseURL, cfg.DatabaseMaxConns)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()

	k8s, err := newK8sClient()
	if err != nil {
		log.Fatalf("k8s: %v", err)
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	log.Printf("actions-controller started, runner ns=%s", cfg.RunnerNS)
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			if err := processQueuedRuns(ctx, pool, k8s, cfg); err != nil {
				log.Printf("dispatch error: %v", err)
			}
			if err := reconcileInProgressRuns(ctx, pool, k8s, cfg); err != nil {
				log.Printf("reconcile error: %v", err)
			}
		}
	}
}

func processQueuedRuns(ctx context.Context, pool *pgxpool.Pool, k8s *kubernetes.Clientset, cfg config.Config) error {
	for {
		runID, sha, branch, event, content, owner, repoName, err := claimQueuedRun(ctx, pool)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := startRun(ctx, pool, runID, content); err != nil {
			log.Printf("start run %s: %v", runID, err)
			continue
		}
		if err := dispatchReadyJobs(ctx, pool, k8s, cfg, runID, content, sha, branch, event, owner, repoName); err != nil {
			log.Printf("dispatch run %s: %v", runID, err)
		}
	}
}

func claimQueuedRun(ctx context.Context, pool *pgxpool.Pool) (uuid.UUID, string, string, string, string, string, string, error) {
	var runID uuid.UUID
	var sha, branch, event, content, owner, repoName string
	err := pool.QueryRow(ctx, `
		UPDATE workflow_runs wr SET status='in_progress', started_at=NOW()
		FROM workflows w, repos r
		LEFT JOIN users u ON r.owner_type='user' AND r.owner_id=u.id
		LEFT JOIN orgs o ON r.owner_type='org' AND r.owner_id=o.id
		WHERE wr.workflow_id = w.id AND wr.repo_id = r.id
		  AND wr.id = (
		    SELECT id FROM workflow_runs WHERE status='queued' ORDER BY created_at LIMIT 1 FOR UPDATE SKIP LOCKED
		  )
		  AND wr.status = 'queued'
		RETURNING wr.id, wr.head_sha, wr.head_branch, wr.event, w.content, COALESCE(u.username, o.name), r.name`,
	).Scan(&runID, &sha, &branch, &event, &content, &owner, &repoName)
	return runID, sha, branch, event, content, owner, repoName, err
}

func startRun(ctx context.Context, pool *pgxpool.Pool, runID uuid.UUID, content string) error {
	wf, err := actions.ParseWorkflow(content)
	if err != nil {
		_, err := pool.Exec(ctx, `UPDATE workflow_runs SET status='completed', conclusion='failure', completed_at=NOW() WHERE id=$1`, runID)
		return err
	}
	jobOrder, err := wf.JobOrder()
	if err != nil {
		_, err := pool.Exec(ctx, `UPDATE workflow_runs SET status='completed', conclusion='failure', completed_at=NOW() WHERE id=$1`, runID)
		return err
	}
	for _, jobID := range jobOrder {
		_, err := pool.Exec(ctx, `
			INSERT INTO workflow_jobs (run_id, job_id, name, status)
			VALUES ($1,$2,$3,'queued')
			ON CONFLICT (run_id, job_id) DO NOTHING`, runID, jobID, jobID)
		if err != nil {
			return err
		}
	}
	return nil
}

func dispatchReadyJobs(ctx context.Context, pool *pgxpool.Pool, k8s *kubernetes.Clientset, cfg config.Config, runID uuid.UUID, content, sha, branch, event, owner, repoName string) error {
	wf, err := actions.ParseWorkflow(content)
	if err != nil {
		return err
	}
	completed, err := completedJobs(ctx, pool, runID)
	if err != nil {
		return err
	}
	rows, err := pool.Query(ctx, `
		SELECT job_id FROM workflow_jobs WHERE run_id=$1 AND status='queued'`, runID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var jobID string
		if err := rows.Scan(&jobID); err != nil {
			return err
		}
		job, ok := wf.Jobs[jobID]
		if !ok {
			continue
		}
		if !needsSatisfied(job.Needs, completed) {
			continue
		}
		if err := dispatchJob(ctx, pool, k8s, cfg, runID, jobID, job, sha, branch, event, owner, repoName); err != nil {
			log.Printf("dispatch job %s: %v", jobID, err)
		}
	}
	return rows.Err()
}

func needsSatisfied(needs interface{}, completed map[string]string) bool {
	for _, dep := range actions.NormalizeNeeds(needs) {
		if completed[dep] != "success" {
			return false
		}
	}
	return true
}

func completedJobs(ctx context.Context, pool *pgxpool.Pool, runID uuid.UUID) (map[string]string, error) {
	rows, err := pool.Query(ctx, `
		SELECT job_id, conclusion FROM workflow_jobs
		WHERE run_id=$1 AND status='completed'`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var jobID string
		var conclusion *string
		if err := rows.Scan(&jobID, &conclusion); err != nil {
			return nil, err
		}
		if conclusion != nil {
			out[jobID] = *conclusion
		}
	}
	return out, rows.Err()
}

func dispatchJob(ctx context.Context, pool *pgxpool.Pool, k8s *kubernetes.Clientset, cfg config.Config, runID uuid.UUID, jobID string, job actions.Job, sha, branch, event, owner, repoName string) error {
	ctxMap := actions.GitHubContext(owner, repoName, sha, branch, event, runID.String(), cfg.ArtifactRoot)
	script := actions.WriteJobScript(job.Steps, ctxMap)
	logDir := filepath.Join(cfg.ArtifactRoot, "logs", runID.String())
	os.MkdirAll(logDir, 0o755)
	logPath := filepath.Join(logDir, jobID+".log")
	jobName := fmt.Sprintf("run-%s-%s", runID.String()[:8], sanitize(jobID))

	_, err := k8s.BatchV1().Jobs(cfg.RunnerNS).Create(ctx, buildJob(cfg, jobName, script, logPath), metav1.CreateOptions{})
	if err != nil {
		_, execErr := pool.Exec(ctx, `
			UPDATE workflow_jobs SET status='completed', conclusion='failure', log_path=$3, completed_at=NOW()
			WHERE run_id=$1 AND job_id=$2`, runID, jobID, logPath)
		return execErr
	}
	_, err = pool.Exec(ctx, `
		UPDATE workflow_jobs SET status='in_progress', k8s_job_name=$3, log_path=$4, started_at=NOW()
		WHERE run_id=$1 AND job_id=$2 AND status='queued'`, runID, jobID, jobName, logPath)
	return err
}

func reconcileInProgressRuns(ctx context.Context, pool *pgxpool.Pool, k8s *kubernetes.Clientset, cfg config.Config) error {
	rows, err := pool.Query(ctx, `SELECT id FROM workflow_runs WHERE status='in_progress'`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var runID uuid.UUID
		if err := rows.Scan(&runID); err != nil {
			return err
		}
		if err := reconcileRun(ctx, pool, k8s, cfg, runID); err != nil {
			log.Printf("reconcile run %s: %v", runID, err)
		}
	}
	return rows.Err()
}

func reconcileRun(ctx context.Context, pool *pgxpool.Pool, k8s *kubernetes.Clientset, cfg config.Config, runID uuid.UUID) error {
	var content, sha, branch, event, owner, repoName string
	err := pool.QueryRow(ctx, `
		SELECT w.content, wr.head_sha, wr.head_branch, wr.event, COALESCE(u.username, o.name), r.name
		FROM workflow_runs wr
		JOIN workflows w ON wr.workflow_id = w.id
		JOIN repos r ON wr.repo_id = r.id
		LEFT JOIN users u ON r.owner_type='user' AND r.owner_id=u.id
		LEFT JOIN orgs o ON r.owner_type='org' AND r.owner_id=o.id
		WHERE wr.id=$1`, runID).Scan(&content, &sha, &branch, &event, &owner, &repoName)
	if err != nil {
		return err
	}

	jobRows, err := pool.Query(ctx, `
		SELECT job_id, status, k8s_job_name FROM workflow_jobs WHERE run_id=$1`, runID)
	if err != nil {
		return err
	}
	type jobState struct {
		status     string
		k8sJobName *string
	}
	states := map[string]jobState{}
	for jobRows.Next() {
		var jobID, status string
		var k8sName *string
		if err := jobRows.Scan(&jobID, &status, &k8sName); err != nil {
			jobRows.Close()
			return err
		}
		states[jobID] = jobState{status: status, k8sJobName: k8sName}
	}
	jobRows.Close()

	for jobID, st := range states {
		if st.status != "in_progress" || st.k8sJobName == nil {
			continue
		}
		conclusion := pollJob(ctx, k8s, cfg.RunnerNS, *st.k8sJobName)
		if conclusion == "" {
			continue
		}
		_, err := pool.Exec(ctx, `
			UPDATE workflow_jobs SET status='completed', conclusion=$3, completed_at=NOW()
			WHERE run_id=$1 AND job_id=$2`, runID, jobID, conclusion)
		if err != nil {
			return err
		}
	}

	if err := dispatchReadyJobs(ctx, pool, k8s, cfg, runID, content, sha, branch, event, owner, repoName); err != nil {
		return err
	}

	var total, done int
	var failed bool
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*), COUNT(*) FILTER (WHERE status='completed'),
		       BOOL_OR(conclusion='failure')
		FROM workflow_jobs WHERE run_id=$1`, runID).Scan(&total, &done, &failed)
	if err != nil {
		return err
	}
	if total == 0 || done < total {
		return nil
	}
	conclusion := "success"
	if failed {
		conclusion = "failure"
	}
	_, err = pool.Exec(ctx, `
		UPDATE workflow_runs SET status='completed', conclusion=$2, completed_at=NOW()
		WHERE id=$1 AND status='in_progress'`, runID, conclusion)
	return err
}

func pollJob(ctx context.Context, k8s *kubernetes.Clientset, ns, name string) string {
	job, err := k8s.BatchV1().Jobs(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return ""
	}
	if job.Status.Succeeded > 0 {
		return "success"
	}
	if job.Status.Failed > 0 {
		return "failure"
	}
	return ""
}

func buildJob(cfg config.Config, name, script, logPath string) *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: cfg.RunnerNS},
		Spec: batchv1.JobSpec{
			BackoffLimit: int32Ptr(0),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{{
						Name:    "runner",
						Image:   envOr("RUNNER_IMAGE", "alpine:3.20"),
						Command: []string{"/bin/sh", "-c"},
						Args:    []string{script + " 2>&1 | tee " + logPath},
						VolumeMounts: []corev1.VolumeMount{
							{Name: "workspace", MountPath: "/github/workspace"},
							{Name: "artifacts", MountPath: filepath.Dir(logPath)},
						},
					}},
					Volumes: []corev1.Volume{
						{Name: "workspace", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
						{Name: "artifacts", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: cfg.ArtifactRoot}}},
					},
				},
			},
		},
	}
}

func newK8sClient() (*kubernetes.Clientset, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, err
		}
	}
	return kubernetes.NewForConfig(config)
}

func sanitize(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			out = append(out, c)
		} else {
			out = append(out, '-')
		}
	}
	return string(out)
}

func int32Ptr(v int32) *int32 { return &v }
func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
