import React, { useState, useEffect } from 'react';
import { Routes, Route, Navigate, useLocation, useNavigate } from 'react-router-dom';
import { AuthProvider, RequireAuth, useAuth } from './auth';
import { AnimatePresence, motion } from 'framer-motion';
import { animations } from './ui';
import { getApiEndpoint, healthCheck } from './api';

import LoginPage from './LoginPage';
import GeneratePage from './GeneratePage';
import ManagePage from './ManagePage';
import SettingsPage from './SettingsPage';

// Enhanced icons with consistent styling
const HomeIcon = () => (
  <svg className="w-5 h-5" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
    <path strokeLinecap="round" strokeLinejoin="round" d="M2.25 12l8.954-8.955c.44-.439 1.152-.439 1.591 0L21.75 12M4.5 9.75v10.125c0 .621.504 1.125 1.125 1.125H9.75v-4.875c0-.621.504-1.125 1.125-1.125h2.25c.621 0 1.125.504 1.125 1.125V21h4.125c.621 0 1.125-.504 1.125-1.125V9.75M8.25 21h8.25" />
  </svg>
);

const KeyIcon = () => (
  <svg className="w-5 h-5" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
    <path strokeLinecap="round" strokeLinejoin="round" d="M15.75 5.25a3 3 0 013 3m3 0a6 6 0 01-7.029 5.912c-.563-.097-1.159.026-1.563.43L10.5 17.25H8.25v2.25H6v2.25H2.25v-2.818c0-.597.237-1.17.659-1.591l6.499-6.499c.404-.404.527-1 .43-1.563A6 6 0 1121.75 8.25z" />
  </svg>
);

const SettingsIcon = () => (
  <svg className="w-5 h-5" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
    <path strokeLinecap="round" strokeLinejoin="round" d="M9.594 3.94c.09-.542.56-.94 1.11-.94h2.593c.55 0 1.02.398 1.11.94l.213 1.281c.063.374.313.686.645.87.074.04.147.083.22.127.324.196.72.257 1.075.124l1.217-.456a1.125 1.125 0 011.37.49l1.296 2.247a1.125 1.125 0 01-.26 1.431l-1.003.827c-.293.24-.438.613-.431.992a6.759 6.759 0 010 .255c-.007.378.138.75.43.99l1.005.828c.424.35.534.954.26 1.43l-1.298 2.247a1.125 1.125 0 01-1.369.491l-1.217-.456c-.355-.133-.75-.072-1.076.124a6.57 6.57 0 01-.22.128c-.331.183-.581.495-.644.869l-.213 1.28c-.09.543-.56.941-1.11.941h-2.594c-.55 0-1.02-.398-1.11-.94l-.213-1.281c-.062-.374-.312-.686-.644-.87a6.52 6.52 0 01-.22-.127c-.325-.196-.72-.257-1.076-.124l-1.217.456a1.125 1.125 0 01-1.369-.49l-1.297-2.247a1.125 1.125 0 01.26-1.431l1.004-.827c.292-.24.437-.613.43-.992a6.932 6.932 0 010-.255c.007-.378-.138-.75-.43-.99l-1.004-.828a1.125 1.125 0 01-.26-1.43l1.297-2.247a1.125 1.125 0 011.37-.491l1.216.456c.356.133.751.072 1.076-.124.072-.044.146-.087.22-.128.332-.183.582-.495.644-.869l.214-1.281z" />
    <path strokeLinecap="round" strokeLinejoin="round" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
  </svg>
);

const LogoutIcon = () => (
  <svg className="w-5 h-5" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
    <path strokeLinecap="round" strokeLinejoin="round" d="M15.75 9V5.25A2.25 2.25 0 0013.5 3h-6a2.25 2.25 0 00-2.25 2.25v13.5A2.25 2.25 0 007.5 21h6a2.25 2.25 0 002.25-2.25V15m3 0l3-3m0 0l-3-3m3 3H9" />
  </svg>
);

const CollapseSidebarIcon = ({ collapsed }: { collapsed: boolean }) => (
  <svg className="w-5 h-5" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
    {collapsed ? (
      <path strokeLinecap="round" strokeLinejoin="round" d="M11.25 4.5l7.5 7.5-7.5 7.5m-6-15l7.5 7.5-7.5 7.5" />
    ) : (
      <path strokeLinecap="round" strokeLinejoin="round" d="M18.75 19.5l-7.5-7.5 7.5-7.5m-6 15L5.25 12l7.5-7.5" />
    )}
  </svg>
);

const ThemeIcon = ({ isDark }: { isDark: boolean }) => (
  <svg className="w-5 h-5" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
    {isDark ? (
      <path strokeLinecap="round" strokeLinejoin="round" d="M12 3v2.25m6.364.386l-1.591 1.591M21 12h-2.25m-.386 6.364l-1.591-1.591M12 18.75V21m-4.773-4.227l-1.591 1.591M5.25 12H3m4.227-4.773L5.636 5.636M15.75 12a3.75 3.75 0 11-7.5 0 3.75 3.75 0 017.5 0z" />
    ) : (
      <path strokeLinecap="round" strokeLinejoin="round" d="M21.752 15.002A9.718 9.718 0 0118 15.75c-5.385 0-9.75-4.365-9.75-9.75 0-1.33.266-2.597.748-3.752A9.753 9.753 0 003 11.25C3 16.635 7.365 21 12.75 21a9.753 9.753 0 009.002-5.998z" />
    )}
  </svg>
);

const Layout: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const { logout } = useAuth();
  const location = useLocation();
  const navigate = useNavigate();
  
  // State for sidebar, theme, and connection
  const [collapsed, setCollapsed] = useState(() => {
    const stored = localStorage.getItem('sidebarCollapsed');
    return stored ? JSON.parse(stored) : window.innerWidth < 768;
  });
  
  const [isDarkTheme, setIsDarkTheme] = useState(() => {
    const stored = localStorage.getItem('theme');
    if (stored) return stored === 'dark';
    return window.matchMedia('(prefers-color-scheme: dark)').matches;
  });
  
  const [connectionStatus, setConnectionStatus] = useState<'connected' | 'disconnected' | 'checking'>('checking');
  const [lastChecked, setLastChecked] = useState<string | null>(null);
  
  // Handle theme change
  useEffect(() => {
    document.documentElement.classList.remove('light', 'dark');
    document.documentElement.classList.add(isDarkTheme ? 'dark' : 'light');
    localStorage.setItem('theme', isDarkTheme ? 'dark' : 'light');
  }, [isDarkTheme]);
  
  // Handle sidebar collapse state persistence
  useEffect(() => {
    localStorage.setItem('sidebarCollapsed', JSON.stringify(collapsed));
  }, [collapsed]);
  
  // Handle responsive sidebar
  useEffect(() => {
    const handleResize = () => {
      if (window.innerWidth < 640 && !collapsed) {
        setCollapsed(true);
      }
    };
    
    window.addEventListener('resize', handleResize);
    return () => window.removeEventListener('resize', handleResize);
  }, [collapsed]);

  // Check API connection status
  useEffect(() => {
    const checkConnection = async () => {
      try {
        setConnectionStatus('checking');
        await healthCheck();
        setConnectionStatus('connected');
        setLastChecked(new Date().toLocaleTimeString());
      } catch (error) {
        setConnectionStatus('disconnected');
        setLastChecked(new Date().toLocaleTimeString());
      }
    };
    
    checkConnection();
    const interval = setInterval(checkConnection, 30000);
    
    return () => clearInterval(interval);
  }, []);
  
  const navItems = [
    {
      key: 'generate',
      label: 'Generate Keys',
      icon: <KeyIcon />,
      onClick: () => navigate('/generate'),
      isActive: location.pathname === '/generate',
    },
    {
      key: 'manage',
      label: 'Manage Keys',
      icon: <HomeIcon />,
      onClick: () => navigate('/manage'),
      isActive: location.pathname === '/manage',
    },
    {
      key: 'settings',
      label: 'Settings',
      icon: <SettingsIcon />,
      onClick: () => navigate('/settings'),
      isActive: location.pathname === '/settings',
    },
    {
      key: 'logout',
      label: 'Logout',
      icon: <LogoutIcon />,
      onClick: logout,
    },
  ];
  
  const connectionStatusClasses = {
    connected: 'connection-status-connected',
    checking: 'connection-status-checking',
    disconnected: 'connection-status-disconnected',
  };
  
  const connectionStatusText = {
    connected: 'Connected',
    checking: 'Checking Connection...',
    disconnected: 'Disconnected',
  };
  
  return (
    <div className="flex min-h-screen bg-gray-50 dark:bg-gray-900 text-gray-900 dark:text-gray-100 transition-colors">
      {/* Sidebar - With improved animations and styling */}
      <aside
        className={`fixed z-20 h-full transition-all duration-300 ease-in-out transform ${
          collapsed ? 'w-16' : 'w-64'
        } bg-white dark:bg-gray-800 border-r border-gray-200 dark:border-gray-700 shadow-md`}
      >
        <div className="flex items-center justify-between h-16 px-4 border-b border-gray-200 dark:border-gray-700">
          {!collapsed && (
            <motion.h1 
              initial={{ opacity: 0 }} 
              animate={{ opacity: 1 }} 
              exit={{ opacity: 0 }}
              className="text-lg font-semibold text-gray-900 dark:text-white truncate"
            >
              API Key Manager
            </motion.h1>
          )}
          <button
            onClick={() => setCollapsed(!collapsed)}
            className="p-1.5 rounded-md text-gray-500 hover:text-gray-600 dark:text-gray-400 dark:hover:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 focus:outline-none focus:ring-2 focus:ring-blue-500"
            aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
          >
            <CollapseSidebarIcon collapsed={collapsed} />
          </button>
        </div>
        
        <nav className="flex-1 overflow-y-auto pt-2">
          <ul className="space-y-1 px-2">
            {navItems.map((item) => (
              <li key={item.key}>
                <button
                  onClick={item.onClick}
                  className={`w-full flex items-center px-3 py-2.5 rounded-lg text-left transition-colors ${
                    item.isActive
                      ? 'bg-blue-50 text-blue-700 dark:bg-blue-900/50 dark:text-blue-300'
                      : 'text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-700/50'
                  }`}
                >
                  <span className="inline-flex items-center justify-center">{item.icon}</span>
                  {!collapsed && (
                    <motion.span 
                      initial={{ opacity: 0 }} 
                      animate={{ opacity: 1 }} 
                      exit={{ opacity: 0 }}
                      transition={{ duration: 0.2 }}
                      className="ml-3 font-medium"
                    >
                      {item.label}
                    </motion.span>
                  )}
                </button>
              </li>
            ))}
          </ul>
        </nav>
        
        <div className="p-4 border-t border-gray-200 dark:border-gray-700 flex flex-col space-y-2">
          {/* Connection Status */}
          <div className="flex items-center">
            <div className={`w-2.5 h-2.5 rounded-full mr-2 ${connectionStatusClasses[connectionStatus]}`} />
            {!collapsed && (
              <motion.span 
                initial={{ opacity: 0 }} 
                animate={{ opacity: 1 }} 
                exit={{ opacity: 0 }}
                className="text-sm text-gray-600 dark:text-gray-400"
              >
                {connectionStatusText[connectionStatus]}
              </motion.span>
            )}
          </div>
          
          {/* Theme Toggle */}
          <button
            onClick={() => setIsDarkTheme(!isDarkTheme)}
            className={`${
              collapsed ? 'justify-center' : 'justify-between'
            } w-full flex items-center px-2 py-1.5 rounded-md text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors`}
            aria-label={isDarkTheme ? "Switch to light theme" : "Switch to dark theme"}
          >
            <ThemeIcon isDark={isDarkTheme} />
            {!collapsed && (
              <motion.span 
                initial={{ opacity: 0 }} 
                animate={{ opacity: 1 }} 
                exit={{ opacity: 0 }}
                className="text-sm"
              >
                {isDarkTheme ? 'Light Mode' : 'Dark Mode'}
              </motion.span>
            )}
          </button>
        </div>
      </aside>
      
      {/* Main Content - With dynamic spacing based on sidebar state */}
      <main 
        className={`flex-1 transition-all duration-300 ease-in-out ${
          collapsed ? 'ml-16' : 'ml-64'
        } pt-6 pb-12`}
      >
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
          <AnimatePresence mode="wait">
            <motion.div 
              key={location.pathname} 
              {...animations.fadeIn}
              className="h-full"
            >
              {children}
            </motion.div>
          </AnimatePresence>
        </div>
      </main>
    </div>
  );
};

function App() {
  return (
    <AuthProvider>
      <AnimatePresence mode="wait">
        <Routes>
          <Route path="/login" element={
            <motion.div key="login" {...animations.fadeIn}>
              <LoginPage />
            </motion.div>
          } />
          <Route
            path="/generate"
            element={
              <RequireAuth>
                <Layout>
                  <GeneratePage />
                </Layout>
              </RequireAuth>
            }
          />
          <Route
            path="/manage"
            element={
              <RequireAuth>
                <Layout>
                  <ManagePage />
                </Layout>
              </RequireAuth>
            }
          />
          <Route
            path="/settings"
            element={
              <RequireAuth>
                <Layout>
                  <SettingsPage />
                </Layout>
              </RequireAuth>
            }
          />
          <Route path="/" element={<Navigate to="/generate" replace />} />
        </Routes>
      </AnimatePresence>
    </AuthProvider>
  );
}

export default App;