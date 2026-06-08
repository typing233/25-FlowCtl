import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import * as Dialog from '@radix-ui/react-dialog';
import { listApprovals, approveApproval, rejectApproval } from '../api/approvals';
import type { Approval } from '../api/approvals';
import StatusBadge from '../components/StatusBadge';

export default function ApprovalsPage() {
  const queryClient = useQueryClient();
  const [selectedApproval, setSelectedApproval] = useState<Approval | null>(null);
  const [comment, setComment] = useState('');
  const [action, setAction] = useState<'approve' | 'reject' | null>(null);

  const { data, isLoading } = useQuery({
    queryKey: ['approvals', 'pending'],
    queryFn: () => listApprovals({ status: 'pending' }),
  });

  const approveMutation = useMutation({
    mutationFn: (id: string) => approveApproval(id, comment),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['approvals'] });
      closeDialog();
    },
  });

  const rejectMutation = useMutation({
    mutationFn: (id: string) => rejectApproval(id, comment),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['approvals'] });
      closeDialog();
    },
  });

  function openDialog(approval: Approval, actionType: 'approve' | 'reject') {
    setSelectedApproval(approval);
    setAction(actionType);
    setComment('');
  }

  function closeDialog() {
    setSelectedApproval(null);
    setAction(null);
    setComment('');
  }

  function handleConfirm() {
    if (!selectedApproval || !action) return;
    if (action === 'approve') {
      approveMutation.mutate(selectedApproval.id);
    } else {
      rejectMutation.mutate(selectedApproval.id);
    }
  }

  if (isLoading) {
    return (
      <div className="flex justify-center py-12">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600" />
      </div>
    );
  }

  const approvals = data?.items || [];

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold">Pending Approvals</h1>

      {approvals.length === 0 ? (
        <div className="card text-center py-12 text-gray-500">
          No pending approvals
        </div>
      ) : (
        <div className="space-y-3">
          {approvals.map((approval) => (
            <div key={approval.id} className="card flex items-center justify-between">
              <div>
                <div className="flex items-center gap-3">
                  <StatusBadge status={approval.status} />
                  <span className="font-medium">{approval.workflowName}</span>
                  <span className="text-gray-500 text-sm">/ {approval.stepName}</span>
                </div>
                <div className="mt-1 text-sm text-gray-500">
                  Requested by {approval.requestedBy} on{' '}
                  {new Date(approval.requestedAt).toLocaleString()}
                </div>
                <div className="mt-1 text-xs text-gray-400 font-mono">
                  Execution: {approval.executionId.slice(0, 8)}...
                </div>
              </div>
              <div className="flex gap-2">
                <button
                  onClick={() => openDialog(approval, 'approve')}
                  className="btn-primary text-sm py-1.5 px-3"
                >
                  Approve
                </button>
                <button
                  onClick={() => openDialog(approval, 'reject')}
                  className="btn-danger text-sm py-1.5 px-3"
                >
                  Reject
                </button>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Approval/Rejection Dialog */}
      <Dialog.Root open={!!selectedApproval} onOpenChange={(open) => !open && closeDialog()}>
        <Dialog.Portal>
          <Dialog.Overlay className="fixed inset-0 bg-black/50 z-50" />
          <Dialog.Content className="fixed top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 bg-white dark:bg-gray-800 rounded-xl shadow-xl p-6 w-full max-w-md z-50">
            <Dialog.Title className="text-lg font-bold">
              {action === 'approve' ? 'Approve' : 'Reject'} Request
            </Dialog.Title>
            <Dialog.Description className="mt-2 text-sm text-gray-500">
              {action === 'approve'
                ? 'Are you sure you want to approve this step?'
                : 'Are you sure you want to reject this step?'}
            </Dialog.Description>

            <div className="mt-4">
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                Comment (optional)
              </label>
              <textarea
                value={comment}
                onChange={(e) => setComment(e.target.value)}
                className="input-field h-24 resize-none"
                placeholder="Add a comment..."
              />
            </div>

            <div className="mt-4 flex gap-3 justify-end">
              <Dialog.Close asChild>
                <button className="btn-secondary">Cancel</button>
              </Dialog.Close>
              <button
                onClick={handleConfirm}
                disabled={approveMutation.isPending || rejectMutation.isPending}
                className={action === 'approve' ? 'btn-primary' : 'btn-danger'}
              >
                {approveMutation.isPending || rejectMutation.isPending
                  ? 'Processing...'
                  : action === 'approve'
                  ? 'Approve'
                  : 'Reject'}
              </button>
            </div>
          </Dialog.Content>
        </Dialog.Portal>
      </Dialog.Root>
    </div>
  );
}
