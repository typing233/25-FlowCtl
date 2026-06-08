import { get, post } from './client';
import type { PaginatedResponse } from './workflows';

export interface Approval {
  id: string;
  executionId: string;
  workflowName: string;
  stepName: string;
  requestedBy: string;
  requestedAt: string;
  status: 'pending' | 'approved' | 'rejected';
  respondedBy: string | null;
  respondedAt: string | null;
  comment: string | null;
}

export interface ListApprovalsParams {
  page?: number;
  pageSize?: number;
  status?: 'pending' | 'approved' | 'rejected';
}

export function listApprovals(params?: ListApprovalsParams): Promise<PaginatedResponse<Approval>> {
  const query = new URLSearchParams();
  if (params?.page) query.set('page', String(params.page));
  if (params?.pageSize) query.set('pageSize', String(params.pageSize));
  if (params?.status) query.set('status', params.status);
  return get(`/approvals?${query.toString()}`);
}

export function approveApproval(id: string, comment?: string): Promise<Approval> {
  return post(`/approvals/${id}/approve`, { comment });
}

export function rejectApproval(id: string, comment?: string): Promise<Approval> {
  return post(`/approvals/${id}/reject`, { comment });
}
