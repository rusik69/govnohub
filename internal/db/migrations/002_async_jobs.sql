ALTER TABLE workflow_jobs ADD COLUMN IF NOT EXISTS k8s_job_name TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS idx_workflow_jobs_run_job ON workflow_jobs(run_id, job_id);
