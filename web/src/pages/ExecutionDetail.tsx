import { useState, useEffect, useRef, useMemo, useCallback } from 'react';
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
import {
  getExecution,
  cancelExecution,
  retryExecution,
  getExecutionLogs,
} from '../api/executions';
import type { StepExecution, LogEntry } from '../api/executions';
import StatusBadge from '../components/StatusBadge';

const STATUS_COLORS: Record<string, { bg: string; border: string }> = {
  success: { bg: '#065f46', border: '#10b981' },
  failed: { bg: '#7f1d1d', border: '#ef4444' },
  running: { bg: '#1e40af', border: '#3b82f6' },
  pending: { bg: '#374151', border: '#6b7280' },
  skipped: { bg: '#4b5563', border: '#9ca3af' },
};

function buildExecutionDAG(steps: StepExecution[]): { nodes: Node[]; edges: Edge[] } {
  const nodes: Node[] = steps.map((step, index) => {
    const colors = STATUS_COLORS[step.status] || STATUS_COLORS.pending;
    return {
      id: step.id,
      data: { label: `${step.name}\n(${step.status})` },
      position: { x: (index % 4) * 220, y: Math.floor(index / 4) * 120 },
      style: {
        background: colors.bg,
        color: '#fff',
        border: `2px solid ${colors.border}`,
        borderRadius: '8px',
        padding: '10px 16px',
        fontSize: '12px',
        fontWeight: 500,
        whiteSpace: 'pre-wrap' as const,
        textAlign: 'center' as const,
      },
    };
  });

  const edges: Edge[] = [];
  steps.forEach((step) => {
    step.dependencies.forEach((dep) => {
      edges.push({
        id: `${dep}-${step.id}`,
        source: dep,
        target: step.id,
        markerEnd: { type: MarkerType.ArrowClosed },
        style: { stroke: '#6b7280' },
      });
    });
  });

  return { nodes, edges };
}

export default function ExecutionDetail() {
  const { id } = useParams<{ id: string }>();
  const queryClient = useQueryClient();
  const logsEndRef = useRef<HTMLDivElement>(null);
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [wsConnected, setWsConnected] = useState(false);

  const { data: execution, isLoading } = useQuery({
    queryKey: ['execution', id],
    queryFn: () => getExecution(id!),
    enabled: !!id,
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      return status === 'running' || status === 'pending' ? 3000 : false;
    },
  });

  const { data: initialLogs } = useQuery({
    queryKey: ['execution-logs', id],
    queryFn: () => getExecutionLogs(id!),
    enabled: !!id,
  });

  useEffect(() => {
    if (initialLogs) {
      setLogs(initialLogs);
    }
  }, [initialLogs]);

  // WebSocket for live logs
  useEffect(() => {
    if (!id || execution?.status === 'success' || execution?.status === 'failed' || execution?.status === 'cancelled') {
      return;
    }

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/ws/logs/${id}`;
    const ws = new WebSocket(wsUrl);

    ws.onopen = () => setWsConnected(true);
    ws.onclose = () => setWsConnected(false);
    ws.onmessage = (event) => {
      try {
        const entry: LogEntry = JSON.parse(event.data);
        setLogs((prev) => [...prev, entry]);
      } catch {
        // ignore malformed messages
      }
    };

    return () => {
      ws.close();
    };
  }, [id, execution?.status]);

  useEffect(() => {
    logsEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [logs]);

  const cancelMutation = useMutation({
    mutationFn: () => cancelExecution(id!),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['execution', id] }),
  });

  const retryMutation = useMutation({
    mutationFn: () => retryExecution(id!),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['execution', id] }),
  });

  const dag = useMemo(
    () => (execution?.steps ? buildExecutionDAG(execution.steps) : { nodes: [], edges: [] }),
    [execution?.steps],
  );

  const onInit = useCallback(() => {}, []);

  if (isLoading) {
    return (
      <div className="flex justify-center py-12">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600" />
      </div>
    );
  }

  if (!execution) {
    return <div className="text-center py-12 text-gray-500">Execution not found</div>;
  }

  const isActive = execution.status === 'running' || execution.status === 'pending';

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Execution Detail</h1>
          <p className="text-gray-500 dark:text-gray-400 font-mono text-sm mt-1">
            {execution.id}
          </p>
        </div>
        <div className="flex gap-3 items-center">
          <StatusBadge status={execution.status} />
          {isActive && (
            <button
              onClick={() => cancelMutation.mutate()}
              disabled={cancelMutation.isPending}
              className="btn-danger"
            >
              Cancel
            </button>
          )}
          {(execution.status === 'failed' || execution.status === 'cancelled') && (
            <button
              onClick={() => retryMutation.mutate()}
              disabled={retryMutation.isPending}
              className="btn-primary"
            >
              Retry
            </button>
          )}
        </div>
      </div>

      {/* Info Grid */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <div className="card">
          <h3 className="text-sm font-medium text-gray-500 dark:text-gray-400">Workflow</h3>
          <p className="mt-1 font-medium">{execution.workflowName} v{execution.version}</p>
        </div>
        <div className="card">
          <h3 className="text-sm font-medium text-gray-500 dark:text-gray-400">Triggered By</h3>
          <p className="mt-1 font-medium">{execution.triggeredBy}</p>
        </div>
        <div className="card">
          <h3 className="text-sm font-medium text-gray-500 dark:text-gray-400">Duration</h3>
          <p className="mt-1 font-medium">
            {execution.startedAt && execution.completedAt
              ? `${Math.round((new Date(execution.completedAt).getTime() - new Date(execution.startedAt).getTime()) / 1000)}s`
              : execution.startedAt
              ? 'In progress...'
              : 'Not started'}
          </p>
        </div>
      </div>

      {/* Inputs / Outputs */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <div className="card">
          <h3 className="text-sm font-medium text-gray-500 dark:text-gray-400 mb-2">Inputs</h3>
          <pre className="text-xs bg-gray-100 dark:bg-gray-900 p-3 rounded-lg overflow-auto max-h-40">
            {JSON.stringify(execution.inputs, null, 2)}
          </pre>
        </div>
        <div className="card">
          <h3 className="text-sm font-medium text-gray-500 dark:text-gray-400 mb-2">Outputs</h3>
          <pre className="text-xs bg-gray-100 dark:bg-gray-900 p-3 rounded-lg overflow-auto max-h-40">
            {execution.outputs ? JSON.stringify(execution.outputs, null, 2) : 'N/A'}
          </pre>
        </div>
      </div>

      {/* DAG Visualization */}
      <div className="card p-0 overflow-hidden" style={{ height: 400 }}>
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

      {/* Steps List */}
      <div className="card">
        <h3 className="text-lg font-bold mb-4">Steps</h3>
        <div className="space-y-2">
          {execution.steps.map((step) => (
            <div
              key={step.id}
              className="flex items-center justify-between py-2 px-3 bg-gray-50 dark:bg-gray-900 rounded-lg"
            >
              <div className="flex items-center gap-3">
                <StatusBadge status={step.status} />
                <span className="font-medium text-sm">{step.name}</span>
              </div>
              <div className="text-xs text-gray-500">
                {step.startedAt && step.completedAt
                  ? `${Math.round((new Date(step.completedAt).getTime() - new Date(step.startedAt).getTime()) / 1000)}s`
                  : step.startedAt
                  ? 'Running...'
                  : '-'}
                {step.error && (
                  <span className="ml-2 text-red-500" title={step.error}>
                    Error
                  </span>
                )}
              </div>
            </div>
          ))}
        </div>
      </div>

      {/* Live Logs */}
      <div className="card">
        <div className="flex items-center justify-between mb-4">
          <h3 className="text-lg font-bold">Logs</h3>
          {wsConnected && (
            <span className="flex items-center gap-2 text-xs text-green-500">
              <span className="w-2 h-2 bg-green-500 rounded-full animate-pulse" />
              Live
            </span>
          )}
        </div>
        <div className="bg-gray-900 rounded-lg p-4 h-64 overflow-auto font-mono text-xs">
          {logs.map((log, i) => (
            <div
              key={i}
              className={`py-0.5 ${
                log.level === 'error'
                  ? 'text-red-400'
                  : log.level === 'warn'
                  ? 'text-yellow-400'
                  : log.level === 'debug'
                  ? 'text-gray-500'
                  : 'text-gray-300'
              }`}
            >
              <span className="text-gray-600">{new Date(log.timestamp).toLocaleTimeString()}</span>{' '}
              <span className="uppercase">[{log.level}]</span>{' '}
              {log.stepId && <span className="text-blue-400">[{log.stepId}]</span>}{' '}
              {log.message}
            </div>
          ))}
          {logs.length === 0 && <span className="text-gray-600">No logs available</span>}
          <div ref={logsEndRef} />
        </div>
      </div>
    </div>
  );
}
