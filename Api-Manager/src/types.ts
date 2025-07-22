// API Response format
export interface ApiResponse<T = any> {
    success: boolean;
    data?: T;
    error?: string;
    meta?: any;
  }
  
  // Tag type for API keys and templates
  export interface Tag {
    name: string;
    color?: string;
  }
  
  // API Key definition
  export interface APIKey {
    key: string;
    name?: string;  // Make sure name is optional with question mark
    expiration: string;
    rpm: number;
    threads_limit: number;
    total_requests: number;
    active: boolean;
    created: string;
    last_used?: string;
    request_count: number;
    tags?: Tag[];
    valid?: boolean;
    expired?: boolean;
  }
  
  // Key template for faster generation
  export interface KeyTemplate {
    id?: string;
    name: string;
    description?: string;
    rpm: number;
    threads_limit: number;
    duration: string;
    tags?: Tag[];
    is_default: boolean;
    created: string;
    last_modified: string;
  }
  
  // System log entry
  export interface SystemLog {
    id?: string;
    timestamp: string;
    level: 'debug' | 'info' | 'warning' | 'error';
    component: string;
    message: string;
    details?: string;
  }
  
  // User settings type
  export interface UserSettings {
    id?: string;
    data: any;
    settingsVersion: number;
    lastUpdated: string;
  }
  
  // System monitoring stats
  export interface MonitoringStats {
    total_keys: number;
    active_keys: number;
    expiring_keys: number;
    total_requests_24h: number;
    error_rate_24h: number;
    avg_response_time: number;
    last_updated: string;
    system_status: string;
    database_connected: boolean;
  }
  
  // Bulk operation types
  export interface BulkOperationRequest {
    keys: string[];
  }
  
  export interface BulkTagRequest extends BulkOperationRequest {
    add_tags?: Tag[];
    del_tags?: string[];
  }
  
  export interface BulkExtendRequest extends BulkOperationRequest {
    extension: string;
  }
  
  export interface BulkOperationResponse {
    success_count: number;
    error_count: number;
    errors?: string[];
  }
  
  // UI Theme settings
  export interface ThemeSettings {
    mode: 'light' | 'dark' | 'system';
    primaryColor: string;
    reduceAnimations: boolean;
    tableCompact: boolean;
  }
  
  // User interface preferences
  export interface UIPreferences {
    theme: ThemeSettings;
    defaultPageSize: number;
    sidebarCollapsed: boolean;
    dateFormat: string;
    defaultFilters: Record<string, any>;
  }