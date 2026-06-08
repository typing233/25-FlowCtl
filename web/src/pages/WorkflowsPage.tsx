import { useState } from 'react';
import { Link } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { listWorkflows } from '../api/workflows';
import DataTable from '../components/DataTable';
import StatusBadge from '../components/StatusBadge';

export default function WorkflowsPage() {
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);

  const { data, isLoading } = useQuery({
    queryKey: ['workflows', page, search],
    queryFn: () => listWorkflows({ page, pageSize: 20, search }),
  });

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Workflows</h1>
        <Link to="/workflows/new" className="btn-primary">
          Create Workflow
        </Link>
      </div>

      <div className="card">
        <div className="mb-4">
          <input
            type="text"
            placeholder="Search workflows..."
            value={search}
            onChange={(e) => {
              setSearch(e.target.value);
              setPage(1);
            }}
            className="input-field max-w-sm"
          />
        </div>

        {isLoading ? (
          <div className="flex justify-center py-12">
            <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600" />
          </div>
        ) : (
          <DataTable
            columns={[
              {
                key: 'name',
                header: 'Name',
                render: (row) => (
                  <Link
                    to={`/workflows/${row.id}`}
                    className="text-primary-600 hover:text-primary-700 font-medium"
                  >
                    {row.name}
                  </Link>
                ),
              },
              { key: 'description', header: 'Description' },
              {
                key: 'currentVersion',
                header: 'Version',
                render: (row) => <span>v{row.currentVersion}</span>,
              },
              {
                key: 'status',
                header: 'Status',
                render: (row) => <StatusBadge status={row.status} />,
              },
              {
                key: 'updatedAt',
                header: 'Updated',
                render: (row) => new Date(row.updatedAt).toLocaleDateString(),
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
