import React from 'react';
import { Tag } from './types';

// Enhanced Button component with variants, sizes, and animations
export const Button: React.FC<{
  variant?: 'primary' | 'secondary' | 'danger' | 'success' | 'outline';
  size?: 'sm' | 'md' | 'lg';
  disabled?: boolean;
  className?: string;
  onClick?: () => void;
  children: React.ReactNode;
  type?: 'button' | 'submit' | 'reset';
  icon?: React.ReactNode;
  isLoading?: boolean;
}> = ({
  variant = 'primary',
  size = 'md',
  disabled = false,
  className = '',
  onClick,
  children,
  type = 'button',
  icon,
  isLoading = false,
}) => {
  const baseStyles = 'font-medium rounded transition-colors focus:outline-none focus:ring-2 focus:ring-offset-2 relative';
  
  const variantStyles = {
    primary: 'bg-blue-600 hover:bg-blue-700 text-white focus:ring-blue-500 dark:bg-blue-600 dark:hover:bg-blue-700',
    secondary: 'bg-gray-600 hover:bg-gray-700 text-white focus:ring-gray-500 dark:bg-gray-700 dark:hover:bg-gray-600',
    danger: 'bg-red-600 hover:bg-red-700 text-white focus:ring-red-500 dark:bg-red-600 dark:hover:bg-red-700',
    success: 'bg-green-600 hover:bg-green-700 text-white focus:ring-green-500 dark:bg-green-600 dark:hover:bg-green-700',
    outline: 'border border-gray-300 bg-white text-gray-700 hover:bg-gray-50 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-800 dark:text-gray-200 dark:hover:bg-gray-700',
  };
  
  const sizeStyles = {
    sm: 'px-2.5 py-1.5 text-xs',
    md: 'px-4 py-2 text-sm',
    lg: 'px-6 py-3 text-base',
  };
  
  const disabledStyles = disabled || isLoading ? 'opacity-60 cursor-not-allowed' : 'cursor-pointer';
  
  const buttonClass = `${baseStyles} ${variantStyles[variant]} ${sizeStyles[size]} ${disabledStyles} ${className}`;
  
  return (
    <button
      type={type}
      className={buttonClass}
      onClick={onClick}
      disabled={disabled || isLoading}
    >
      {isLoading ? (
        <div className="flex items-center justify-center">
          <svg className="animate-spin -ml-1 mr-2 h-4 w-4 text-current" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
            <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
            <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
          </svg>
          {children}
        </div>
      ) : (
        <div className="flex items-center justify-center">
          {icon && <span className="mr-2">{icon}</span>}
          {children}
        </div>
      )}
    </button>
  );
};

// Enhanced Card component with animations and custom header/footer
export const Card: React.FC<{
  title?: string;
  className?: string;
  children: React.ReactNode;
  headerRight?: React.ReactNode;
  footer?: React.ReactNode;
  isLoading?: boolean;
  hoverEffect?: boolean;
}> = ({ title, className = '', children, headerRight, footer, isLoading = false, hoverEffect = false }) => {
  const cardClass = `bg-white dark:bg-gray-800 shadow-md rounded-lg overflow-hidden ${hoverEffect ? 'transition-all hover:shadow-lg hover:-translate-y-1' : ''} ${className}`;
  
  return (
    <div className={cardClass}>
      {title && (
        <div className="px-4 py-3 border-b border-gray-200 dark:border-gray-700 flex justify-between items-center">
          <h3 className="text-lg font-medium text-gray-900 dark:text-white">{title}</h3>
          {headerRight && <div>{headerRight}</div>}
        </div>
      )}
      
      <div className={`p-4 ${isLoading ? 'opacity-60' : ''}`}>
        {isLoading ? (
          <div className="flex justify-center items-center py-12">
            <svg className="animate-spin h-8 w-8 text-blue-500" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
              <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
              <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
            </svg>
          </div>
        ) : (
          children
        )}
      </div>
      
      {footer && (
        <div className="px-4 py-3 border-t border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-700">
          {footer}
        </div>
      )}
    </div>
  );
};

// Enhanced Input component with full features
export const Input: React.FC<{
  label?: string;
  type?: string;
  value: string;
  onChange: (e: React.ChangeEvent<HTMLInputElement>) => void;
  placeholder?: string;
  error?: string;
  required?: boolean;
  disabled?: boolean;
  className?: string;
  id?: string;
  name?: string;
  icon?: React.ReactNode;
  iconPosition?: 'left' | 'right';
  onIconClick?: () => void;
  autoComplete?: string;
  maxLength?: number;
  min?: number;
  max?: number;
  step?: number;
}> = ({
  label,
  type = 'text',
  value,
  onChange,
  placeholder = '',
  error,
  required = false,
  disabled = false,
  className = '',
  id,
  name,
  icon,
  iconPosition = 'left',
  onIconClick,
  autoComplete,
  maxLength,
  min,
  max,
  step,
}) => {
  const inputId = id || `input-${name || Math.random().toString(36).substring(2, 9)}`;
  const hasIcon = !!icon;
  const iconClickable = !!onIconClick;
  
  return (
    <div className={`mb-4 ${className}`}>
      {label && (
        <label htmlFor={inputId} className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
          {label} {required && <span className="text-red-500">*</span>}
        </label>
      )}
      
      <div className="relative">
        {hasIcon && iconPosition === 'left' && (
          <div className={`absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-${iconClickable ? 'auto' : 'none'} ${iconClickable ? 'cursor-pointer' : ''}`} onClick={onIconClick}>
            <span className="text-gray-500 dark:text-gray-400">{icon}</span>
          </div>
        )}
        
        <input
          id={inputId}
          name={name}
          type={type}
          value={value}
          onChange={onChange}
          placeholder={placeholder}
          disabled={disabled}
          required={required}
          autoComplete={autoComplete}
          maxLength={maxLength}
          min={min}
          max={max}
          step={step}
          className={`w-full ${hasIcon && iconPosition === 'left' ? 'pl-10' : ''} ${hasIcon && iconPosition === 'right' ? 'pr-10' : ''} px-3 py-2 border ${
            error ? 'border-red-500 focus:ring-red-500' : 'border-gray-300 dark:border-gray-600 focus:ring-blue-500'
          } rounded-md shadow-sm focus:outline-none focus:border-blue-500 dark:bg-gray-700 dark:text-white ${
            disabled ? 'bg-gray-100 dark:bg-gray-800 cursor-not-allowed' : ''
          } transition-colors`}
        />
        
        {hasIcon && iconPosition === 'right' && (
          <div className={`absolute inset-y-0 right-0 pr-3 flex items-center pointer-events-${iconClickable ? 'auto' : 'none'} ${iconClickable ? 'cursor-pointer' : ''}`} onClick={onIconClick}>
            <span className="text-gray-500 dark:text-gray-400">{icon}</span>
          </div>
        )}
      </div>
      
      {error && <p className="mt-1 text-sm text-red-500 dark:text-red-400">{error}</p>}
    </div>
  );
};

// Enhanced Select component
export const Select: React.FC<{
  label?: string;
  value: string;
  onChange: (e: React.ChangeEvent<HTMLSelectElement>) => void;
  options: Array<{ value: string; label: string }>;
  error?: string;
  required?: boolean;
  disabled?: boolean;
  className?: string;
  id?: string;
  name?: string;
  placeholder?: string;
}> = ({
  label,
  value,
  onChange,
  options,
  error,
  required = false,
  disabled = false,
  className = '',
  id,
  name,
  placeholder,
}) => {
  const selectId = id || `select-${name || Math.random().toString(36).substring(2, 9)}`;
  
  return (
    <div className={`mb-4 ${className}`}>
      {label && (
        <label htmlFor={selectId} className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
          {label} {required && <span className="text-red-500">*</span>}
        </label>
      )}
      
      <div className="relative">
        <select
          id={selectId}
          name={name}
          value={value}
          onChange={onChange}
          disabled={disabled}
          required={required}
          className={`block w-full pl-3 pr-10 py-2 text-base border ${
            error ? 'border-red-500 focus:ring-red-500' : 'border-gray-300 dark:border-gray-600 focus:ring-blue-500'
          } rounded-md focus:outline-none focus:border-blue-500 appearance-none bg-white dark:bg-gray-700 dark:text-white ${
            disabled ? 'bg-gray-100 dark:bg-gray-800 cursor-not-allowed' : ''
          } transition-colors`}
        >
          {placeholder && (
            <option value="" disabled>
              {placeholder}
            </option>
          )}
          {options.map((option) => (
            <option key={option.value} value={option.value}>
              {option.label}
            </option>
          ))}
        </select>
        
        <div className="pointer-events-none absolute inset-y-0 right-0 flex items-center px-2 text-gray-500 dark:text-gray-400">
          <svg className="h-5 w-5" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" aria-hidden="true">
            <path fillRule="evenodd" d="M5.293 7.293a1 1 0 011.414 0L10 10.586l3.293-3.293a1 1 0 111.414 1.414l-4 4a1 1 0 01-1.414 0l-4-4a1 1 0 010-1.414z" clipRule="evenodd" />
          </svg>
        </div>
      </div>
      
      {error && <p className="mt-1 text-sm text-red-500 dark:text-red-400">{error}</p>}
    </div>
  );
};

// Enhanced Badge component with more variants
export const Badge: React.FC<{
  variant?: 'default' | 'success' | 'warning' | 'danger' | 'info' | 'primary';
  className?: string;
  children: React.ReactNode;
  size?: 'sm' | 'md' | 'lg';
  dot?: boolean;
}> = ({ variant = 'default', className = '', children, size = 'md', dot = false }) => {
  const variantStyles = {
    default: 'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-300',
    primary: 'bg-blue-100 text-blue-800 dark:bg-blue-800 dark:text-blue-100',
    success: 'bg-green-100 text-green-800 dark:bg-green-800 dark:text-green-100',
    warning: 'bg-yellow-100 text-yellow-800 dark:bg-yellow-800 dark:text-yellow-100',
    danger: 'bg-red-100 text-red-800 dark:bg-red-800 dark:text-red-100',
    info: 'bg-blue-100 text-blue-800 dark:bg-blue-800 dark:text-blue-100',
  };
  
  const sizeStyles = {
    sm: 'px-2 py-0.5 text-xs',
    md: 'px-2.5 py-0.5 text-xs',
    lg: 'px-3 py-1 text-sm',
  };
  
  return (
    <span
      className={`inline-flex items-center font-medium rounded-full ${variantStyles[variant]} ${sizeStyles[size]} ${className}`}
    >
      {dot && (
        <span className={`mr-1.5 h-2 w-2 rounded-full ${variant === 'default' ? 'bg-gray-400' : `bg-${variant}-500`}`}></span>
      )}
      {children}
    </span>
  );
};

// Enhanced Modal component with animations
export const Modal: React.FC<{
  isOpen: boolean;
  onClose: () => void;
  title: string;
  children: React.ReactNode;
  footer?: React.ReactNode;
  size?: 'sm' | 'md' | 'lg' | 'xl' | 'full';
  closeOnClickOutside?: boolean;
}> = ({ isOpen, onClose, title, children, footer, size = 'md', closeOnClickOutside = true }) => {
  if (!isOpen) return null;
  
  const handleBackdropClick = (e: React.MouseEvent) => {
    if (closeOnClickOutside && e.target === e.currentTarget) {
      onClose();
    }
  };
  
  const sizeClasses = {
    sm: 'max-w-sm',
    md: 'max-w-md',
    lg: 'max-w-2xl',
    xl: 'max-w-4xl',
    full: 'max-w-full mx-4',
  };
  
  return (
    <div 
      className="fixed inset-0 z-50 overflow-y-auto backdrop-blur-sm bg-black bg-opacity-40 dark:bg-opacity-60 transition-opacity duration-300" 
      aria-labelledby="modal-title" 
      role="dialog" 
      aria-modal="true"
      onClick={handleBackdropClick}
    >
      <div className="flex items-end justify-center min-h-screen pt-4 px-4 pb-20 text-center sm:block sm:p-0">
        <span className="hidden sm:inline-block sm:align-middle sm:h-screen" aria-hidden="true">&#8203;</span>
        <div 
          className={`inline-block align-bottom bg-white dark:bg-gray-800 rounded-lg text-left overflow-hidden shadow-xl transform transition-all sm:my-8 sm:align-middle ${sizeClasses[size]} w-full animate-scale-in`}
          role="dialog"
          aria-modal="true"
          aria-labelledby="modal-headline"
        >
          <div className="bg-white dark:bg-gray-800 px-4 pt-5 pb-4 sm:p-6 sm:pb-4">
            <div className="flex items-start justify-between">
              <h3 className="text-lg leading-6 font-medium text-gray-900 dark:text-white" id="modal-headline">
                {title}
              </h3>
              <button
                type="button"
                className="bg-white dark:bg-gray-800 rounded-md text-gray-400 hover:text-gray-500 dark:hover:text-gray-300 focus:outline-none focus:ring-2 focus:ring-blue-500"
                onClick={onClose}
              >
                <span className="sr-only">Close</span>
                <svg className="h-6 w-6" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor" aria-hidden="true">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </div>
            <div className="mt-4">{children}</div>
          </div>
          {footer && (
            <div className="bg-gray-50 dark:bg-gray-700 px-4 py-3 sm:px-6 flex items-center justify-end">
              {footer}
            </div>
          )}
        </div>
      </div>
    </div>
  );
};

// Enhanced Alert component
export const Alert: React.FC<{
  type?: 'info' | 'success' | 'warning' | 'error';
  message: string;
  onClose?: () => void;
  className?: string;
  showIcon?: boolean;
}> = ({ type = 'info', message, onClose, className = '', showIcon = true }) => {
  const typeClasses = {
    info: 'bg-blue-50 text-blue-800 dark:bg-blue-900 dark:text-blue-100 border-blue-500',
    success: 'bg-green-50 text-green-800 dark:bg-green-900 dark:text-green-100 border-green-500',
    warning: 'bg-yellow-50 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-100 border-yellow-500',
    error: 'bg-red-50 text-red-800 dark:bg-red-900 dark:text-red-100 border-red-500',
  };
  
  const icons = {
    info: (
      <svg className="h-5 w-5 text-blue-500 dark:text-blue-400" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor">
        <path fillRule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a1 1 0 000 2v3a1 1 0 001 1h1a1 1 0 100-2v-3a1 1 0 00-1-1H9z" clipRule="evenodd" />
      </svg>
    ),
    success: (
      <svg className="h-5 w-5 text-green-500 dark:text-green-400" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor">
        <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" />
      </svg>
    ),
    warning: (
      <svg className="h-5 w-5 text-yellow-500 dark:text-yellow-400" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor">
        <path fillRule="evenodd" d="M8.257 3.099c.765-1.36 2.722-1.36 3.486 0l5.58 9.92c.75 1.334-.213 2.98-1.742 2.98H4.42c-1.53 0-2.493-1.646-1.743-2.98l5.58-9.92zM11 13a1 1 0 11-2 0 1 1 0 012 0zm-1-8a1 1 0 00-1 1v3a1 1 0 002 0V6a1 1 0 00-1-1z" clipRule="evenodd" />
      </svg>
    ),
    error: (
      <svg className="h-5 w-5 text-red-500 dark:text-red-400" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor">
        <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clipRule="evenodd" />
      </svg>
    ),
  };
  
  return (
    <div className={`rounded-md p-4 border-l-4 animate-fade-in ${typeClasses[type]} ${className}`} role="alert">
      <div className="flex">
        {showIcon && <div className="flex-shrink-0 mr-3">{icons[type]}</div>}
        <div className="flex-1">
          <p className="text-sm">{message}</p>
        </div>
        {onClose && (
          <div className="pl-3">
            <button
              type="button"
              className={`inline-flex rounded-md focus:outline-none ${
                type === 'info' ? 'text-blue-500 hover:bg-blue-100' :
                type === 'success' ? 'text-green-500 hover:bg-green-100' :
                type === 'warning' ? 'text-yellow-500 hover:bg-yellow-100' :
                'text-red-500 hover:bg-red-100'
              } dark:hover:bg-opacity-20 p-1.5 transition-colors`}
              onClick={onClose}
              aria-label="Dismiss"
            >
              <span className="sr-only">Dismiss</span>
              <svg className="h-5 w-5" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor">
                <path fillRule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clipRule="evenodd" />
              </svg>
            </button>
          </div>
        )}
      </div>
    </div>
  );
};

// Enhanced Table component with more features
export const Table: React.FC<{
  columns: Array<{
    title: string;
    key: string;
    render?: (value: any, record: any) => React.ReactNode;
    width?: string;
    sortable?: boolean;
  }>;
  data: any[];
  isLoading?: boolean;
  rowKey: string;
  onRowClick?: (record: any) => void;
  className?: string;
  emptyText?: string;
  selectable?: boolean;
  selectedRows?: string[];
  onSelectChange?: (selectedRowKeys: string[]) => void;
  sortOrder?: { column: string; direction: 'asc' | 'desc' };
  onSort?: (column: string) => void;
}> = ({ 
  columns, 
  data, 
  isLoading = false, 
  rowKey, 
  onRowClick, 
  className = '',
  emptyText = 'No data available',
  selectable = false,
  selectedRows = [],
  onSelectChange,
  sortOrder,
  onSort
}) => {
  // Handle row selection
  const handleSelectRow = (e: React.ChangeEvent<HTMLInputElement>, record: any) => {
    e.stopPropagation();
    if (!onSelectChange) return;
    
    const recordKey = record[rowKey];
    const newSelectedRows = [...selectedRows];
    
    if (e.target.checked) {
      if (!newSelectedRows.includes(recordKey)) {
        newSelectedRows.push(recordKey);
      }
    } else {
      const index = newSelectedRows.indexOf(recordKey);
      if (index !== -1) {
        newSelectedRows.splice(index, 1);
      }
    }
    
    onSelectChange(newSelectedRows);
  };
  
  // Handle select all
  const handleSelectAll = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (!onSelectChange) return;
    
    if (e.target.checked) {
      const allKeys = data.map(record => record[rowKey]);
      onSelectChange(allKeys);
    } else {
      onSelectChange([]);
    }
  };
  
  // Construct the columns array with selection column if needed
  const tableColumns = [
    ...(selectable ? [{
      title: (
        <input
          type="checkbox"
          checked={data.length > 0 && selectedRows.length === data.length}
          onChange={handleSelectAll}
          className="h-4 w-4 text-blue-600 focus:ring-blue-500 border-gray-300 rounded"
        />
      ),
      key: 'selection',
      width: '40px',
      render: (_: any, record: any) => (
        <input
          type="checkbox"
          checked={selectedRows.includes(record[rowKey])}
          onChange={(e) => handleSelectRow(e, record)}
          onClick={(e) => e.stopPropagation()}
          className="h-4 w-4 text-blue-600 focus:ring-blue-500 border-gray-300 rounded cursor-pointer"
        />
      ),
    }] : []),
    ...columns
  ];

  // Skeleton loading
  if (isLoading) {
    return (
      <div className={`bg-white dark:bg-gray-800 overflow-hidden rounded-md border border-gray-200 dark:border-gray-700 ${className}`}>
        <div className="overflow-x-auto">
          <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
            <thead>
              <tr>
                {tableColumns.map((column, index) => (
                  <th 
                    key={index}
                    scope="col"
                    className="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider bg-gray-50 dark:bg-gray-900"
                    style={{ width: column.width }}
                  >
                    {typeof column.title === 'string' ? column.title : column.title}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody className="bg-white dark:bg-gray-800 divide-y divide-gray-200 dark:divide-gray-700">
              {[...Array(3)].map((_, rowIndex) => (
                <tr key={rowIndex}>
                  {tableColumns.map((_, colIndex) => (
                    <td key={colIndex} className="px-6 py-4 whitespace-nowrap">
                      <div className="skeleton h-4 w-24 rounded"></div>
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    );
  }
  
  // Empty state
  if (data.length === 0) {
    return (
      <div className={`bg-white dark:bg-gray-800 overflow-hidden rounded-md border border-gray-200 dark:border-gray-700 ${className}`}>
        <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
          <thead>
            <tr>
              {tableColumns.map((column, index) => (
                <th 
                  key={index}
                  scope="col"
                  className="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider bg-gray-50 dark:bg-gray-900"
                  style={{ width: column.width }}
                >
                  {typeof column.title === 'string' ? column.title : column.title}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            <tr>
              <td 
                colSpan={tableColumns.length} 
                className="px-6 py-10 text-center text-sm text-gray-500 dark:text-gray-400"
              >
                {emptyText}
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    );
  }
  
  // Regular table with data
  return (
    <div className={`bg-white dark:bg-gray-800 overflow-hidden rounded-md border border-gray-200 dark:border-gray-700 ${className}`}>
      <div className="overflow-x-auto">
        <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
          <thead>
            <tr>
              {tableColumns.map((column, index) => (
                <th 
                  key={index}
                  scope="col"
                  className={`px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider bg-gray-50 dark:bg-gray-900 ${column.sortable ? 'cursor-pointer select-none' : ''}`}
                  style={{ width: column.width }}
                  onClick={() => column.sortable && onSort && onSort(column.key)}
                >
                  <div className="flex items-center">
                    {typeof column.title === 'string' ? column.title : column.title}
                    {column.sortable && sortOrder && sortOrder.column === column.key && (
                      <span className="ml-1">
                        {sortOrder.direction === 'asc' ? (
                          <svg className="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 15l7-7 7 7" />
                          </svg>
                        ) : (
                          <svg className="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
                          </svg>
                        )}
                      </span>
                    )}
                  </div>
                </th>
              ))}
            </tr>
          </thead>
          <tbody className="bg-white dark:bg-gray-800 divide-y divide-gray-200 dark:divide-gray-700">
            {data.map((record) => (
              <tr 
                key={record[rowKey]} 
                onClick={onRowClick ? () => onRowClick(record) : undefined}
                className={`transition-colors ${onRowClick ? 'cursor-pointer hover:bg-gray-50 dark:hover:bg-gray-700' : ''}`}
              >
                {tableColumns.map((column, index) => (
                  <td key={index} className="px-6 py-4 whitespace-nowrap text-sm text-gray-500 dark:text-gray-400">
                    {column.render
                      ? column.render(record[column.key], record)
                      : record[column.key] !== undefined
                      ? String(record[column.key])
                      : '-'}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
};

// Enhanced Pagination component
export const Pagination: React.FC<{
  currentPage: number;
  totalPages: number;
  onPageChange: (page: number) => void;
  className?: string;
  showTotal?: boolean;
  totalItems?: number;
  pageSize?: number;
}> = ({ 
  currentPage, 
  totalPages, 
  onPageChange, 
  className = '',
  showTotal = false,
  totalItems = 0,
  pageSize = 10 
}) => {
  const pages = [];
  const maxVisiblePages = 5;
  
  let startPage = Math.max(1, currentPage - Math.floor(maxVisiblePages / 2));
  let endPage = Math.min(totalPages, startPage + maxVisiblePages - 1);
  
  if (endPage - startPage + 1 < maxVisiblePages) {
    startPage = Math.max(1, endPage - maxVisiblePages + 1);
  }
  
  for (let i = startPage; i <= endPage; i++) {
    pages.push(i);
  }
  
  if (totalPages <= 1) {
    return null;
  }
  
  return (
    <div className={`flex flex-col sm:flex-row items-center justify-between py-3 gap-4 ${className}`}>
      {showTotal && (
        <div className="text-sm text-gray-700 dark:text-gray-300">
          Showing <span className="font-medium">{Math.min((currentPage - 1) * pageSize + 1, totalItems)}</span> to{' '}
          <span className="font-medium">{Math.min(currentPage * pageSize, totalItems)}</span> of{' '}
          <span className="font-medium">{totalItems}</span> results
        </div>
      )}
      
      <nav className="flex items-center space-x-1" aria-label="Pagination">
        <button
          onClick={() => onPageChange(Math.max(1, currentPage - 1))}
          disabled={currentPage === 1}
          className={`px-2 py-1 rounded-md text-sm font-medium ${
            currentPage === 1
              ? 'bg-gray-100 text-gray-400 cursor-not-allowed dark:bg-gray-700 dark:text-gray-500'
              : 'bg-white text-gray-700 hover:bg-gray-50 dark:bg-gray-800 dark:text-gray-300 dark:hover:bg-gray-700'
          } border border-gray-300 dark:border-gray-600 transition-colors`}
        >
          <span className="sr-only">Previous</span>
          <svg className="h-5 w-5" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" aria-hidden="true">
            <path fillRule="evenodd" d="M12.707 5.293a1 1 0 010 1.414L9.414 10l3.293 3.293a1 1 0 01-1.414 1.414l-4-4a1 1 0 010-1.414l4-4a1 1 0 011.414 0z" clipRule="evenodd" />
          </svg>
        </button>
        
        {startPage > 1 && (
          <>
            <button
              onClick={() => onPageChange(1)}
              className="hidden sm:block px-3 py-1 rounded-md text-sm font-medium bg-white text-gray-700 hover:bg-gray-50 dark:bg-gray-800 dark:text-gray-300 dark:hover:bg-gray-700 border border-gray-300 dark:border-gray-600 transition-colors"
            >
              1
            </button>
            {startPage > 2 && (
              <span className="px-2 py-1 text-gray-500 dark:text-gray-400">...</span>
            )}
          </>
        )}
        
        {pages.map((page) => (
          <button
            key={page}
            onClick={() => onPageChange(page)}
            className={`px-3 py-1 rounded-md text-sm font-medium ${
              page === currentPage
                ? 'bg-blue-600 text-white border-blue-600 dark:bg-blue-800 dark:border-blue-800'
                : 'bg-white text-gray-700 hover:bg-gray-50 dark:bg-gray-800 dark:text-gray-300 dark:hover:bg-gray-700 border-gray-300 dark:border-gray-600'
            } border transition-colors`}
          >
            {page}
          </button>
        ))}
        
        {endPage < totalPages && (
          <>
            {endPage < totalPages - 1 && (
              <span className="px-2 py-1 text-gray-500 dark:text-gray-400">...</span>
            )}
            <button
              onClick={() => onPageChange(totalPages)}
              className="hidden sm:block px-3 py-1 rounded-md text-sm font-medium bg-white text-gray-700 hover:bg-gray-50 dark:bg-gray-800 dark:text-gray-300 dark:hover:bg-gray-700 border border-gray-300 dark:border-gray-600 transition-colors"
            >
              {totalPages}
            </button>
          </>
        )}
        
        <button
          onClick={() => onPageChange(Math.min(totalPages, currentPage + 1))}
          disabled={currentPage === totalPages}
          className={`px-2 py-1 rounded-md text-sm font-medium ${
            currentPage === totalPages
              ? 'bg-gray-100 text-gray-400 cursor-not-allowed dark:bg-gray-700 dark:text-gray-500'
              : 'bg-white text-gray-700 hover:bg-gray-50 dark:bg-gray-800 dark:text-gray-300 dark:hover:bg-gray-700'
          } border border-gray-300 dark:border-gray-600 transition-colors`}
        >
          <span className="sr-only">Next</span>
          <svg className="h-5 w-5" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" aria-hidden="true">
            <path fillRule="evenodd" d="M7.293 14.707a1 1 0 010-1.414L10.586 10 7.293 6.707a1 1 0 011.414-1.414l4 4a1 1 0 010 1.414l-4 4a1 1 0 01-1.414 0z" clipRule="evenodd" />
          </svg>
        </button>
      </nav>
    </div>
  );
};

// Enhanced TagInput component
export const TagInput: React.FC<{
  value: Tag[];
  onChange: (tags: Tag[]) => void;
  label?: string;
  placeholder?: string;
  error?: string;
  className?: string;
  required?: boolean;
  disabled?: boolean;
  id?: string;
  maxTags?: number;
  creatable?: boolean;
  predefinedColors?: string[];
}> = ({ 
  value = [], 
  onChange, 
  label, 
  placeholder = 'Add a tag...', 
  error, 
  className = '',
  required = false,
  disabled = false,
  id,
  maxTags = 0,
  creatable = true,
  predefinedColors = ['#e5e7eb', '#fecaca', '#d1fae5', '#bfdbfe', '#ddd6fe', '#fca5a5', '#a7f3d0', '#93c5fd', '#c4b5fd']
}) => {
  const [input, setInput] = React.useState('');
  const inputRef = React.useRef<HTMLInputElement>(null);
  const inputId = id || `tag-input-${Math.random().toString(36).substring(2, 9)}`;
  
  const addTag = () => {
    const trimmedInput = input.trim();
    if (trimmedInput && (!maxTags || value.length < maxTags) && !value.some((tag) => tag.name === trimmedInput)) {
      const newTag: Tag = {
        name: trimmedInput,
        color: getRandomColor(),
      };
      onChange([...value, newTag]);
      setInput('');
    }
  };
  
  const removeTag = (tagToRemove: string) => {
    onChange(value.filter((tag) => tag.name !== tagToRemove));
  };
  
  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if ((e.key === 'Enter' || e.key === ',') && input.trim()) {
      e.preventDefault();
      addTag();
    } else if (e.key === 'Backspace' && !input && value.length > 0) {
      removeTag(value[value.length - 1].name);
    }
  };
  
  // Generate a random color from predefined colors
  const getRandomColor = () => {
    return predefinedColors[Math.floor(Math.random() * predefinedColors.length)];
  };
  
  // Focus the input when clicking on the container
  const handleContainerClick = () => {
    if (inputRef.current && !disabled) {
      inputRef.current.focus();
    }
  };
  
  return (
    <div className={`mb-4 ${className}`}>
      {label && (
        <label htmlFor={inputId} className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
          {label} {required && <span className="text-red-500">*</span>}
        </label>
      )}
      
      <div 
        className={`flex flex-wrap items-center gap-2 p-2 border ${
          error ? 'border-red-500' : 'border-gray-300 dark:border-gray-600 hover:border-gray-400 dark:hover:border-gray-500'
        } rounded-md focus-within:ring-2 focus-within:ring-blue-500 focus-within:border-blue-500 ${
          disabled ? 'bg-gray-100 dark:bg-gray-800 cursor-not-allowed' : 'bg-white dark:bg-gray-700'
        } transition-colors cursor-text`}
        onClick={handleContainerClick}
      >
        {value.map((tag) => (
          <div
            key={tag.name}
            className="flex items-center py-1 px-2 rounded-full text-sm text-gray-800 dark:text-gray-900"
            style={{ backgroundColor: tag.color || '#e5e7eb' }}
          >
            <span>{tag.name}</span>
            {!disabled && (
              <button
                type="button"
                className="ml-1.5 text-gray-500 hover:text-gray-700 focus:outline-none"
                onClick={(e) => {
                  e.stopPropagation();
                  removeTag(tag.name);
                }}
              >
                <svg className="h-3.5 w-3.5" fill="currentColor" viewBox="0 0 20 20">
                  <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clipRule="evenodd"/>
                </svg>
              </button>
            )}
          </div>
        ))}
        
        {creatable && (!maxTags || value.length < maxTags) && !disabled && (
          <input
            ref={inputRef}
            id={inputId}
            type="text"
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={handleKeyDown}
            onBlur={addTag}
            placeholder={value.length === 0 ? placeholder : ''}
            className="flex-grow min-w-[80px] px-1 py-0.5 bg-transparent outline-none text-gray-700 dark:text-white placeholder-gray-400 border-none focus:ring-0"
            disabled={disabled}
          />
        )}
      </div>
      
      {error && <p className="mt-1 text-sm text-red-500 dark:text-red-400">{error}</p>}
      
      {maxTags > 0 && (
        <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {value.length} of {maxTags} tags used
        </p>
      )}
    </div>
  );
};

// Spinner component for loading states
export const Spinner: React.FC<{
  size?: 'sm' | 'md' | 'lg';
  color?: 'primary' | 'white' | 'gray';
  className?: string;
}> = ({ size = 'md', color = 'primary', className = '' }) => {
  const sizeClasses = {
    sm: 'h-4 w-4',
    md: 'h-6 w-6',
    lg: 'h-8 w-8',
  };
  
  const colorClasses = {
    primary: 'text-blue-600 dark:text-blue-500',
    white: 'text-white',
    gray: 'text-gray-500 dark:text-gray-400',
  };
  
  return (
    <svg
      className={`animate-spin ${sizeClasses[size]} ${colorClasses[color]} ${className}`}
      xmlns="http://www.w3.org/2000/svg"
      fill="none"
      viewBox="0 0 24 24"
    >
      <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
      <path
        className="opacity-75"
        fill="currentColor"
        d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
      ></path>
    </svg>
  );
};

// EmptyState component for no data situations
export const EmptyState: React.FC<{
  title: string;
  description?: string;
  icon?: React.ReactNode;
  action?: React.ReactNode;
  className?: string;
}> = ({ title, description, icon, action, className = '' }) => {
  return (
    <div className={`text-center py-12 px-4 ${className}`}>
      {icon && (
        <div className="mx-auto h-16 w-16 text-gray-400 dark:text-gray-500 mb-4">
          {icon}
        </div>
      )}
      <h3 className="text-lg font-medium text-gray-900 dark:text-white mb-1">{title}</h3>
      {description && (
        <p className="text-sm text-gray-500 dark:text-gray-400 max-w-md mx-auto mb-6">{description}</p>
      )}
      {action && <div>{action}</div>}
    </div>
  );
};

// Common animation variants for Framer Motion
export const animations = {
  fadeIn: {
    initial: { opacity: 0 },
    animate: { opacity: 1 },
    exit: { opacity: 0 },
    transition: { duration: 0.3 },
  },
  slideUp: {
    initial: { opacity: 0, y: 20 },
    animate: { opacity: 1, y: 0 },
    exit: { opacity: 0, y: 20 },
    transition: { duration: 0.3, type: 'spring', stiffness: 400, damping: 30 },
  },
  slideDown: {
    initial: { opacity: 0, y: -20 },
    animate: { opacity: 1, y: 0 },
    exit: { opacity: 0, y: -20 },
    transition: { duration: 0.3, type: 'spring', stiffness: 400, damping: 30 },
  },
  slideIn: {
    initial: { opacity: 0, x: -20 },
    animate: { opacity: 1, x: 0 },
    exit: { opacity: 0, x: -20 },
    transition: { duration: 0.3, type: 'spring', stiffness: 400, damping: 30 },
  },
  slideInRight: {
    initial: { opacity: 0, x: 20 },
    animate: { opacity: 1, x: 0 },
    exit: { opacity: 0, x: 20 },
    transition: { duration: 0.3, type: 'spring', stiffness: 400, damping: 30 },
  },
  scale: {
    initial: { opacity: 0, scale: 0.95 },
    animate: { opacity: 1, scale: 1 },
    exit: { opacity: 0, scale: 0.95 },
    transition: { duration: 0.2, type: 'spring', stiffness: 400, damping: 30 },
  },
  pop: {
    initial: { opacity: 0, scale: 0.9 },
    animate: { opacity: 1, scale: 1 },
    exit: { opacity: 0, scale: 0.9 },
    transition: { type: 'spring', stiffness: 400, damping: 30 },
  },
};