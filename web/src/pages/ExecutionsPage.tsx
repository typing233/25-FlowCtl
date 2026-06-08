import { useState } from 'react';
import { Link } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { listExecutions } from '../api/executions';
import type { ExecutionStatus } from '../api/executions';
import DataTable from '../components/DataTable';
import StatusBadge from '../components/StatusBadge';

export default function ExecutionsPage() {
  const [page, setPage] = useState(1);
  const [statusFilter, setStatusFilter] = useState<ExecutionStatus | ''>('');

  const { data, isLoading } = useQuery({
    queryKey: ['executions', page, statusFilter],
    queryFn: () =>
      listExecutions({
        page,
        pageSize: 20,
        status: statusFilter || undefined,
      }),
  });

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Executions</h1>
      </div>

      <div className="card">
        <div className="mb-4 flex gap-3">
          <select
            value={statusFilter}
            onChange={(e) => {
              setStatusFilter(e.target.value as ExecutionStatus | '');
              setPage(1);
            }}
            className="input-field max-w-xs"
          >
            <option value="">All Statuses</option>
            <option value="pending">Pending</option>
            <option value="running">Running</option>
            <option value="success">Success</option>
            <option value="failed">Failed</option>
            <option value="cancelled">Cancelled</option>
            <option value="waiting_approval">Waiting Approval</option>
          </select>
        </div>

        {isLoading ? (
          <div className="flex justify-center py-12">
            <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600" />
          </div>
        ) : (
          <DataTable
            columns={[
              {
                key: 'id',
                header: 'Execution',
                render: (row) => (
                  <Link
                    to={`/executions/${row.id}`}
                    className="text-primary-600 hover:text-primary-700 font-mono text-sm"
                  >
                    {row.id.slice(0, 8)}...
                  </Link>
                ),
              },
              { key: 'workflowName', header: 'Workflow' },
              {
                key: 'version',
                header: 'Version',
                render: (row) => <span>v{row.version}</span>,
              },
              {
                key: 'status',
                header: 'Status',
                render: (row) => <StatusBadge status={row.status} />,
              },
              { key: 'triggeredBy', header: 'Triggered By' },
              {
                key: 'startedAt',
                header: 'Started',
                render: (row) => new Date(row.startedAt).toLocaleString(),
              },
              {
                key: 'completedAt',
                header: 'Completed',
                render: (row) =>
                  row.completedAt ? new Date(row.completedAt).toLocaleString() : '-',
              },
            ]}
            data={data?.items || []}
            totalItems={data?.total || 0}
            page={page}
            pageSize={20}
            onPageChange={setPage}
          />
        )}
      </div>
    </div>
  );
}
