import { useState, useCallback, useMemo } from 'react';
import { useParams } from 'react-router-dom';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import ReactFlow, {
  Background,
  Controls,
  type Node,
  type Edge,
  MarkerType,
} from 'reactflow';
import 'reactflow/dist/style.css';
import { getWorkflow, publishVersion, runWorkflow } from '../api/workflows';
import type { WorkflowStep } from '../api/workflows';
import StatusBadge from '../components/StatusBadge';

function buildDAG(steps: WorkflowStep[]): { nodes: Node[]; edges: Edge[] } {
  const nodes: Node[] = steps.map((step, index) => ({
    id: step.id,
    data: { label: step.name },
    position: { x: (index % 4) * 220, y: Math.floor(index / 4) * 120 },
    style: {
      background: '#1e40af',
      color: '#fff',
      border: '1px solid #3b82f6',
      borderRadius: '8px',
      padding: '10px 16px',
      fontSize: '13px',
      fontWeight: 500,
    },
  }));

  const edges: Edge[] = [];
  steps.forEach((step) => {
    step.dependencies.forEach((dep) => {
      edges.push({
        id: `${dep}-${step.id}`,
        source: dep,
        target: step.id,
        markerEnd: { type: MarkerType.ArrowClosed },
        style: { stroke: '#60a5fa' },
      });
    });
  });

  return { nodes, edges };
}

export default function WorkflowDetail() {
  const { id } = useParams<{ id: string }>();
  const queryClient = useQueryClient();
  const [source, setSource] = useState('');
  const [changelog, setChangelog] = useState('');
  const [runInputs, setRunInputs] = useState('{}');
  const [showRunDialog, setShowRunDialog] = useState(false);
  const [activeTab, setActiveTab] = useState<'dag' | 'source' | 'versions'>('dag');

  const { data: workflow, isLoading } = useQuery({
    queryKey: ['workflow', id],
    queryFn: () => getWorkflow(id!),
    enabled: !!id,
  });

  const publishMutation = useMutation({
    mutationFn: () => publishVersion(id!, { source, changelog }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['workflow', id] });
      setChangelog('');
    },
  });

  const runMutation = useMutation({
    mutationFn: () => runWorkflow(id!, JSON.parse(runInputs)),
    onSuccess: () => {
      setShowRunDialog(false);
      setRunInputs('{}');
    },
  });

  const dag = useMemo(
    () => (workflow?.steps ? buildDAG(workflow.steps) : { nodes: [], edges: [] }),
    [workflow?.steps],
  );

  const onInit = useCallback(() => {}, []);

  if (isLoading) {
    return (
      <div className="flex justify-center py-12">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600" />
      </div>
    );
  }

  if (!workflow) {
    return <div className="text-center py-12 text-gray-500">Workflow not found</div>;
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">{workflow.name}</h1>
          <p className="text-gray-500 dark:text-gray-400 mt-1">{workflow.description}</p>
        </div>
        <div className="flex gap-3">
          <StatusBadge status={workflow.status} />
          <span className="text-sm text-gray-500">v{workflow.currentVersion}</span>
          <button onClick={() => setShowRunDialog(true)} className="btn-primary">
            Run Workflow
          </button>
        </div>
      </div>

      {/* Tabs */}
      <div className="border-b border-gray-200 dark:border-gray-700">
        <nav className="flex gap-6">
          {(['dag', 'source', 'versions'] as const).map((tab) => (
            <button
              key={tab}
              onClick={() => setActiveTab(tab)}
              className={`pb-3 px-1 border-b-2 font-medium text-sm capitalize transition-colors ${
                activeTab === tab
                  ? 'border-primary-600 text-primary-600'
                  : 'border-transparent text-gray-500 hover:text-gray-700 dark:hover:text-gray-300'
              }`}
            >
              {tab === 'dag' ? 'DAG Visualization' : tab}
            </button>
          ))}
        </nav>
      </div>

      {/* DAG View */}
      {activeTab === 'dag' && (
        <div className="card p-0 overflow-hidden" style={{ height: 500 }}>
          <ReactFlow
            nodes={dag.nodes}
            edges={dag.edges}
            onInit={onInit}
            fitView
            attributionPosition="bottom-left"
          >
            <Background />
            <Controls />
          </ReactFlow>
        </div>
      )}

      {/* Source Editor */}
      {activeTab === 'source' && (
        <div className="card space-y-4">
          <textarea
            value={source || workflow.source}
            onChange={(e) => setSource(e.target.value)}
            className="w-full h-80 font-mono text-sm input-field resize-none"
            placeholder="Workflow source (YAML/JSON)"
          />
          <div className="flex gap-3 items-end">
            <div className="flex-1">
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                Changelog
              </label>
              <input
                type="text"
                value={changelog}
                onChange={(e) => setChangelog(e.target.value)}
                className="input-field"
                placeholder="Describe the changes..."
              />
            </div>
            <button
              onClick={() => publishMutation.mutate()}
              disabled={publishMutation.isPending || !changelog}
              className="btn-primary"
            >
              {publishMutation.isPending ? 'Publishing...' : 'Publish Version'}
            </button>
          </div>
        </div>
      )}

      {/* Version History */}
      {activeTab === 'versions' && (
        <div className="card">
          <div className="space-y-3">
            {workflow.versions?.map((v) => (
              <div
                key={v.version}
                className="flex items-center justify-between py-3 border-b border-gray-100 dark:border-gray-700 last:border-0"
              >
                <div>
                  <span className="font-medium">v{v.version}</span>
                  <span className="ml-3 text-gray-500 text-sm">{v.changelog}</span>
                </div>
                <div className="text-sm text-gray-400">
                  {new Date(v.publishedAt).toLocaleString()} by {v.publishedBy}
                </div>
              </div>
            ))}
            {(!workflow.versions || workflow.versions.length === 0) && (
              <p className="text-gray-500 text-center py-4">No versions published yet</p>
            )}
          </div>
        </div>
      )}

      {/* Run Dialog */}
      {showRunDialog && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="bg-white dark:bg-gray-800 rounded-xl shadow-xl p-6 w-full max-w-md">
            <h2 className="text-lg font-bold mb-4">Run Workflow</h2>
            <div className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                  Inputs (JSON)
                </label>
                <textarea
                  value={runInputs}
                  onChange={(e) => setRunInputs(e.target.value)}
                  className="input-field font-mono text-sm h-40 resize-none"
                />
              </div>
              <div className="flex gap-3 justify-end">
                <button onClick={() => setShowRunDialog(false)} className="btn-secondary">
                  Cancel
                </button>
                <button
                  onClick={() => runMutation.mutate()}
                  disabled={runMutation.isPending}
                  className="btn-primary"
                >
                  {runMutation.isPending ? 'Starting...' : 'Run'}
                </button>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
