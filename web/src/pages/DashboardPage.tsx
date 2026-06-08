import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { listWorkflows } from '../api/workflows';
import { listExecutions } from '../api/executions';
import { listApprovals } from '../api/approvals';
import StatusBadge from '../components/StatusBadge';

export default function DashboardPage() {
  const { data: workflows } = useQuery({
    queryKey: ['workflows', 'dashboard'],
    queryFn: () => listWorkflows({ page: 1, pageSize: 1, status: 'active' }),
  });

  const { data: executions } = useQuery({
    queryKey: ['executions', 'dashboard'],
    queryFn: () => listExecutions({ page: 1, pageSize: 5, status: 'running' }),
  });

  const { data: recentExecutions } = useQuery({
    queryKey: ['executions', 'recent'],
    queryFn: () => listExecutions({ page: 1, pageSize: 10 }),
  });

  const { data: approvals } = useQuery({
    queryKey: ['approvals', 'dashboard'],
    queryFn: () => listApprovals({ page: 1, pageSize: 1, status: 'pending' }),
  });

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold">Dashboard</h1>

      {/* Stats */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <Link to="/workflows" className="card hover:shadow-md transition-shadow">
          <h3 className="text-sm font-medium text-gray-500 dark:text-gray-400">
            Active Workflows
          </h3>
          <p className="mt-2 text-3xl font-bold text-primary-600">
            {workflows?.total ?? '-'}
          </p>
        </Link>

        <Link to="/executions" className="card hover:shadow-md transition-shadow">
          <h3 className="text-sm font-medium text-gray-500 dark:text-gray-400">
            Running Executions
          </h3>
          <p className="mt-2 text-3xl font-bold text-blue-600">
            {executions?.total ?? '-'}
          </p>
        </Link>

        <Link to="/approvals" className="card hover:shadow-md transition-shadow">
          <h3 className="text-sm font-medium text-gray-500 dark:text-gray-400">
            Pending Approvals
          </h3>
          <p className="mt-2 text-3xl font-bold text-amber-600">
            {approvals?.total ?? '-'}
          </p>
        </Link>
      </div>

      {/* Recent Executions */}
      <div className="card">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-lg font-semibold">Recent Executions</h2>
          <Link to="/executions" className="text-sm text-primary-600 hover:text-primary-700">
            View all
          </Link>
        </div>

        {!recentExecutions?.items?.length ? (
          <p className="text-gray-500 text-center py-8">No recent executions</p>
        ) : (
          <div className="space-y-2">
            {recentExecutions.items.map((exec) => (
              <Link
                key={exec.id}
                to={`/executions/${exec.id}`}
                className="flex items-center justify-between py-3 px-4 bg-gray-50 dark:bg-gray-900 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-800 transition-colors"
              >
                <div className="flex items-center gap-3">
                  <StatusBadge status={exec.status} />
                  <div>
                    <span className="font-medium text-sm">{exec.workflowName}</span>
                    <span className="text-xs text-gray-500 ml-2 font-mono">
                      {exec.id.slice(0, 8)}
                    </span>
                  </div>
                </div>
                <div className="text-xs text-gray-500">
                  {new Date(exec.startedAt).toLocaleString()}
                </div>
              </Link>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
