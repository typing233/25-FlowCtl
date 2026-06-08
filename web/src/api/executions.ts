import { get, post } from './client';
import type { PaginatedResponse } from './workflows';

export type ExecutionStatus =
  | 'pending'
  | 'queued'
  | 'running'
  | 'waiting_approval'
  | 'paused'
  | 'succeeded'
  | 'failed'
  | 'cancelled'
  | 'retrying';

export type StepStatus =
  | 'pending'
  | 'queued'
  | 'running'
  | 'waiting_approval'
  | 'succeeded'
  | 'failed'
  | 'skipped'
  | 'cancelled'
  | 'retrying';

export interface Execution {
  id: string;
  tenant_id: string;
  workflow_id: string;
  workflow_version_id: string;
  status: ExecutionStatus;
  inputs: Record<string, unknown>;
  outputs: Record<string, unknown> | null;
  trigger_type: 'manual' | 'cron' | 'api' | 'webhook';
  triggered_by: string | null;
  started_at: string | null;
  finished_at: string | null;
  created_at: string;
}

export interface ExecutionStep {
  id: string;
  execution_id: string;
  step_id: string;
  status: StepStatus;
  runner_type: string;
  config: Record<string, unknown>;
  inputs: Record<string, unknown>;
  outputs: Record<string, unknown>;
  error: string | null;
  attempt: number;
  max_retries: number;
  timeout_seconds: number;
  started_at: string | null;
  finished_at: string | null;
}

export interface LogEntry {
  id: number;
  execution_id: string;
  step_id: string;
  stream: 'stdout' | 'stderr' | 'system';
  line: string;
  timestamp: string;
}

export interface ListExecutionsParams {
  page?: number;
  limit?: number;
  workflow_id?: string;
  status?: ExecutionStatus;
}

export function listExecutions(params?: ListExecutionsParams): Promise<PaginatedResponse<Execution>> {
  const query = new URLSearchParams();
  if (params?.page) query.set('page', String(params.page));
  if (params?.limit) query.set('limit', String(params.limit));
  if (params?.workflow_id) query.set('workflow_id', params.workflow_id);
  if (params?.status) query.set('status', params.status);
  const qs = query.toString();
  return get(`/api/v1/executions${qs ? '?' + qs : ''}`);
}

export function getExecution(id: string): Promise<Execution> {
  return get(`/api/v1/executions/${id}`);
}

export function getExecutionSteps(id: string): Promise<ExecutionStep[]> {
  return get(`/api/v1/executions/${id}/steps`);
}

export function getExecutionLogs(id: string, stepId?: string): Promise<LogEntry[]> {
  const query = stepId ? `?step_id=${stepId}` : '';
  return get(`/api/v1/executions/${id}/logs${query}`);
}

export function cancelExecution(id: string): Promise<void> {
  return post(`/api/v1/executions/${id}/cancel`);
}

export function retryExecution(id: string): Promise<Execution> {
  return post(`/api/v1/executions/${id}/retry`);
}
