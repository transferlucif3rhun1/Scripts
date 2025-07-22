import React, { useState, useEffect, useCallback } from 'react';
import { motion } from 'framer-motion';
import { 
  Button, Card, Input, Select, Table, Modal, Alert, Badge, Pagination, TagInput 
} from './ui';
import { 
  listApiKeys, getApiKey, updateApiKey, deleteApiKey,
  bulkDeleteApiKeys, bulkUpdateApiKeyTags, bulkExtendApiKeys
} from './api';
import { APIKey, Tag } from './types';

const KeyStatus: React.FC<{ apiKey: APIKey }> = ({ apiKey }) => {
  if (!apiKey.active) {
    return <Badge variant="danger">Inactive</Badge>;
  }
  
  const now = new Date();
  const expirationDate = new Date(apiKey.expiration);
  
  if (expirationDate < now) {
    return <Badge variant="danger">Expired</Badge>;
  }
  
  const sevenDaysFromNow = new Date();
  sevenDaysFromNow.setDate(now.getDate() + 7);
  
  if (expirationDate < sevenDaysFromNow) {
    return <Badge variant="warning">Expiring Soon</Badge>;
  }
  
  return <Badge variant="success">Active</Badge>;
};

const KeyDetailsModal: React.FC<{
  isOpen: boolean;
  onClose: () => void;
  keyId: string | null;
  onUpdate: () => void;
}> = ({ isOpen, onClose, keyId, onUpdate }) => {
  const [apiKey, setApiKey] = useState<APIKey | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [isEditing, setIsEditing] = useState(false);
  const [copySuccess, setCopySuccess] = useState(false);
  const [formState, setFormState] = useState({
    name: '',
    rpm: '',
    threadsLimit: '',
    active: true,
  });
  const [tags, setTags] = useState<Tag[]>([]);

  const fetchKeyDetails = useCallback(async () => {
    if (!keyId) return;
    
    try {
      setLoading(true);
      setError(null);
      const keyData = await getApiKey(keyId);
      setApiKey(keyData);
      setFormState({
        name: keyData.name || '',
        rpm: keyData.rpm.toString(),
        threadsLimit: keyData.threads_limit.toString(),
        active: keyData.active,
      });
      setTags(keyData.tags || []);
    } catch (err) {
      console.error('Error fetching key details:', err);
      setError('Failed to fetch API key details.');
    } finally {
      setLoading(false);
    }
  }, [keyId]);

  useEffect(() => {
    if (isOpen && keyId) {
      fetchKeyDetails();
    }
  }, [isOpen, keyId, fetchKeyDetails]);

  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) => {
    const { name, value, type } = e.target as HTMLInputElement;
    
    setFormState((prev) => ({
      ...prev,
      [name]: type === 'checkbox' ? (e.target as HTMLInputElement).checked : value,
    }));
  };

  const handleUpdate = async () => {
    if (!apiKey) return;
    
    try {
      setLoading(true);
      setError(null);
      
      const updateData: Partial<APIKey> = {
        name: formState.name,
        rpm: parseInt(formState.rpm),
        threads_limit: parseInt(formState.threadsLimit),
        active: formState.active,
        tags: tags
      };
      
      await updateApiKey(apiKey.key, updateData);
      
      setSuccess('API key updated successfully');
      setIsEditing(false);
      fetchKeyDetails();
      onUpdate();
      
      setTimeout(() => {
        setSuccess(null);
      }, 3000);
    } catch (err) {
      console.error('Error updating key:', err);
      setError('Failed to update API key.');
    } finally {
      setLoading(false);
    }
  };

  const handleDelete = async () => {
    if (!apiKey) return;
    
    try {
      setLoading(true);
      setError(null);
      
      await deleteApiKey(apiKey.key);
      onUpdate();
      onClose();
    } catch (err) {
      console.error('Error deleting key:', err);
      setError('Failed to delete API key.');
    } finally {
      setLoading(false);
      setConfirmDelete(false);
    }
  };

  const handleExtend = async (duration: string) => {
    if (!apiKey) return;
    
    try {
      setLoading(true);
      setError(null);
      
      await bulkExtendApiKeys([apiKey.key], duration);
      setSuccess(`API key extended by ${duration}`);
      fetchKeyDetails();
      onUpdate();
      
      setTimeout(() => {
        setSuccess(null);
      }, 3000);
    } catch (err) {
      console.error('Error extending key:', err);
      setError('Failed to extend API key.');
    } finally {
      setLoading(false);
    }
  };
  
  const handleCopyKey = () => {
    if (!apiKey) return;
    
    navigator.clipboard.writeText(apiKey.key)
      .then(() => {
        setCopySuccess(true);
        setTimeout(() => setCopySuccess(false), 2000);
      })
      .catch(err => {
        console.error('Failed to copy:', err);
      });
  };

  if (!isOpen || !apiKey) return null;

  const formatDate = (dateString: string) => {
    if (!dateString) return '-';
    try {
      return new Date(dateString).toLocaleString();
    } catch (e) {
      return dateString;
    }
  };

  const DeleteConfirmation = () => (
    <motion.div 
      initial={{ opacity: 0, y: 10 }}
      animate={{ opacity: 1, y: 0 }}
      className="p-4 bg-red-50 dark:bg-red-900 rounded-md mt-4"
    >
      <p className="text-red-700 dark:text-red-200 mb-3">
        Are you sure you want to delete this API key? This action cannot be undone.
      </p>
      <div className="flex space-x-3">
        <Button variant="danger" onClick={handleDelete} disabled={loading}>
          {loading ? 'Deleting...' : 'Yes, Delete'}
        </Button>
        <Button variant="outline" onClick={() => setConfirmDelete(false)}>
          Cancel
        </Button>
      </div>
    </motion.div>
  );

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={isEditing ? "Edit API Key" : "API Key Details"}
      size="lg"
      footer={
        <div className="flex justify-end space-x-3">
          {isEditing ? (
            <>
              <Button variant="outline" onClick={() => setIsEditing(false)}>
                Cancel
              </Button>
              <Button variant="primary" onClick={handleUpdate} disabled={loading}>
                {loading ? 'Saving...' : 'Save Changes'}
              </Button>
            </>
          ) : (
            <>
              <Button variant="outline" onClick={onClose}>
                Close
              </Button>
              <Button variant="primary" onClick={() => setIsEditing(true)}>
                Edit
              </Button>
            </>
          )}
        </div>
      }
    >
      {loading && !apiKey && (
        <div className="flex justify-center my-4">
          <div className="animate-spin h-8 w-8 border-4 border-blue-500 rounded-full border-t-transparent"></div>
        </div>
      )}

      {error && (
        <Alert
          type="error"
          message={error}
          onClose={() => setError(null)}
          className="mb-4"
        />
      )}

      {success && (
        <Alert
          type="success"
          message={success}
          onClose={() => setSuccess(null)}
          className="mb-4"
        />
      )}

      {!loading && apiKey && (
        <motion.div
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          transition={{ duration: 0.3 }}
        >
          {isEditing ? (
            <div className="space-y-4">
              <Input
                label="Key Name"
                name="name"
                value={formState.name}
                onChange={handleInputChange}
                placeholder="Enter a descriptive name"
              />
              
              <Input
                label="Rate Limit (requests per minute)"
                name="rpm"
                type="number"
                value={formState.rpm}
                onChange={handleInputChange}
                placeholder="e.g. 60"
              />
              
              <Input
                label="Concurrency Limit (max threads)"
                name="threadsLimit"
                type="number"
                value={formState.threadsLimit}
                onChange={handleInputChange}
                placeholder="e.g. 5"
              />
              
              <div className="flex items-center mb-4">
                <input
                  type="checkbox"
                  id="active"
                  name="active"
                  checked={formState.active}
                  onChange={handleInputChange}
                  className="h-4 w-4 text-blue-600 focus:ring-blue-500 border-gray-300 rounded"
                />
                <label htmlFor="active" className="ml-2 block text-sm text-gray-900 dark:text-gray-300">
                  Active
                </label>
              </div>
              
              <TagInput
                label="Tags"
                value={tags}
                onChange={setTags}
                placeholder="Type tag name and press Enter..."
              />
            </div>
          ) : (
            <>
              <div className="mb-4 relative group p-3 bg-gray-50 dark:bg-gray-700 rounded-md">
                <div className="font-mono text-sm break-all">{apiKey.key}</div>
                <button
                  onClick={handleCopyKey}
                  className="absolute top-2 right-2 p-2 opacity-0 group-hover:opacity-100 transition-opacity bg-white dark:bg-gray-600 rounded-md shadow-sm text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200"
                  title="Copy to clipboard"
                >
                  {copySuccess ? (
                    <svg className="h-5 w-5 text-green-500" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
                    </svg>
                  ) : (
                    <svg className="h-5 w-5" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 5H6a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2v-1M8 5a2 2 0 002 2h2a2 2 0 002-2M8 5a2 2 0 012-2h2a2 2 0 012 2m0 0h2a2 2 0 012 2v3m2 4H10m0 0l3-3m-3 3l3 3" />
                    </svg>
                  )}
                </button>
              </div>
              
              <div className="grid grid-cols-2 gap-x-4 gap-y-6 mb-4">
                <div>
                  <p className="text-sm font-medium text-gray-500 dark:text-gray-400">Name</p>
                  <p className="text-base font-medium">{apiKey.name || '-'}</p>
                </div>
                
                <div>
                  <p className="text-sm font-medium text-gray-500 dark:text-gray-400">Status</p>
                  <KeyStatus apiKey={apiKey} />
                </div>
                
                <div>
                  <p className="text-sm font-medium text-gray-500 dark:text-gray-400">Created</p>
                  <p className="text-base">{formatDate(apiKey.created)}</p>
                </div>
                
                <div>
                  <p className="text-sm font-medium text-gray-500 dark:text-gray-400">Expires</p>
                  <p className="text-base">{formatDate(apiKey.expiration)}</p>
                </div>
                
                <div>
                  <p className="text-sm font-medium text-gray-500 dark:text-gray-400">Rate Limit</p>
                  <p className="text-base">{apiKey.rpm || 0} req/min</p>
                </div>
                
                <div>
                  <p className="text-sm font-medium text-gray-500 dark:text-gray-400">Thread Limit</p>
                  <p className="text-base">{apiKey.threads_limit || 0}</p>
                </div>
                
                <div>
                  <p className="text-sm font-medium text-gray-500 dark:text-gray-400">Request Count</p>
                  <p className="text-base">{apiKey.request_count ? apiKey.request_count.toLocaleString() : '0'}</p>
                </div>
                
                <div>
                  <p className="text-sm font-medium text-gray-500 dark:text-gray-400">Total Requests Limit</p>
                  <p className="text-base">
                    {apiKey.total_requests && apiKey.total_requests > 0 
                      ? apiKey.total_requests.toLocaleString() 
                      : 'Unlimited'}
                  </p>
                </div>
                
                {apiKey.last_used && (
                  <div className="col-span-2">
                    <p className="text-sm font-medium text-gray-500 dark:text-gray-400">Last Used</p>
                    <p className="text-base">{formatDate(apiKey.last_used)}</p>
                  </div>
                )}
                
                {apiKey.tags && apiKey.tags.length > 0 && (
                  <div className="col-span-2">
                    <p className="text-sm font-medium text-gray-500 dark:text-gray-400 mb-2">Tags</p>
                    <div className="flex flex-wrap gap-2">
                      {apiKey.tags.map((tag, index) => (
                        <span
                          key={index}
                          className="px-2 py-1 text-xs rounded-full"
                          style={{
                            backgroundColor: tag.color || '#e5e7eb',
                            color: '#1f2937',
                          }}
                        >
                          {tag.name}
                        </span>
                      ))}
                    </div>
                  </div>
                )}
              </div>
              
              <div className="border-t border-gray-200 dark:border-gray-700 pt-4 space-y-3">
                <div>
                  <p className="text-sm font-medium text-gray-500 dark:text-gray-400 mb-2">Quick Actions</p>
                  <div className="flex flex-wrap gap-2">
                    <Button size="sm" variant="outline" onClick={() => handleExtend('24h')}>
                      Extend 24h
                    </Button>
                    <Button size="sm" variant="outline" onClick={() => handleExtend('7d')}>
                      Extend 7d
                    </Button>
                    <Button size="sm" variant="outline" onClick={() => handleExtend('30d')}>
                      Extend 30d
                    </Button>
                    <Button size="sm" variant="danger" onClick={() => setConfirmDelete(true)}>
                      Delete
                    </Button>
                  </div>
                </div>
              </div>
              
              {confirmDelete && <DeleteConfirmation />}
            </>
          )}
        </motion.div>
      )}
    </Modal>
  );
};

const BulkActionsModal: React.FC<{
  isOpen: boolean;
  onClose: () => void;
  selectedKeys: string[];
  onActionComplete: () => void;
}> = ({ isOpen, onClose, selectedKeys, onActionComplete }) => {
  const [action, setAction] = useState<'delete' | 'extend' | 'tags'>('delete');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [extension, setExtension] = useState('7d');
  const [tags, setTags] = useState<Tag[]>([]);
  const [confirm, setConfirm] = useState(false);

  const handleActionSelect = (e: React.ChangeEvent<HTMLSelectElement>) => {
    setAction(e.target.value as 'delete' | 'extend' | 'tags');
    setConfirm(false);
  };

  const handleSubmit = async () => {
    if (selectedKeys.length === 0) {
      setError('No keys selected');
      return;
    }

    setLoading(true);
    setError(null);
    
    try {
      if (action === 'delete' && confirm) {
        await bulkDeleteApiKeys(selectedKeys);
        setSuccess(`Successfully deleted ${selectedKeys.length} keys`);
      } else if (action === 'extend') {
        await bulkExtendApiKeys(selectedKeys, extension);
        setSuccess(`Successfully extended ${selectedKeys.length} keys by ${extension}`);
      } else if (action === 'tags') {
        await bulkUpdateApiKeyTags(selectedKeys, tags, []);
        setSuccess(`Successfully updated tags for ${selectedKeys.length} keys`);
      } else if (action === 'delete' && !confirm) {
        setConfirm(true);
        setLoading(false);
        return;
      }
      
      onActionComplete();
      
      setTimeout(() => {
        onClose();
      }, 2000);
    } catch (err) {
      console.error('Error performing bulk action:', err);
      setError('Failed to perform bulk action');
    } finally {
      setLoading(false);
    }
  };

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title="Bulk Actions"
      size="md"
      footer={
        <div className="flex justify-end space-x-3">
          <Button variant="outline" onClick={onClose} disabled={loading}>
            Cancel
          </Button>
          <Button variant="primary" onClick={handleSubmit} disabled={loading}>
            {loading ? 'Processing...' : action === 'delete' && confirm ? 'Confirm Delete' : 'Apply'}
          </Button>
        </div>
      }
    >
      {error && (
        <Alert type="error" message={error} onClose={() => setError(null)} className="mb-4" />
      )}
      
      {success && (
        <Alert type="success" message={success} className="mb-4" />
      )}
      
      <div className="mb-4">
        <p className="mb-2 font-medium">Selected Keys: {selectedKeys.length}</p>
        <Select
          label="Action"
          value={action}
          onChange={handleActionSelect}
          options={[
            { value: 'delete', label: 'Delete Keys' },
            { value: 'extend', label: 'Extend Expiration' },
            { value: 'tags', label: 'Manage Tags' },
          ]}
        />
      </div>
      
      {action === 'delete' && confirm && (
        <motion.div
          initial={{ opacity: 0, y: -10 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.3 }}
        >
          <Alert
            type="warning"
            message={`Are you sure you want to delete ${selectedKeys.length} keys? This action cannot be undone.`}
            className="mb-4"
          />
        </motion.div>
      )}
      
      {action === 'extend' && (
        <motion.div
          initial={{ opacity: 0, height: 0 }}
          animate={{ opacity: 1, height: 'auto' }}
          transition={{ duration: 0.3 }}
        >
          <Select
            label="Extension Period"
            value={extension}
            onChange={(e) => setExtension(e.target.value)}
            options={[
              { value: '24h', label: '24 Hours' },
              { value: '7d', label: '7 Days' },
              { value: '30d', label: '30 Days' },
              { value: '90d', label: '90 Days' },
              { value: '365d', label: '1 Year' },
            ]}
          />
        </motion.div>
      )}
      
      {action === 'tags' && (
        <motion.div
          initial={{ opacity: 0, height: 0 }}
          animate={{ opacity: 1, height: 'auto' }}
          transition={{ duration: 0.3 }}
        >
          <TagInput
            label="Add Tags"
            value={tags}
            onChange={setTags}
            placeholder="Type tag name and press Enter..."
          />
        </motion.div>
      )}
    </Modal>
  );
};

const ManagePage: React.FC = () => {
  const [keys, setKeys] = useState<APIKey[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedKeyId, setSelectedKeyId] = useState<string | null>(null);
  const [showKeyDetails, setShowKeyDetails] = useState(false);
  const [showBulkActions, setShowBulkActions] = useState(false);
  const [selectedKeys, setSelectedKeys] = useState<string[]>([]);
  const [searchTerm, setSearchTerm] = useState('');
  const [statusFilter, setStatusFilter] = useState('all');
  const [currentPage, setCurrentPage] = useState(1);
  const [totalPages, setTotalPages] = useState(1);
  const [isRefreshing, setIsRefreshing] = useState(false);

  const fetchKeys = useCallback(async () => {
    setLoading(true);
    setError(null);
    
    try {
      const filters: Record<string, string> = {
        page: currentPage.toString(),
        pageSize: '10',
      };
      
      if (searchTerm) {
        filters.search = searchTerm;
      }
      
      if (statusFilter !== 'all') {
        filters.status = statusFilter;
      }
      
      const response = await listApiKeys(currentPage, 10, filters);
      setKeys(response.data || []);
      setTotalPages(response.total_pages || 1);
    } catch (err) {
      console.error('Error fetching API keys:', err);
      setError('Failed to fetch API keys. Please try again later.');
      setKeys([]);
    } finally {
      setLoading(false);
      setIsRefreshing(false);
    }
  }, [currentPage, searchTerm, statusFilter]);

  useEffect(() => {
    fetchKeys();
  }, [fetchKeys]);

  const handleKeyClick = (keyId: string) => {
    setSelectedKeyId(keyId);
    setShowKeyDetails(true);
  };

  const handleSelectKey = (keyId: string, checked: boolean) => {
    if (checked) {
      setSelectedKeys((prev) => [...prev, keyId]);
    } else {
      setSelectedKeys((prev) => prev.filter((id) => id !== keyId));
    }
  };

  const handleSelectAll = (e: React.ChangeEvent<HTMLInputElement>) => {
    const checked = e.target.checked;
    if (checked) {
      const allKeyIds = keys.map((key) => key.key);
      setSelectedKeys(allKeyIds);
    } else {
      setSelectedKeys([]);
    }
  };

  const handleSearch = (e: React.FormEvent) => {
    e.preventDefault();
    setCurrentPage(1);
    fetchKeys();
  };

  const handleStatusChange = (e: React.ChangeEvent<HTMLSelectElement>) => {
    setStatusFilter(e.target.value);
    setCurrentPage(1);
  };

  const handlePageChange = (page: number) => {
    setCurrentPage(page);
  };

  const handleBulkActionComplete = () => {
    setSelectedKeys([]);
    fetchKeys();
  };
  
  const handleRefresh = () => {
    setIsRefreshing(true);
    fetchKeys();
  };

  const tableColumns = [
    {
      title: ' ',
      key: 'select',
      render: (_: any, record: APIKey) => (
        <input
          type="checkbox"
          checked={selectedKeys.includes(record.key)}
          onChange={(e) => handleSelectKey(record.key, e.target.checked)}
          onClick={(e) => e.stopPropagation()}
          className="h-4 w-4 text-blue-600 focus:ring-blue-500 border-gray-300 rounded cursor-pointer"
        />
      ),
    },
    {
      title: 'Name/Key',
      key: 'name',
      render: (_: string, record: APIKey) => (
        <div>
          <div className="font-medium text-gray-900 dark:text-white">
            {record.name || 'Unnamed Key'}
          </div>
          <div className="flex items-center">
            <div className="text-xs text-gray-500 dark:text-gray-400 font-mono truncate max-w-xs">
              {record.key}
            </div>
            <button 
              onClick={(e) => {
                e.stopPropagation();
                navigator.clipboard.writeText(record.key);
              }}
              className="ml-2 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 focus:outline-none"
              title="Copy to clipboard"
            >
              <svg className="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 5H6a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2v-1M8 5a2 2 0 002 2h2a2 2 0 002-2M8 5a2 2 0 012-2h2a2 2 0 012 2m0 0h2a2 2 0 012 2v3m2 4H10m0 0l3-3m-3 3l3 3" />
              </svg>
            </button>
          </div>
        </div>
      ),
    },
    {
      title: 'Status',
      key: 'status',
      render: (_: string, record: APIKey) => <KeyStatus apiKey={record} />,
    },
    {
      title: 'Expiration',
      key: 'expiration',
      render: (value: string) => value ? new Date(value).toLocaleDateString() : '-',
    },
    {
      title: 'Limits',
      key: 'limits',
      render: (_: string, record: APIKey) => (
        <div>
          <div className="text-xs">RPM: {record.rpm || 0}</div>
          <div className="text-xs">Threads: {record.threads_limit || 0}</div>
        </div>
      ),
    },
    {
      title: 'Usage',
      key: 'usage',
      render: (_: string, record: APIKey) => (
        <div>
          <div className="text-xs">Requests: {record.request_count ? record.request_count.toLocaleString() : '0'}</div>
          {record.last_used && (
            <div className="text-xs">
              Last: {new Date(record.last_used).toLocaleDateString()}
            </div>
          )}
        </div>
      ),
    },
    {
      title: 'Tags',
      key: 'tags',
      render: (_: string, record: APIKey) => (
        <div className="flex flex-wrap gap-1">
          {record.tags && record.tags.length > 0 ? (
            record.tags.map((tag, idx) => (
              <span
                key={idx}
                className="px-2 py-0.5 text-xs rounded-full truncate max-w-xs"
                style={{
                  backgroundColor: tag.color || '#e5e7eb',
                  color: '#1f2937',
                }}
              >
                {tag.name}
              </span>
            ))
          ) : (
            <span className="text-xs text-gray-400">No tags</span>
          )}
        </div>
      ),
    },
  ];

  return (
    <div className="container mx-auto px-4">
      <motion.div
        initial={{ opacity: 0, y: 20 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.3 }}
      >
        <div className="flex flex-col md:flex-row justify-between items-start md:items-center mb-8 gap-4">
          <h1 className="text-2xl md:text-3xl font-bold text-gray-900 dark:text-white">Manage API Keys</h1>
          
          {selectedKeys.length > 0 ? (
            <Button 
              variant="primary" 
              onClick={() => setShowBulkActions(true)}
              className="flex items-center"
            >
              <svg className="h-5 w-5 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 6h16M4 12h16m-7 6h7" />
              </svg>
              Bulk Actions ({selectedKeys.length})
            </Button>
          ) : null}
        </div>

        <Card className="mb-8 p-4 shadow-sm">
          <div className="flex flex-col md:flex-row md:items-end gap-4">
            <form onSubmit={handleSearch} className="flex-grow">
              <Input
                label="Search"
                placeholder="Search by name or key..."
                value={searchTerm}
                onChange={(e) => setSearchTerm(e.target.value)}
                className="relative"
              />
            </form>
            
            <div className="w-full md:w-48">
              <Select
                label="Status"
                value={statusFilter}
                onChange={handleStatusChange}
                options={[
                  { value: 'all', label: 'All' },
                  { value: 'active', label: 'Active' },
                  { value: 'inactive', label: 'Inactive' },
                  { value: 'expired', label: 'Expired' },
                ]}
              />
            </div>
            
            <Button type="submit" variant="outline" onClick={() => fetchKeys()}>
              <svg className="h-4 w-4 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M3 4a1 1 0 011-1h16a1 1 0 011 1v2.586a1 1 0 01-.293.707l-6.414 6.414a1 1 0 00-.293.707V17l-4 4v-6.586a1 1 0 00-.293-.707L3.293 7.293A1 1 0 013 6.586V4z" />
              </svg>
              Filter
            </Button>
          </div>
        </Card>

        {error && (
          <Alert
            type="error"
            message={error}
            onClose={() => setError(null)}
            className="mb-4"
          />
        )}

        <div className="table-responsive bg-white dark:bg-gray-800 rounded-lg shadow-sm overflow-hidden mb-4">
          <div className="flex items-center justify-between p-4 border-b border-gray-200 dark:border-gray-700">
            <div className="flex items-center">
              <input
                type="checkbox"
                className="h-4 w-4 text-blue-600 focus:ring-blue-500 border-gray-300 rounded"
                checked={selectedKeys.length === keys.length && keys.length > 0}
                onChange={handleSelectAll}
              />
              <span className="text-sm text-gray-600 dark:text-gray-400 ml-2">
                {selectedKeys.length === 0 
                  ? 'Select All'
                  : `Selected ${selectedKeys.length} of ${keys.length}`}
              </span>
            </div>
            
            <button 
              onClick={handleRefresh}
              className="flex items-center text-sm text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300"
              disabled={isRefreshing}
            >
              <svg 
                className={`h-4 w-4 mr-1 ${isRefreshing ? 'animate-spin' : ''}`} 
                fill="none" 
                stroke="currentColor" 
                viewBox="0 0 24 24" 
                xmlns="http://www.w3.org/2000/svg"
              >
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
              </svg>
              {isRefreshing ? 'Refreshing...' : 'Refresh'}
            </button>
          </div>

          <Table
            columns={tableColumns}
            data={keys} 
            isLoading={loading}
            rowKey="key"
            onRowClick={(record) => handleKeyClick(record.key)}
            className="mb-0"
          />

          {keys.length === 0 && !loading && (
            <div className="flex flex-col items-center justify-center py-16 bg-gray-50 dark:bg-gray-800">
              <svg 
                className="h-16 w-16 text-gray-400 mb-4" 
                fill="none" 
                stroke="currentColor" 
                viewBox="0 0 24 24" 
                xmlns="http://www.w3.org/2000/svg"
              >
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z" />
              </svg>
              <p className="text-gray-500 dark:text-gray-400 mb-2">No API keys found</p>
              <p className="text-gray-400 dark:text-gray-500 text-sm mb-4">
                {searchTerm || statusFilter !== 'all' 
                  ? 'Try adjusting your search or filters' 
                  : 'Generate your first API key to get started'}
              </p>
              
              {(searchTerm || statusFilter !== 'all') && (
                <Button 
                  variant="outline" 
                  onClick={() => {
                    setSearchTerm('');
                    setStatusFilter('all');
                  }}
                  size="sm"
                >
                  Clear Filters
                </Button>
              )}
            </div>
          )}
        </div>

        <Pagination
          currentPage={currentPage}
          totalPages={totalPages}
          onPageChange={handlePageChange}
        />

        <KeyDetailsModal
          isOpen={showKeyDetails}
          onClose={() => setShowKeyDetails(false)}
          keyId={selectedKeyId}
          onUpdate={fetchKeys}
        />

        <BulkActionsModal
          isOpen={showBulkActions}
          onClose={() => setShowBulkActions(false)}
          selectedKeys={selectedKeys}
          onActionComplete={handleBulkActionComplete}
        />
      </motion.div>
    </div>
  );
};

export default ManagePage;