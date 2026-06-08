import { get, post, put, del } from './client';

export interface Workflow {
  id: string;
  tenant_id: string;
  slug: string;
  name: string;
  description: string;
  active_version_id: string | null;
  created_at: string;
  updated_at: string;
}

export interface WorkflowVersion {
  id: string;
  workflow_id: string;
  version: number;
  schema_version: string;
  source_format: 'yaml' | 'huml';
  definition: Record<string, unknown>;
  published_at: string | null;
  published_by: string | null;
  created_at: string;
}

export interface WorkflowDetail extends Workflow {
  versions?: WorkflowVersion[];
}

export interface PaginatedResponse<T> {
  data: T[];
  total: number;
  page: number;
  limit: number;
}

export interface ListWorkflowsParams {
  page?: number;
  limit?: number;
  search?: string;
}

export function listWorkflows(params?: ListWorkflowsParams): Promise<PaginatedResponse<Workflow>> {
  const query = new URLSearchParams();
  if (params?.page) query.set('page', String(params.page));
  if (params?.limit) query.set('limit', String(params.limit));
  if (params?.search) query.set('search', params.search);
  const qs = query.toString();
  return get(`/api/v1/workflows${qs ? '?' + qs : ''}`);
}

export function getWorkflow(id: string): Promise<Workflow> {
  return get(`/api/v1/workflows/${id}`);
}

export function createWorkflow(data: {
  slug: string;
  name: string;
  description?: string;
}): Promise<Workflow> {
  return post('/api/v1/workflows', data);
}

export function publishVersion(
  workflowId: string,
  data: { source: string; format: 'yaml' | 'huml' },
): Promise<WorkflowVersion> {
  return post(`/api/v1/workflows/${workflowId}/versions`, data);
}

export function rollbackWorkflow(
  workflowId: string,
  version: number,
): Promise<void> {
  return post(`/api/v1/workflows/${workflowId}/rollback`, { version });
}

export function listVersions(workflowId: string): Promise<WorkflowVersion[]> {
  return get(`/api/v1/workflows/${workflowId}/versions`);
}

export function runWorkflow(
  workflowId: string,
  inputs: Record<string, unknown>,
  idempotencyKey?: string,
): Promise<{ id: string }> {
  return post(`/api/v1/workflows/${workflowId}/run`, {
    inputs,
    idempotency_key: idempotencyKey,
  });
}

export function updateWorkflow(
  id: string,
  data: { name?: string; description?: string },
): Promise<void> {
  return put(`/api/v1/workflows/${id}`, data);
}

export function deleteWorkflow(id: string): Promise<void> {
  return del(`/api/v1/workflows/${id}`);
}
