package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/google/uuid"
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
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
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
				log.Printf("reconcile error: %v", err)
			}
		}
	}
}

func processQueuedRuns(ctx context.Context, pool *pgxpool.Pool, k8s *kubernetes.Clientset, cfg config.Config) error {
	rs, err := pool.Query(ctx, `
		SELECT wr.id, wr.head_sha, wr.head_branch, wr.event,
		       w.content, COALESCE(u.username, o.name) AS owner, r.name AS repo_name
		FROM workflow_runs wr
		JOIN workflows w ON w.id = wr.workflow_id
		JOIN repos r ON r.id = wr.repo_id
		LEFT JOIN users u ON r.owner_type='user' AND r.owner_id=u.id
		LEFT JOIN orgs o ON r.owner_type='org' AND r.owner_id=o.id
		WHERE wr.status = 'queued'
		ORDER BY wr.created_at LIMIT 5`)
	if err != nil {
		return err
	}
	defer rs.Close()

	for rs.Next() {
		var runID uuid.UUID
		var sha, branch, event, content, owner, repoName string
		if err := rs.Scan(&runID, &sha, &branch, &event, &content, &owner, &repoName); err != nil {
			continue
		}
		wf, err := actions.ParseWorkflow(content)
		if err != nil {
			pool.Exec(ctx, `UPDATE workflow_runs SET status='completed', conclusion='failure', completed_at=NOW() WHERE id=$1`, runID)
			continue
		}
		pool.Exec(ctx, `UPDATE workflow_runs SET status='in_progress', started_at=NOW() WHERE id=$1`, runID)

		jobOrder, err := wf.JobOrder()
		if err != nil {
			pool.Exec(ctx, `UPDATE workflow_runs SET status='completed', conclusion='failure', completed_at=NOW() WHERE id=$1`, runID)
			continue
		}

		allSuccess := true
		for _, jobID := range jobOrder {
			job := wf.Jobs[jobID]
			ctxMap := actions.GitHubContext(owner, repoName, sha, branch, event)
			script := actions.WriteJobScript(job.Steps, ctxMap)
			logDir := filepath.Join(cfg.ArtifactRoot, "logs", runID.String())
			os.MkdirAll(logDir, 0o755)
			logPath := filepath.Join(logDir, jobID+".log")

			jobName := fmt.Sprintf("run-%s-%s", runID.String()[:8], sanitize(jobID))
			_, err := k8s.BatchV1().Jobs(cfg.RunnerNS).Create(ctx, buildJob(cfg, jobName, script, logPath), metav1.CreateOptions{})
			if err != nil {
				log.Printf("create job %s: %v", jobName, err)
				allSuccess = false
				pool.Exec(ctx, `
					INSERT INTO workflow_jobs (run_id, job_id, name, status, conclusion, log_path)
					VALUES ($1,$2,$3,'completed','failure',$4)`, runID, jobID, jobID, logPath)
				continue
			}

			pool.Exec(ctx, `
				INSERT INTO workflow_jobs (run_id, job_id, name, status, log_path)
				VALUES ($1,$2,$3,'in_progress',$4)`, runID, jobID, jobID, logPath)

			conclusion := waitForJob(ctx, k8s, cfg.RunnerNS, jobName)
			if conclusion != "success" {
				allSuccess = false
			}
			pool.Exec(ctx, `UPDATE workflow_jobs SET status='completed', conclusion=$3, completed_at=NOW() WHERE run_id=$1 AND job_id=$2`,
				runID, jobID, conclusion)
		}

		conclusion := "success"
		if !allSuccess {
			conclusion = "failure"
		}
		pool.Exec(ctx, `UPDATE workflow_runs SET status='completed', conclusion=$2, completed_at=NOW() WHERE id=$1`, runID, conclusion)
	}
	return rs.Err()
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

func waitForJob(ctx context.Context, k8s *kubernetes.Clientset, ns, name string) string {
	for i := 0; i < 60; i++ {
		job, err := k8s.BatchV1().Jobs(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}
		if job.Status.Succeeded > 0 {
			return "success"
		}
		if job.Status.Failed > 0 {
			return "failure"
		}
		time.Sleep(2 * time.Second)
	}
	return "failure"
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
