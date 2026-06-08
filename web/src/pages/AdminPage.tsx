import { useState } from 'react';

interface User {
  id: string;
  email: string;
  name: string;
  role: string;
  status: 'active' | 'disabled';
}

interface Role {
  id: string;
  name: string;
  permissions: string[];
}

export default function AdminPage() {
  const [activeTab, setActiveTab] = useState<'users' | 'roles'>('users');

  // Placeholder data - in a real app these would come from API queries
  const users: User[] = [];
  const roles: Role[] = [];

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold">Administration</h1>

      {/* Tabs */}
      <div className="border-b border-gray-200 dark:border-gray-700">
        <nav className="flex gap-6">
          <button
            onClick={() => setActiveTab('users')}
            className={`pb-3 px-1 border-b-2 font-medium text-sm transition-colors ${
              activeTab === 'users'
                ? 'border-primary-600 text-primary-600'
                : 'border-transparent text-gray-500 hover:text-gray-700 dark:hover:text-gray-300'
            }`}
          >
            Users
          </button>
          <button
            onClick={() => setActiveTab('roles')}
            className={`pb-3 px-1 border-b-2 font-medium text-sm transition-colors ${
              activeTab === 'roles'
                ? 'border-primary-600 text-primary-600'
                : 'border-transparent text-gray-500 hover:text-gray-700 dark:hover:text-gray-300'
            }`}
          >
            Roles
          </button>
        </nav>
      </div>

      {/* Users Tab */}
      {activeTab === 'users' && (
        <div className="card">
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-lg font-semibold">User Management</h2>
            <button className="btn-primary text-sm">Invite User</button>
          </div>

          {users.length === 0 ? (
            <p className="text-gray-500 text-center py-8">No users configured</p>
          ) : (
            <table className="w-full">
              <thead>
                <tr className="border-b border-gray-200 dark:border-gray-700 text-left">
                  <th className="pb-3 text-sm font-medium text-gray-500">Name</th>
                  <th className="pb-3 text-sm font-medium text-gray-500">Email</th>
                  <th className="pb-3 text-sm font-medium text-gray-500">Role</th>
                  <th className="pb-3 text-sm font-medium text-gray-500">Status</th>
                  <th className="pb-3 text-sm font-medium text-gray-500">Actions</th>
                </tr>
              </thead>
              <tbody>
                {users.map((user) => (
                  <tr key={user.id} className="border-b border-gray-100 dark:border-gray-700/50">
                    <td className="py-3 text-sm font-medium">{user.name}</td>
                    <td className="py-3 text-sm text-gray-500">{user.email}</td>
                    <td className="py-3 text-sm">{user.role}</td>
                    <td className="py-3">
                      <span
                        className={`text-xs px-2 py-1 rounded-full ${
                          user.status === 'active'
                            ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400'
                            : 'bg-gray-100 text-gray-700 dark:bg-gray-700 dark:text-gray-400'
                        }`}
                      >
                        {user.status}
                      </span>
                    </td>
                    <td className="py-3">
                      <button className="text-sm text-primary-600 hover:text-primary-700">
                        Edit
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      )}

      {/* Roles Tab */}
      {activeTab === 'roles' && (
        <div className="card">
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-lg font-semibold">Role Management</h2>
            <button className="btn-primary text-sm">Create Role</button>
          </div>

          {roles.length === 0 ? (
            <p className="text-gray-500 text-center py-8">No custom roles defined</p>
          ) : (
            <div className="space-y-3">
              {roles.map((role) => (
                <div
                  key={role.id}
                  className="flex items-center justify-between py-3 px-4 bg-gray-50 dark:bg-gray-900 rounded-lg"
                >
                  <div>
                    <span className="font-medium">{role.name}</span>
                    <p className="text-xs text-gray-500 mt-1">
                      {role.permissions.length} permissions
                    </p>
                  </div>
                  <button className="text-sm text-primary-600 hover:text-primary-700">
                    Edit
                  </button>
                </div>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
