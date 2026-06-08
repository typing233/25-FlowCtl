import { Routes, Route, Navigate } from 'react-router-dom';
import { useAuthStore } from './store/auth';
import Layout from './components/Layout';
import LoginPage from './pages/LoginPage';
import DashboardPage from './pages/DashboardPage';
import WorkflowsPage from './pages/WorkflowsPage';
import WorkflowDetail from './pages/WorkflowDetail';
import ExecutionsPage from './pages/ExecutionsPage';
import ExecutionDetail from './pages/ExecutionDetail';
import ApprovalsPage from './pages/ApprovalsPage';
import AdminPage from './pages/AdminPage';

function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const token = useAuthStore((s) => s.token);
  if (!token) {
    return <Navigate to="/login" replace />;
  }
  return <>{children}</>;
}

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route
        path="/*"
        element={
          <ProtectedRoute>
            <Layout>
              <Routes>
                <Route path="/" element={<DashboardPage />} />
                <Route path="/workflows" element={<WorkflowsPage />} />
                <Route path="/workflows/:id" element={<WorkflowDetail />} />
                <Route path="/executions" element={<ExecutionsPage />} />
                <Route path="/executions/:id" element={<ExecutionDetail />} />
                <Route path="/approvals" element={<ApprovalsPage />} />
                <Route path="/admin" element={<AdminPage />} />
              </Routes>
            </Layout>
          </ProtectedRoute>
        }
      />
    </Routes>
  );
}
