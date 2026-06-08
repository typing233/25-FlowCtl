import { get, post } from './client';
import type { PaginatedResponse } from './workflows';

export type ExecutionStatus =
  | 'pending'
  | 'running'
  | 'success'
  | 'failed'
  | 'cancelled'
  | 'waiting_approval';

export type StepStatus = 'pending' | 'running' | 'success' | 'failed' | 'skipped';

export interface Execution {
  id: string;
  workflowId: string;
  workflowName: string;
  version: number;
  status: ExecutionStatus;
  triggeredBy: string;
  startedAt: string;
  completedAt: string | null;
}

export interface StepExecution {
  id: string;
  name: string;
  status: StepStatus;
  startedAt: string | null;
  completedAt: string | null;
  error: string | null;
  dependencies: string[];
}

export interface ExecutionDetail extends Execution {
  inputs: Record<string, unknown>;
  outputs: Record<string, unknown> | null;
  steps: StepExecution[];
}

export interface LogEntry {
  timestamp: string;
  level: 'info' | 'warn' | 'error' | 'debug';
  message: string;
  stepId?: string;
}

export interface ListExecutionsParams {
  page?: number;
  pageSize?: number;
  workflowId?: string;
  status?: ExecutionStatus;
}

export function listExecutions(params?: ListExecutionsParams): Promise<PaginatedResponse<Execution>> {
  const query = new URLSearchParams();
  if (params?.page) query.set('page', String(params.page));
  if (params?.pageSize) query.set('pageSize', String(params.pageSize));
  if (params?.workflowId) query.set('workflowId', params.workflowId);
  if (params?.status) query.set('status', params.status);
  return get(`/executions?${query.toString()}`);
}

export function getExecution(id: string): Promise<ExecutionDetail> {
  return get(`/executions/${id}`);
}

export function cancelExecution(id: string): Promise<void> {
  return post(`/executions/${id}/cancel`);
}

export function retryExecution(id: string): Promise<{ executionId: string }> {
  return post(`/executions/${id}/retry`);
}

export function getExecutionLogs(id: string): Promise<LogEntry[]> {
  return get(`/executions/${id}/logs`);
}
