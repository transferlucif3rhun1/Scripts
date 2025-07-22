import React, { useState, useEffect } from 'react';
import { motion } from 'framer-motion';
import { 
  Button, Card, Input, Select, Alert, Table, Badge 
} from './ui';
import { 
  getSettings, saveSettings, getMonitoringStats,
  getLogs, clearLogs, healthCheck, getApiEndpoint, setApiEndpoint,
  listTemplates, createTemplate, updateTemplate, deleteTemplate, getDefaultTemplate 
} from './api';
import { 
  SystemLog, UserSettings, MonitoringStats, KeyTemplate, UIPreferences, ThemeSettings 
} from './types';

// Logs component
const LogsViewer: React.FC = () => {
  const [logs, setLogs] = useState<SystemLog[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [level, setLevel] = useState('all');
  const [component, setComponent] = useState('all');
  const [limit, setLimit] = useState('100');
  const [search, setSearch] = useState('');
  const [success, setSuccess] = useState<string | null>(null);

  const fetchLogs = async () => {
    setLoading(true);
    setError(null);
    
    try {
      const filters = {
        level,
        component,
        limit: parseInt(limit),
        search: search.trim(),
      };
      
      const logsData = await getLogs(filters);
      setLogs(logsData);
    } catch (err) {
      console.error('Error fetching logs:', err);
      setError('Failed to fetch logs');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchLogs();
  }, []);

  const handleClearLogs = async () => {
    if (!window.confirm('Are you sure you want to clear all logs? This action cannot be undone.')) {
      return;
    }
    
    setLoading(true);
    
    try {
      await clearLogs();
      setSuccess('Logs cleared successfully');
      fetchLogs();
      
      setTimeout(() => {
        setSuccess(null);
      }, 3000);
    } catch (err) {
      console.error('Error clearing logs:', err);
      setError('Failed to clear logs');
    } finally {
      setLoading(false);
    }
  };

  const getLevelBadge = (level: string) => {
    switch (level) {
      case 'error':
        return <Badge variant="danger">Error</Badge>;
      case 'warning':
        return <Badge variant="warning">Warning</Badge>;
      case 'info':
        return <Badge variant="info">Info</Badge>;
      default:
        return <Badge variant="default">Debug</Badge>;
    }
  };

  const logColumns = [
    {
      title: 'Time',
      key: 'timestamp',
      render: (value: string) => new Date(value).toLocaleString(),
    },
    {
      title: 'Level',
      key: 'level',
      render: (value: string) => getLevelBadge(value),
    },
    {
      title: 'Component',
      key: 'component',
    },
    {
      title: 'Message',
      key: 'message',
    },
    {
      title: 'Details',
      key: 'details',
      render: (value: string) => value || '-',
    },
  ];

  return (
    <Card title="System Logs" className="mb-8">
      {error && (
        <Alert type="error" message={error} onClose={() => setError(null)} className="mb-4" />
      )}
      
      {success && (
        <Alert type="success" message={success} onClose={() => setSuccess(null)} className="mb-4" />
      )}
      
      <div className="flex flex-col md:flex-row gap-4 mb-4">
        <Select
          label="Level"
          value={level}
          onChange={(e) => setLevel(e.target.value)}
          options={[
            { value: 'all', label: 'All Levels' },
            { value: 'error', label: 'Error Only' },
            { value: 'warning', label: 'Warning & Error' },
            { value: 'info', label: 'Info & Above' },
            { value: 'debug', label: 'Debug & Above' },
          ]}
          className="w-full md:w-1/4"
        />
        
        <Select
          label="Component"
          value={component}
          onChange={(e) => setComponent(e.target.value)}
          options={[
            { value: 'all', label: 'All Components' },
            { value: 'system', label: 'System' },
            { value: 'api-key', label: 'API Key' },
            { value: 'database', label: 'Database' },
            { value: 'api-request', label: 'API Request' },
            { value: 'api-response', label: 'API Response' },
            { value: 'settings-manager', label: 'Settings Manager' },
          ]}
          className="w-full md:w-1/4"
        />
        
        <Select
          label="Limit"
          value={limit}
          onChange={(e) => setLimit(e.target.value)}
          options={[
            { value: '50', label: '50 logs' },
            { value: '100', label: '100 logs' },
            { value: '200', label: '200 logs' },
            { value: '500', label: '500 logs' },
          ]}
          className="w-full md:w-1/4"
        />
        
        <Input
          label="Search"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search logs..."
          className="w-full md:w-1/4"
        />
      </div>
      
      <div className="flex justify-between mb-4">
        <Button variant="primary" onClick={fetchLogs} disabled={loading}>
          {loading ? 'Loading...' : 'Refresh Logs'}
        </Button>
        
        <Button variant="danger" onClick={handleClearLogs} disabled={loading}>
          Clear Logs
        </Button>
      </div>
      
      <Table
        columns={logColumns}
        data={logs}
        isLoading={loading}
        rowKey="id"
        className="mt-4"
      />
    </Card>
  );
};

// Templates Manager
const TemplatesManager: React.FC = () => {
  const [templates, setTemplates] = useState<KeyTemplate[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [editingTemplate, setEditingTemplate] = useState<KeyTemplate | null>(null);
  const [isCreating, setIsCreating] = useState(false);
  
  const [formState, setFormState] = useState<Partial<KeyTemplate>>({
    name: '',
    description: '',
    rpm: 60,
    threads_limit: 5,
    duration: '30d',
    is_default: false,
  });

  const fetchTemplates = async () => {
    setLoading(true);
    setError(null);
    
    try {
      const templatesData = await listTemplates();
      setTemplates(templatesData);
    } catch (err) {
      console.error('Error fetching templates:', err);
      setError('Failed to fetch templates');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchTemplates();
  }, []);

  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement>) => {
    const { name, value, type } = e.target as HTMLInputElement;
    
    setFormState((prev) => ({
      ...prev,
      [name]: type === 'checkbox' ? (e.target as HTMLInputElement).checked : 
               type === 'number' ? parseInt(value) : value,
    }));
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    setError(null);
    
    try {
      if (isCreating) {
        await createTemplate({
          ...formState,
          created: new Date().toISOString(),
          last_modified: new Date().toISOString(),
        });
        setSuccess('Template created successfully');
      } else if (editingTemplate) {
        await updateTemplate(editingTemplate.id!, {
          ...formState,
          last_modified: new Date().toISOString(),
        });
        setSuccess('Template updated successfully');
      }
      
      fetchTemplates();
      resetForm();
    } catch (err) {
      console.error('Error saving template:', err);
      setError('Failed to save template');
    } finally {
      setLoading(false);
    }
  };

  const handleEdit = (template: KeyTemplate) => {
    setEditingTemplate(template);
    setIsCreating(false);
    setFormState({
      name: template.name,
      description: template.description || '',
      rpm: template.rpm,
      threads_limit: template.threads_limit,
      duration: template.duration,
      is_default: template.is_default,
    });
  };

  const handleDelete = async (id: string) => {
    if (!window.confirm('Are you sure you want to delete this template?')) {
      return;
    }
    
    setLoading(true);
    
    try {
      await deleteTemplate(id);
      setSuccess('Template deleted successfully');
      fetchTemplates();
    } catch (err) {
      console.error('Error deleting template:', err);
      setError('Failed to delete template');
    } finally {
      setLoading(false);
    }
  };

  const resetForm = () => {
    setFormState({
      name: '',
      description: '',
      rpm: 60,
      threads_limit: 5,
      duration: '30d',
      is_default: false,
    });
    setEditingTemplate(null);
    setIsCreating(false);
  };

  return (
    <Card title="Key Templates" className="mb-8">
      {error && (
        <Alert type="error" message={error} onClose={() => setError(null)} className="mb-4" />
      )}
      
      {success && (
        <Alert type="success" message={success} onClose={() => setSuccess(null)} className="mb-4" />
      )}
      
      <div className="grid grid-cols-1 md:grid-cols-3 gap-8">
        <div className="md:col-span-1">
          <h3 className="text-lg font-medium mb-4">
            {isCreating ? 'Create Template' : editingTemplate ? 'Edit Template' : 'Template Form'}
          </h3>
          
          <form onSubmit={handleSubmit}>
            <Input
              label="Template Name"
              name="name"
              value={formState.name || ''}
              onChange={handleInputChange}
              placeholder="e.g. Default Template"
              required
            />
            
            <div className="mb-4">
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                Description
              </label>
              <textarea
                name="description"
                value={formState.description || ''}
                onChange={handleInputChange}
                placeholder="Describe the template purpose"
                className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-700 dark:text-white"
                rows={3}
              ></textarea>
            </div>
            
            <Input
              label="Rate Limit (RPM)"
              name="rpm"
              type="number"
              value={formState.rpm?.toString() || '60'}
              onChange={handleInputChange}
              required
            />
            
            <Input
              label="Thread Limit"
              name="threads_limit"
              type="number"
              value={formState.threads_limit?.toString() || '5'}
              onChange={handleInputChange}
              required
            />
            
            <Select
              label="Duration"
              name="duration"
              value={formState.duration || '30d'}
              onChange={handleInputChange}
              options={[
                { value: '24h', label: '24 Hours' },
                { value: '7d', label: '7 Days' },
                { value: '30d', label: '30 Days' },
                { value: '90d', label: '90 Days' },
                { value: '365d', label: '1 Year' },
              ]}
              required
            />
            
            <div className="flex items-center mb-4">
              <input
                type="checkbox"
                id="is_default"
                name="is_default"
                checked={formState.is_default || false}
                onChange={handleInputChange}
                className="h-4 w-4 text-blue-600 focus:ring-blue-500 border-gray-300 rounded"
              />
              <label htmlFor="is_default" className="ml-2 block text-sm text-gray-900 dark:text-gray-300">
                Set as Default Template
              </label>
            </div>
            
            <div className="flex space-x-3 mt-6">
              <Button type="submit" variant="primary" disabled={loading}>
                {loading ? 'Saving...' : isCreating ? 'Create Template' : 'Update Template'}
              </Button>
              
              <Button type="button" variant="outline" onClick={resetForm}>
                Cancel
              </Button>
            </div>
          </form>
        </div>
        
        <div className="md:col-span-2">
          <div className="flex justify-between items-center mb-4">
            <h3 className="text-lg font-medium">Available Templates</h3>
            <Button variant="outline" onClick={() => { resetForm(); setIsCreating(true); }}>
              New Template
            </Button>
          </div>
          
          {loading && templates.length === 0 ? (
            <div className="flex justify-center my-8">
              <svg className="animate-spin h-8 w-8 text-blue-500" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
                <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
              </svg>
            </div>
          ) : templates.length === 0 ? (
            <div className="text-center py-8 bg-gray-50 dark:bg-gray-800 rounded-md">
              <p className="text-gray-500 dark:text-gray-400">No templates found. Create your first template!</p>
            </div>
          ) : (
            <div className="space-y-4">
              {templates.map((template) => (
                <div key={template.id} className="border border-gray-200 dark:border-gray-700 rounded-md p-4">
                  <div className="flex justify-between">
                    <div>
                      <h4 className="text-lg font-medium flex items-center">
                        {template.name}
                        {template.is_default && (
                          <Badge variant="info" className="ml-2">Default</Badge>
                        )}
                      </h4>
                      {template.description && (
                        <p className="text-gray-500 dark:text-gray-400 text-sm mt-1">{template.description}</p>
                      )}
                    </div>
                    <div className="flex space-x-2">
                      <button
                        onClick={() => handleEdit(template)}
                        className="text-blue-600 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-300"
                      >
                        Edit
                      </button>
                      <button
                        onClick={() => handleDelete(template.id!)}
                        className="text-red-600 hover:text-red-800 dark:text-red-400 dark:hover:text-red-300"
                      >
                        Delete
                      </button>
                    </div>
                  </div>
                  
                  <div className="grid grid-cols-2 sm:grid-cols-3 gap-4 mt-4 text-sm">
                    <div>
                      <span className="text-gray-500 dark:text-gray-400">Rate Limit:</span>
                      <span className="ml-1 font-medium">{template.rpm} RPM</span>
                    </div>
                    <div>
                      <span className="text-gray-500 dark:text-gray-400">Thread Limit:</span>
                      <span className="ml-1 font-medium">{template.threads_limit}</span>
                    </div>
                    <div>
                      <span className="text-gray-500 dark:text-gray-400">Duration:</span>
                      <span className="ml-1 font-medium">{template.duration}</span>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </Card>
  );
};

// SystemConfig component to be integrated into your existing SettingsPage.tsx
// Replace the current SystemConfig component with this improved version

const SystemConfig: React.FC = () => {
  const [apiEndpoint, setApiEndpointState] = useState(getApiEndpoint());
  const [testStatus, setTestStatus] = useState<'idle' | 'testing' | 'success' | 'error'>('idle');
  const [stats, setStats] = useState<MonitoringStats | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [connectionStatus, setConnectionStatus] = useState<'connected' | 'disconnected' | 'checking'>('checking');
  const [lastChecked, setLastChecked] = useState<Date | null>(null);

  const fetchStats = async () => {
    setLoading(true);
    setError(null);
    
    try {
      console.log('Fetching monitoring stats...');
      const statsData = await getMonitoringStats();
      console.log('Stats fetched successfully:', statsData);
      setStats(statsData);
    } catch (err) {
      console.error('Error fetching stats:', err);
      setError('Failed to fetch statistics. Check your connection to the API server.');
    } finally {
      setLoading(false);
    }
  };

  // Check connection status on component mount and periodically
  useEffect(() => {
    const checkConnection = async () => {
      setConnectionStatus('checking');
      try {
        await healthCheck();
        setConnectionStatus('connected');
        setLastChecked(new Date());
      } catch (error) {
        console.error('Connection check failed:', error);
        setConnectionStatus('disconnected');
        setLastChecked(new Date());
      }
    };
    
    checkConnection();
    fetchStats();
    
    const interval = setInterval(checkConnection, 30000);
    return () => clearInterval(interval);
  }, []);

  const handleTestConnection = async () => {
    setTestStatus('testing');
    setError(null);
    
    try {
      console.log('Testing connection to:', apiEndpoint);
      await healthCheck();
      console.log('Connection test successful');
      setTestStatus('success');
      
      setTimeout(() => {
        setTestStatus('idle');
      }, 3000);
    } catch (err) {
      console.error('Test connection failed:', err);
      setTestStatus('error');
      setError(`Failed to connect to ${apiEndpoint}. Please check that the server is running and accessible.`);
      
      setTimeout(() => {
        setTestStatus('idle');
      }, 5000);
    }
  };

  const handleSaveEndpoint = () => {
    try {
      // Basic validation
      if (!apiEndpoint.startsWith('http://') && !apiEndpoint.startsWith('https://')) {
        setError('API endpoint must start with http:// or https://');
        return;
      }
      
      console.log('Saving new API endpoint:', apiEndpoint);
      setApiEndpoint(apiEndpoint);
      setSuccess('API endpoint updated successfully. Reloading...');
      
      setTimeout(() => {
        window.location.reload();
      }, 1500);
    } catch (err) {
      console.error('Error setting API endpoint:', err);
      setError('Failed to update API endpoint');
    }
  };

  const getStatusIndicator = () => {
    const statusColor = connectionStatus === 'connected' 
      ? 'bg-green-500' 
      : connectionStatus === 'checking' 
        ? 'bg-yellow-500' 
        : 'bg-red-500';
    
    const statusText = connectionStatus === 'connected' 
      ? 'Connected' 
      : connectionStatus === 'checking' 
        ? 'Checking...' 
        : 'Disconnected';
        
    return (
      <div className="flex items-center mt-2">
        <div className={`w-3 h-3 rounded-full mr-2 ${statusColor}`}></div>
        <span className="text-sm text-gray-600 dark:text-gray-400">
          {statusText} to {apiEndpoint}
          {lastChecked && ` (Last checked: ${lastChecked.toLocaleTimeString()})`}
        </span>
      </div>
    );
  };

  return (
    <Card title="System Configuration" className="mb-8">
      {error && (
        <Alert type="error" message={error} onClose={() => setError(null)} className="mb-4" />
      )}
      
      {success && (
        <Alert type="success" message={success} onClose={() => setSuccess(null)} className="mb-4" />
      )}
      
      <div className="mb-6">
        <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
          API Endpoint
        </label>
        <div className="flex flex-col sm:flex-row gap-2">
          <input
            type="text"
            value={apiEndpoint}
            onChange={(e) => setApiEndpointState(e.target.value)}
            placeholder="http://localhost:8080"
            className="flex-grow px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md sm:rounded-l-md sm:rounded-r-none shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-700 dark:text-white"
          />
          <div className="flex">
            <Button
              type="button"
              variant="outline"
              className="sm:rounded-none border-r-0"
              onClick={handleTestConnection}
              disabled={testStatus === 'testing'}
            >
              {testStatus === 'testing' ? 'Testing...' : 'Test'}
            </Button>
            <Button
              type="button"
              variant="primary"
              className="sm:rounded-l-none"
              onClick={handleSaveEndpoint}
              disabled={testStatus === 'testing'}
            >
              Save
            </Button>
          </div>
        </div>
        
        {testStatus === 'success' && (
          <p className="mt-1 text-sm text-green-600 dark:text-green-400">
            Connection successful! API is responding.
          </p>
        )}
        
        {getStatusIndicator()}
      </div>
      
      <div className="border-t border-gray-200 dark:border-gray-700 pt-6 mb-6">
        <div className="flex justify-between items-center mb-4">
          <h3 className="text-lg font-medium">System Statistics</h3>
          <Button variant="outline" onClick={fetchStats} disabled={loading}>
            {loading ? 'Refreshing...' : 'Refresh Stats'}
          </Button>
        </div>
        
        {loading ? (
          <div className="flex justify-center my-8">
            <svg className="animate-spin h-8 w-8 text-blue-500" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
              <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
              <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
            </svg>
          </div>
        ) : stats ? (
          <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-4 gap-4">
            <div className="bg-white dark:bg-gray-800 p-4 rounded-md shadow">
              <p className="text-sm text-gray-500 dark:text-gray-400">Total Keys</p>
              <p className="text-2xl font-semibold">{stats.total_keys}</p>
            </div>
            
            <div className="bg-white dark:bg-gray-800 p-4 rounded-md shadow">
              <p className="text-sm text-gray-500 dark:text-gray-400">Active Keys</p>
              <p className="text-2xl font-semibold">{stats.active_keys}</p>
            </div>
            
            <div className="bg-white dark:bg-gray-800 p-4 rounded-md shadow">
              <p className="text-sm text-gray-500 dark:text-gray-400">Expiring Soon</p>
              <p className="text-2xl font-semibold">{stats.expiring_keys}</p>
            </div>
            
            <div className="bg-white dark:bg-gray-800 p-4 rounded-md shadow">
              <p className="text-sm text-gray-500 dark:text-gray-400">24h Requests</p>
              <p className="text-2xl font-semibold">{stats.total_requests_24h.toLocaleString()}</p>
            </div>
            
            <div className="bg-white dark:bg-gray-800 p-4 rounded-md shadow">
              <p className="text-sm text-gray-500 dark:text-gray-400">Error Rate</p>
              <p className="text-2xl font-semibold">{(stats.error_rate_24h * 100).toFixed(2)}%</p>
            </div>
            
            <div className="bg-white dark:bg-gray-800 p-4 rounded-md shadow">
              <p className="text-sm text-gray-500 dark:text-gray-400">Avg Response Time</p>
              <p className="text-2xl font-semibold">{stats.avg_response_time} ms</p>
            </div>
            
            <div className="bg-white dark:bg-gray-800 p-4 rounded-md shadow">
              <p className="text-sm text-gray-500 dark:text-gray-400">System Status</p>
              <p className="text-2xl font-semibold">{stats.system_status}</p>
            </div>
            
            <div className="bg-white dark:bg-gray-800 p-4 rounded-md shadow">
              <p className="text-sm text-gray-500 dark:text-gray-400">Database</p>
              <p className="text-2xl font-semibold">
                {stats.database_connected ? (
                  <span className="text-green-600 dark:text-green-400">Connected</span>
                ) : (
                  <span className="text-red-600 dark:text-red-400">Disconnected</span>
                )}
              </p>
            </div>
          </div>
        ) : connectionStatus === 'disconnected' ? (
          <div className="bg-red-50 dark:bg-red-900/30 rounded-lg p-6 text-center">
            <p className="text-red-600 dark:text-red-400 mb-4">
              Cannot retrieve statistics: API server is not connected.
            </p>
            <p className="text-gray-600 dark:text-gray-400 mb-4">
              Please check that the API server is running at: {apiEndpoint}
            </p>
            <Button variant="primary" onClick={fetchStats}>
              Try Again
            </Button>
          </div>
        ) : (
          <div className="bg-gray-50 dark:bg-gray-800 rounded-lg p-6 text-center">
            <p className="text-gray-500 dark:text-gray-400">No stats available. Check your connection.</p>
            <Button variant="primary" onClick={fetchStats} className="mt-4">
              Try Again
            </Button>
          </div>
        )}
      </div>
    </Card>
  );
};

// UI Preferences
const UIPreferencesCard: React.FC = () => {
  const defaultPreferences: UIPreferences = {
    theme: {
      mode: 'system',
      primaryColor: '#3b82f6',
      reduceAnimations: false,
      tableCompact: false,
    },
    defaultPageSize: 10,
    sidebarCollapsed: false,
    dateFormat: 'MMM DD, YYYY HH:mm',
    defaultFilters: {},
  };
  
  const [preferences, setPreferences] = useState<UIPreferences>(defaultPreferences);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  useEffect(() => {
    const loadPreferences = async () => {
      try {
        const settings = await getSettings('ui_preferences');
        if (settings && settings.data) {
          setPreferences(settings.data as UIPreferences);
        }
      } catch (err) {
        console.error('Error loading preferences:', err);
      }
    };
    
    loadPreferences();
  }, []);

  const handleThemeChange = (e: React.ChangeEvent<HTMLSelectElement>) => {
    setPreferences({
      ...preferences,
      theme: {
        ...preferences.theme,
        mode: e.target.value as 'light' | 'dark' | 'system',
      },
    });
  };

  const handleColorChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    setPreferences({
      ...preferences,
      theme: {
        ...preferences.theme,
        primaryColor: e.target.value,
      },
    });
  };

  const handleCheckboxChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const { name, checked } = e.target;
    
    setPreferences({
      ...preferences,
      theme: {
        ...preferences.theme,
        [name]: checked,
      },
    });
  };

  const handlePageSizeChange = (e: React.ChangeEvent<HTMLSelectElement>) => {
    setPreferences({
      ...preferences,
      defaultPageSize: parseInt(e.target.value),
    });
  };

  const handleDateFormatChange = (e: React.ChangeEvent<HTMLSelectElement>) => {
    setPreferences({
      ...preferences,
      dateFormat: e.target.value,
    });
  };

  const handleSavePreferences = async () => {
    setLoading(true);
    setError(null);
    
    try {
      await saveSettings({
        id: 'ui_preferences',
        data: preferences,
        settingsVersion: 1,
      });
      
      // Apply theme
      document.documentElement.classList.remove('light', 'dark');
      
      if (preferences.theme.mode === 'dark' || 
         (preferences.theme.mode === 'system' && 
          window.matchMedia('(prefers-color-scheme: dark)').matches)) {
        document.documentElement.classList.add('dark');
      } else {
        document.documentElement.classList.add('light');
      }
      
      setSuccess('Preferences saved successfully');
      
      setTimeout(() => {
        setSuccess(null);
      }, 3000);
    } catch (err) {
      console.error('Error saving preferences:', err);
      setError('Failed to save preferences');
    } finally {
      setLoading(false);
    }
  };

  const handleResetPreferences = () => {
    setPreferences(defaultPreferences);
  };

  return (
    <Card title="User Interface Preferences" className="mb-8">
      {error && (
        <Alert type="error" message={error} onClose={() => setError(null)} className="mb-4" />
      )}
      
      {success && (
        <Alert type="success" message={success} onClose={() => setSuccess(null)} className="mb-4" />
      )}
      
      <div className="space-y-6">
        <div>
          <h3 className="text-lg font-medium mb-4">Theme Settings</h3>
          
          <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
            <Select
              label="Theme Mode"
              value={preferences.theme.mode}
              onChange={handleThemeChange}
              options={[
                { value: 'light', label: 'Light' },
                { value: 'dark', label: 'Dark' },
                { value: 'system', label: 'System Default' },
              ]}
            />
            
            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                Primary Color
              </label>
              <div className="flex items-center">
                <input
                  type="color"
                  value={preferences.theme.primaryColor}
                  onChange={handleColorChange}
                  className="h-10 w-10 border-0 p-0 mr-2"
                />
                <span className="text-sm">{preferences.theme.primaryColor}</span>
              </div>
            </div>
            
            <div className="flex items-center">
              <input
                type="checkbox"
                id="reduceAnimations"
                name="reduceAnimations"
                checked={preferences.theme.reduceAnimations}
                onChange={handleCheckboxChange}
                className="h-4 w-4 text-blue-600 focus:ring-blue-500 border-gray-300 rounded"
              />
              <label htmlFor="reduceAnimations" className="ml-2 block text-sm text-gray-900 dark:text-gray-300">
                Reduce Animations
              </label>
            </div>
            
            <div className="flex items-center">
              <input
                type="checkbox"
                id="tableCompact"
                name="tableCompact"
                checked={preferences.theme.tableCompact}
                onChange={handleCheckboxChange}
                className="h-4 w-4 text-blue-600 focus:ring-blue-500 border-gray-300 rounded"
              />
              <label htmlFor="tableCompact" className="ml-2 block text-sm text-gray-900 dark:text-gray-300">
                Compact Table View
              </label>
            </div>
          </div>
        </div>
        
        <div className="border-t border-gray-200 dark:border-gray-700 pt-6">
          <h3 className="text-lg font-medium mb-4">Display Preferences</h3>
          
          <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
            <Select
              label="Default Page Size"
              value={preferences.defaultPageSize.toString()}
              onChange={handlePageSizeChange}
              options={[
                { value: '5', label: '5 items per page' },
                { value: '10', label: '10 items per page' },
                { value: '20', label: '20 items per page' },
                { value: '50', label: '50 items per page' },
              ]}
            />
            
            <Select
              label="Date Format"
              value={preferences.dateFormat}
              onChange={handleDateFormatChange}
              options={[
                { value: 'MMM DD, YYYY HH:mm', label: 'Jan 01, 2023 13:30' },
                { value: 'MM/DD/YYYY HH:mm', label: '01/01/2023 13:30' },
                { value: 'DD/MM/YYYY HH:mm', label: '01/01/2023 13:30' },
                { value: 'YYYY-MM-DD HH:mm', label: '2023-01-01 13:30' },
              ]}
            />
          </div>
        </div>
        
        <div className="border-t border-gray-200 dark:border-gray-700 pt-6 flex justify-end space-x-3">
          <Button variant="outline" onClick={handleResetPreferences}>
            Reset to Default
          </Button>
          <Button variant="primary" onClick={handleSavePreferences} disabled={loading}>
            {loading ? 'Saving...' : 'Save Preferences'}
          </Button>
        </div>
      </div>
    </Card>
  );
};

// Main settings page
const SettingsPage: React.FC = () => {
  const [activeTab, setActiveTab] = useState('system');
  
  const tabs = [
    { id: 'system', label: 'System' },
    { id: 'templates', label: 'Templates' },
    { id: 'logs', label: 'Logs' },
    { id: 'preferences', label: 'Preferences' },
  ];

  return (
    <div className="container mx-auto">
      <motion.div
        initial={{ opacity: 0, y: 20 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.3 }}
      >
        <h1 className="text-3xl font-bold text-gray-900 dark:text-white mb-8">Settings</h1>
        
        <div className="mb-8 border-b border-gray-200 dark:border-gray-700">
          <nav className="flex flex-wrap -mb-px">
            {tabs.map((tab) => (
              <button
                key={tab.id}
                className={`py-4 px-6 font-medium text-sm border-b-2 ${
                  activeTab === tab.id
                    ? 'border-blue-500 text-blue-600 dark:text-blue-400'
                    : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300 dark:text-gray-400 dark:hover:text-gray-300'
                }`}
                onClick={() => setActiveTab(tab.id)}
              >
                {tab.label}
              </button>
            ))}
          </nav>
        </div>
        
        {activeTab === 'system' && <SystemConfig />}
        {activeTab === 'templates' && <TemplatesManager />}
        {activeTab === 'logs' && <LogsViewer />}
        {activeTab === 'preferences' && <UIPreferencesCard />}
      </motion.div>
    </div>
  );
};

export default SettingsPage;
