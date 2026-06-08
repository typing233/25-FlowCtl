import { get, post, put } from './client';

export interface Workflow {
  id: string;
  name: string;
  description: string;
  currentVersion: number;
  status: 'active' | 'inactive' | 'archived';
  createdAt: string;
  updatedAt: string;
}

export interface WorkflowVersion {
  version: number;
  source: string;
  publishedAt: string;
  publishedBy: string;
  changelog: string;
}

export interface WorkflowStep {
  id: string;
  name: string;
  type: string;
  dependencies: string[];
}

export interface WorkflowDetail extends Workflow {
  versions: WorkflowVersion[];
  steps: WorkflowStep[];
  source: string;
}

export interface ListWorkflowsParams {
  page?: number;
  pageSize?: number;
  search?: string;
  status?: string;
}

export interface PaginatedResponse<T> {
  items: T[];
  total: number;
  page: number;
  pageSize: number;
}

export function listWorkflows(params?: ListWorkflowsParams): Promise<PaginatedResponse<Workflow>> {
  const query = new URLSearchParams();
  if (params?.page) query.set('page', String(params.page));
  if (params?.pageSize) query.set('pageSize', String(params.pageSize));
  if (params?.search) query.set('search', params.search);
  if (params?.status) query.set('status', params.status);
  return get(`/workflows?${query.toString()}`);
}

export function getWorkflow(id: string): Promise<WorkflowDetail> {
  return get(`/workflows/${id}`);
}

export function createWorkflow(data: {
  name: string;
  description: string;
  source: string;
}): Promise<Workflow> {
  return post('/workflows', data);
}

export function publishVersion(
  id: string,
  data: { source: string; changelog: string },
): Promise<WorkflowVersion> {
  return post(`/workflows/${id}/versions`, data);
}

export function rollbackWorkflow(
  id: string,
  version: number,
): Promise<WorkflowDetail> {
  return put(`/workflows/${id}/rollback`, { version });
}

export function runWorkflow(
  id: string,
  inputs: Record<string, unknown>,
): Promise<{ executionId: string }> {
  return post(`/workflows/${id}/run`, { inputs });
}
