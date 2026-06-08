import { get, post } from './client';
import type { PaginatedResponse } from './workflows';

export type ApprovalStatus = 'pending' | 'approved' | 'rejected';

export interface Approval {
  id: string;
  execution_id: string;
  step_id: string;
  tenant_id: string;
  status: ApprovalStatus;
  required_roles: string[];
  requested_at: string;
  responded_at: string | null;
  responded_by: string | null;
  comment: string;
}

export interface ListApprovalsParams {
  page?: number;
  limit?: number;
}

export function listApprovals(params?: ListApprovalsParams): Promise<PaginatedResponse<Approval>> {
  const query = new URLSearchParams();
  if (params?.page) query.set('page', String(params.page));
  if (params?.limit) query.set('limit', String(params.limit));
  const qs = query.toString();
  return get(`/api/v1/approvals${qs ? '?' + qs : ''}`);
}

export function approveApproval(id: string, comment?: string): Promise<{ status: string }> {
  return post(`/api/v1/approvals/${id}/approve`, { comment });
}

export function rejectApproval(id: string, comment?: string): Promise<{ status: string }> {
  return post(`/api/v1/approvals/${id}/reject`, { comment });
}
