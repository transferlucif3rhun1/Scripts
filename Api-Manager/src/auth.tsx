import { createContext, useContext, useState, useEffect } from 'react';

interface User {
  username: string;
  isAdmin: boolean;
}

interface AuthContextType {
  user: User | null;
  login: (username: string, password: string) => Promise<void>;
  logout: () => void;
  isAuthenticated: boolean;
  isLoading: boolean;
  error: string | null;
}

// Default admin credentials - in a real app this would be handled securely
const DEFAULT_USERNAME = 'admin';
const DEFAULT_PASSWORD = 'admin123';

// Create the auth context
export const AuthContext = createContext<AuthContextType>({
  user: null,
  login: async () => {},
  logout: () => {},
  isAuthenticated: false,
  isLoading: true,
  error: null,
});

// Hook to use the auth context
export const useAuth = () => useContext(AuthContext);

// Auth provider component
export const AuthProvider = ({ children }: { children: React.ReactNode }) => {
  const [user, setUser] = useState<User | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Check if user is already logged in
  useEffect(() => {
    const storedUser = localStorage.getItem('user');
    if (storedUser) {
      try {
        setUser(JSON.parse(storedUser));
      } catch (e) {
        console.error('Failed to parse stored user:', e);
        localStorage.removeItem('user');
      }
    }
    setIsLoading(false);
  }, []);

  // Login function
  const login = async (username: string, password: string) => {
    setIsLoading(true);
    setError(null);
    
    try {
      // Simple validation - in real app would call an API
      if (username === DEFAULT_USERNAME && password === DEFAULT_PASSWORD) {
        const user = { username, isAdmin: true };
        localStorage.setItem('user', JSON.stringify(user));
        setUser(user);
      } else {
        throw new Error('Invalid username or password');
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Login failed');
      throw err;
    } finally {
      setIsLoading(false);
    }
  };

  // Logout function
  const logout = () => {
    localStorage.removeItem('user');
    setUser(null);
  };

  const value = {
    user,
    login,
    logout,
    isAuthenticated: !!user,
    isLoading,
    error,
  };

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
};

// Auth Guard HOC
export const RequireAuth = ({ children }: { children: React.ReactNode }) => {
  const { isAuthenticated, isLoading } = useAuth();
  
  if (isLoading) {
    return <div className="loading-screen">Loading authentication status...</div>;
  }
  
  if (!isAuthenticated) {
    window.location.href = '/login';
    return null;
  }
  
  return <>{children}</>;
};