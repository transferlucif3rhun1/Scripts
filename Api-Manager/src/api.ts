import { APIKey, KeyTemplate, MonitoringStats, SystemLog, UserSettings } from './types';

// Improved API endpoint handling with better logging
const API_ENDPOINT = localStorage.getItem('api_endpoint') || 'http://localhost:8080';
const API_BASE = `${API_ENDPOINT}/api/v1`;

// Log the endpoint being used (helpful for debugging)
console.log('Using API endpoint:', API_ENDPOINT);
console.log('API base URL:', API_BASE);

// Cache mechanism with improved error handling
const cache = new Map<string, { data: any; timestamp: number }>();
const CACHE_TTL = 30 * 1000; // 30 seconds cache

const isCacheValid = (key: string): boolean => {
  const item = cache.get(key);
  if (!item) return false;
  return Date.now() - item.timestamp < CACHE_TTL;
};

// Enhanced fetch API with better error handling and logging
async function fetchAPI<T>(
  url: string,
  method: string = 'GET',
  body?: any,
  useCache: boolean = false
): Promise<T> {
  // Check cache for GET requests only
  const cacheKey = `${method}-${url}-${body ? JSON.stringify(body) : ''}`;
  if (method === 'GET' && useCache && isCacheValid(cacheKey)) {
    return cache.get(cacheKey)!.data as T;
  }

  const headers: HeadersInit = {
    'Content-Type': 'application/json',
  };

  const options: RequestInit = {
    method,
    headers,
    credentials: 'include',
    mode: 'cors', // Explicitly set CORS mode
  };

  if (body) {
    options.body = JSON.stringify(body);
  }

  const fullUrl = `${API_BASE}${url}`;
  console.log(`Making ${method} request to: ${fullUrl}`);

  try {
    const response = await fetch(fullUrl, options);
    console.log(`Response status from ${url}: ${response.status}`);
    
    // Improved error handling with response parsing
    if (!response.ok) {
      let errorMessage = 'Unknown error occurred';
      try {
        // Try to get the body content for debugging
        const responseText = await response.text();
        console.log(`Error response body: ${responseText}`);
        
        try {
          const errorData = JSON.parse(responseText);
          errorMessage = errorData.error || `HTTP error ${response.status}`;
        } catch (parseError) {
          // If it's not JSON, just use the text
          errorMessage = responseText || `HTTP error ${response.status}: ${response.statusText}`;
        }
      } catch (e) {
        errorMessage = `Failed to parse error response: ${response.statusText}`;
      }
      throw new Error(errorMessage);
    }

    // Try to read the response text first to debug
    const responseText = await response.text();
    console.log(`Response from ${url} (first 100 chars): ${responseText.substring(0, 100)}...`);
    
    // Parse as JSON
    let data;
    try {
      data = JSON.parse(responseText);
    } catch (error) {
      console.error('Failed to parse JSON response:', error);
      console.log('Full response text:', responseText);
      throw new Error('Invalid JSON response from server');
    }

    // Check if the response has the expected structure
    if (!data || typeof data !== 'object') {
      console.error('Unexpected response format:', data);
      throw new Error('Unexpected response format from server');
    }

    // Cache successful GET requests
    if (method === 'GET' && useCache) {
      cache.set(cacheKey, { data: data.data, timestamp: Date.now() });
    }

    // Provide a default empty object/array if data.data is undefined
    if (data.data === undefined) {
      console.warn(`API response for ${url} has no data property, returning empty default`);
      
      // Try to infer the appropriate default value based on the expected return type
      if (url.includes('/keys?')) {
        return { data: [], page: 1, page_size: 0, total_items: 0, total_pages: 1 } as any as T;
      } else if (url.includes('/templates')) {
        return [] as any as T;
      } else if (url.includes('/monitoring/stats')) {
        return {
          total_keys: 0,
          active_keys: 0,
          expiring_keys: 0,
          total_requests_24h: 0,
          error_rate_24h: 0,
          avg_response_time: 0,
          last_updated: new Date().toISOString(),
          system_status: 'unknown',
          database_connected: false
        } as any as T;
      } else if (url.includes('/monitoring/logs')) {
        return [] as any as T;
      } else {
        return {} as T;
      }
    }

    return data.data as T;
  } catch (error) {
    console.error(`API request failed for ${url}:`, error);
    throw error;
  }
}

// API Keys functions with error handling and defaults
export const listApiKeys = (page: number = 1, pageSize: number = 20, filters: any = {}) => {
  const queryParams = new URLSearchParams();
  queryParams.append('page', page.toString());
  queryParams.append('pageSize', pageSize.toString());
  
  if (filters.status) queryParams.append('status', filters.status);
  if (filters.search) queryParams.append('search', filters.search);
  if (filters.tag) queryParams.append('tag', filters.tag);
  if (filters.sortField) queryParams.append('sortField', filters.sortField);
  if (filters.sortOrder) queryParams.append('sortOrder', filters.sortOrder);
  
  return fetchAPI<{ data: APIKey[], page: number, page_size: number, total_items: number, total_pages: number }>(
    `/keys?${queryParams.toString()}`
  ).catch(error => {
    console.error('Failed to list API keys:', error);
    // Return default empty response
    return {
      data: [],
      page: page,
      page_size: pageSize,
      total_items: 0,
      total_pages: 1
    };
  });
};

export const getApiKey = (id: string) => 
    fetchAPI<APIKey>(`/keys/${id}`, 'GET', undefined, true)
      .catch(error => {
        console.error(`Failed to get API key ${id}:`, error);
        return {
          key: id,
          name: undefined,  // Explicitly set as undefined to match optional type
          expiration: new Date().toISOString(),
          rpm: 0,
          threads_limit: 0,
          total_requests: 0,
          active: false,
          created: new Date().toISOString(),
          request_count: 0,
          tags: []
        } as APIKey;  // Add type assertion to ensure it matches APIKey interface
      });

export const updateApiKey = (id: string, data: Partial<APIKey>) => 
  fetchAPI<APIKey>(`/keys/${id}`, 'PUT', data);

export const deleteApiKey = (id: string) => 
  fetchAPI<{ message: string }>(`/keys/${id}`, 'DELETE');

export const generateApiKey = (params: Record<string, string>) => {
  const queryParams = new URLSearchParams();
  Object.entries(params).forEach(([key, value]) => {
    queryParams.append(key, value);
  });
  
  return fetchAPI<APIKey>(`/generate-key?${queryParams.toString()}`, 'GET');
};

// Bulk operations
export const bulkDeleteApiKeys = (keys: string[]) =>
  fetchAPI<{ success_count: number, error_count: number }>('/keys/bulk-delete', 'POST', { keys })
    .catch(error => {
      console.error('Failed to bulk delete keys:', error);
      return { success_count: 0, error_count: keys.length };
    });

export const bulkUpdateApiKeyTags = (keys: string[], addTags: any[] = [], delTags: string[] = []) =>
  fetchAPI<{ success_count: number, error_count: number }>('/keys/bulk-tags', 'POST', { keys, add_tags: addTags, del_tags: delTags })
    .catch(error => {
      console.error('Failed to bulk update tags:', error);
      return { success_count: 0, error_count: keys.length };
    });

export const bulkExtendApiKeys = (keys: string[], extension: string) =>
  fetchAPI<{ success_count: number, error_count: number, errors: string[] }>('/keys/bulk-extend', 'POST', { keys, extension })
    .catch(error => {
      console.error('Failed to bulk extend keys:', error);
      return { success_count: 0, error_count: keys.length, errors: ['Connection failed'] };
    });

// Templates
export const listTemplates = () => 
  fetchAPI<KeyTemplate[]>('/templates', 'GET', undefined, true)
    .catch(error => {
      console.error('Failed to list templates:', error);
      return [];
    });

export const getTemplate = (id: string) => 
  fetchAPI<KeyTemplate>(`/templates/${id}`, 'GET', undefined, true)
    .catch(error => {
      console.error(`Failed to get template ${id}:`, error);
      return {
        id,
        name: 'Error loading template',
        rpm: 0,
        threads_limit: 0,
        duration: '0h',
        is_default: false,
        created: new Date().toISOString(),
        last_modified: new Date().toISOString()
      };
    });

export const createTemplate = (template: Partial<KeyTemplate>) => 
  fetchAPI<KeyTemplate>('/templates', 'POST', template);

export const updateTemplate = (id: string, template: Partial<KeyTemplate>) => 
  fetchAPI<KeyTemplate>(`/templates/${id}`, 'PUT', template);

export const deleteTemplate = (id: string) => 
  fetchAPI<{ message: string }>(`/templates/${id}`, 'DELETE');

export const getDefaultTemplate = () => 
  fetchAPI<KeyTemplate | null>('/templates/default', 'GET', undefined, true)
    .catch(error => {
      console.error('Failed to get default template:', error);
      return null;
    });

// Settings
export const getSettings = (id: string = 'global') => {
  const queryParams = new URLSearchParams();
  queryParams.append('id', id);
  
  return fetchAPI<UserSettings>(`/settings?${queryParams.toString()}`, 'GET', undefined, true)
    .catch(error => {
      console.error(`Failed to get settings for ${id}:`, error);
      return {
        id,
        data: {},
        settingsVersion: 1,
        lastUpdated: new Date().toISOString()
      };
    });
};

export const saveSettings = (settings: Partial<UserSettings>) => 
  fetchAPI<UserSettings>('/settings', 'POST', settings);

export const deleteSettings = (id: string) => 
  fetchAPI<{ message: string }>(`/settings/${id}`, 'DELETE');

// Monitoring - Enhanced error handling for critical endpoints
export const getMonitoringStats = () => {
  console.log('Fetching monitoring stats...');
  return fetchAPI<MonitoringStats>('/monitoring/stats', 'GET')
    .catch(error => {
      console.error('Failed to fetch monitoring stats:', error);
      // Return default stats object instead of propagating the error
      return {
        total_keys: 0,
        active_keys: 0,
        expiring_keys: 0,
        total_requests_24h: 0,
        error_rate_24h: 0,
        avg_response_time: 0,
        last_updated: new Date().toISOString(),
        system_status: 'offline',
        database_connected: false
      };
    });
};

export const getLogs = (filters: any = {}) => {
  const queryParams = new URLSearchParams();
  
  if (filters.level) queryParams.append('level', filters.level);
  if (filters.component) queryParams.append('component', filters.component);
  if (filters.limit) queryParams.append('limit', filters.limit.toString());
  if (filters.since) queryParams.append('since', filters.since);
  if (filters.search) queryParams.append('search', filters.search);
  
  return fetchAPI<SystemLog[]>(`/monitoring/logs?${queryParams.toString()}`, 'GET')
    .catch(error => {
      console.error('Failed to fetch logs:', error);
      return []; // Return empty array instead of propagating the error
    });
};

export const clearLogs = () => 
  fetchAPI<{ message: string }>('/monitoring/logs', 'DELETE')
    .catch(error => {
      console.error('Failed to clear logs:', error);
      return { message: 'Failed to clear logs: Connection error' };
    });

// Enhanced health check with better error handling
export const healthCheck = async () => {
  console.log('Performing health check to', `${API_BASE}/monitoring/health`);
  try {
    return await fetchAPI<{ status: string, version: string, dbConnected: boolean, timestamp: string }>(
      '/monitoring/health', 'GET'
    );
  } catch (error) {
    console.error('Health check failed:', error);
    throw error;
  }
};

// Utility to invalidate cache for specific endpoints
export const invalidateCache = (keyPattern: string = '') => {
  if (!keyPattern) {
    console.log('Clearing entire cache');
    cache.clear();
    return;
  }
  
  console.log(`Clearing cache for pattern: ${keyPattern}`);
  Array.from(cache.keys()).forEach(key => {
    if (key.includes(keyPattern)) {
      cache.delete(key);
    }
  });
};

// API endpoint management with improved logging
export const setApiEndpoint = (endpoint: string) => {
  console.log('Setting API endpoint to:', endpoint);
  localStorage.setItem('api_endpoint', endpoint);
  window.location.reload();
};

export const getApiEndpoint = () => {
  return localStorage.getItem('api_endpoint') || API_ENDPOINT;
};