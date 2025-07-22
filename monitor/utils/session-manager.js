class SessionManager {
  constructor() {
    this.sessions = new Map();
    this.sessionStorage = new Map();
    this.recoveryData = new Map();
    this.maxSessions = 5;
    this.sessionTimeout = 30 * 60 * 1000;
    
    this.setupPeriodicCleanup();
  }

  createSession(tabId, url, options = {}) {
    const sessionId = this.generateSessionId(tabId);
    
    const session = {
      id: sessionId,
      tabId: tabId,
      url: url,
      state: 'CREATED',
      createdAt: Date.now(),
      lastActivity: Date.now(),
      
      config: {
        enableGPUMonitoring: options.enableGPUMonitoring !== false,
        enableNetworkMonitoring: options.enableNetworkMonitoring !== false,
        enablePerformanceTracking: options.enablePerformanceTracking !== false,
        testTimeout: options.testTimeout || 30000,
        maxRetries: options.maxRetries || 3,
        ...options
      },

      data: {
        baseline: null,
        testResults: new Map(),
        networkRequests: [],
        consoleLogs: [],
        performanceMetrics: [],
        gpuMetrics: {},
        errors: []
      },

      progress: {
        currentPhase: 'INITIALIZING',
        currentTest: null,
        testQueue: options.testQueue || ['analytics', 'social-widgets', 'advertising', 'fonts', 'images', 'javascript', 'css'],
        currentIndex: 0,
        completedTests: [],
        skippedTests: []
      },

      metadata: {
        userAgent: null,
        viewport: null,
        deviceInfo: null,
        pageTitle: null
      }
    };

    this.sessions.set(sessionId, session);
    this.saveSessionData(sessionId, session);
    this.cleanupOldSessions();

    return session;
  }

  getSession(sessionId) {
    const session = this.sessions.get(sessionId);
    if (session) {
      session.lastActivity = Date.now();
      return session;
    }
    
    const savedSession = this.loadSessionData(sessionId);
    if (savedSession && this.isSessionValid(savedSession)) {
      this.sessions.set(sessionId, savedSession);
      return savedSession;
    }
    
    return null;
  }

  updateSession(sessionId, updates) {
    const session = this.getSession(sessionId);
    if (!session) {
      throw new Error('Session not found');
    }

    if (updates.state) {
      this.validateStateTransition(session.state, updates.state);
      session.state = updates.state;
    }

    if (updates.data) {
      Object.assign(session.data, updates.data);
    }

    if (updates.progress) {
      Object.assign(session.progress, updates.progress);
    }

    if (updates.metadata) {
      Object.assign(session.metadata, updates.metadata);
    }

    session.lastActivity = Date.now();
    this.saveSessionData(sessionId, session);

    return session;
  }

  deleteSession(sessionId) {
    const session = this.sessions.get(sessionId);
    if (session) {
      this.createRecoveryPoint(session);
      this.sessions.delete(sessionId);
      this.deleteSessionData(sessionId);
      return true;
    }
    return false;
  }

  getSessionsByTabId(tabId) {
    const sessions = [];
    for (const session of this.sessions.values()) {
      if (session.tabId === tabId) {
        sessions.push(session);
      }
    }
    return sessions;
  }

  getActiveSessions() {
    const activeSessions = [];
    for (const session of this.sessions.values()) {
      if (this.isSessionActive(session)) {
        activeSessions.push(session);
      }
    }
    return activeSessions;
  }

  pauseSession(sessionId) {
    const session = this.getSession(sessionId);
    if (!session) {
      throw new Error('Session not found');
    }

    if (session.state === 'RUNNING' || session.state === 'TESTING') {
      session.state = 'PAUSED';
      session.pausedAt = Date.now();
      this.saveSessionData(sessionId, session);
      return true;
    }
    
    return false;
  }

  resumeSession(sessionId) {
    const session = this.getSession(sessionId);
    if (!session) {
      throw new Error('Session not found');
    }

    if (session.state === 'PAUSED') {
      session.state = 'RUNNING';
      session.resumedAt = Date.now();
      if (session.pausedAt) {
        session.totalPauseTime = (session.totalPauseTime || 0) + (Date.now() - session.pausedAt);
        delete session.pausedAt;
      }
      this.saveSessionData(sessionId, session);
      return true;
    }
    
    return false;
  }

  recoverSession(sessionId) {
    const recoveryData = this.recoveryData.get(sessionId);
    if (!recoveryData) {
      throw new Error('No recovery data found');
    }

    const recoveredSession = this.createSession(
      recoveryData.tabId,
      recoveryData.url,
      recoveryData.config
    );

    recoveredSession.state = 'RECOVERED';
    recoveredSession.data = recoveryData.data;
    recoveredSession.progress = recoveryData.progress;
    recoveredSession.metadata = recoveryData.metadata;
    recoveredSession.recoveredFrom = sessionId;
    recoveredSession.recoveredAt = Date.now();

    this.recoveryData.delete(sessionId);
    return recoveredSession;
  }

  addTestResult(sessionId, testName, result) {
    const session = this.getSession(sessionId);
    if (!session) {
      throw new Error('Session not found');
    }

    session.data.testResults.set(testName, {
      ...result,
      timestamp: Date.now(),
      testName: testName
    });

    session.progress.completedTests.push(testName);
    session.progress.currentIndex++;

    this.saveSessionData(sessionId, session);
    return session;
  }

  addNetworkRequest(sessionId, request) {
    const session = this.getSession(sessionId);
    if (!session) return;

    session.data.networkRequests.push({
      ...request,
      sessionTimestamp: Date.now() - session.createdAt
    });

    if (session.data.networkRequests.length > 1000) {
      session.data.networkRequests = session.data.networkRequests.slice(-500);
    }

    this.saveSessionData(sessionId, session);
  }

  addConsoleLog(sessionId, logEntry) {
    const session = this.getSession(sessionId);
    if (!session) return;

    session.data.consoleLogs.push({
      ...logEntry,
      sessionTimestamp: Date.now() - session.createdAt
    });

    if (session.data.consoleLogs.length > 500) {
      session.data.consoleLogs = session.data.consoleLogs.slice(-250);
    }

    this.saveSessionData(sessionId, session);
  }

  addPerformanceMetric(sessionId, metric) {
    const session = this.getSession(sessionId);
    if (!session) return;

    session.data.performanceMetrics.push({
      ...metric,
      sessionTimestamp: Date.now() - session.createdAt
    });

    if (session.data.performanceMetrics.length > 100) {
      session.data.performanceMetrics = session.data.performanceMetrics.slice(-50);
    }

    this.saveSessionData(sessionId, session);
  }

  updateGPUMetrics(sessionId, gpuData) {
    const session = this.getSession(sessionId);
    if (!session) return;

    session.data.gpuMetrics = {
      ...session.data.gpuMetrics,
      ...gpuData,
      lastUpdated: Date.now(),
      sessionTimestamp: Date.now() - session.createdAt
    };

    this.saveSessionData(sessionId, session);
  }

  addError(sessionId, error) {
    const session = this.getSession(sessionId);
    if (!session) return;

    session.data.errors.push({
      timestamp: Date.now(),
      sessionTimestamp: Date.now() - session.createdAt,
      message: error.message || error,
      stack: error.stack,
      source: error.source || 'unknown',
      type: error.type || 'error'
    });

    if (session.data.errors.length > 100) {
      session.data.errors = session.data.errors.slice(-50);
    }

    this.saveSessionData(sessionId, session);
  }

  getSessionStats(sessionId) {
    const session = this.getSession(sessionId);
    if (!session) return null;

    const duration = Date.now() - session.createdAt;
    const pauseTime = session.totalPauseTime || 0;
    const activeTime = duration - pauseTime;

    return {
      sessionId: sessionId,
      duration: duration,
      activeTime: activeTime,
      pauseTime: pauseTime,
      state: session.state,
      progress: {
        currentPhase: session.progress.currentPhase,
        completedTests: session.progress.completedTests.length,
        totalTests: session.progress.testQueue.length,
        currentTest: session.progress.currentTest
      },
      counts: {
        networkRequests: session.data.networkRequests.length,
        consoleLogs: session.data.consoleLogs.length,
        errors: session.data.errors.length,
        performanceMetrics: session.data.performanceMetrics.length
      },
      memory: this.calculateSessionMemoryUsage(session)
    };
  }

  exportSession(sessionId) {
    const session = this.getSession(sessionId);
    if (!session) {
      throw new Error('Session not found');
    }

    return {
      ...session,
      exportedAt: new Date().toISOString(),
      version: '1.0.0',
      data: {
        ...session.data,
        testResults: Object.fromEntries(session.data.testResults)
      }
    };
  }

  importSession(sessionData) {
    const sessionId = sessionData.id || this.generateSessionId(sessionData.tabId);
    
    const session = {
      ...sessionData,
      id: sessionId,
      importedAt: Date.now(),
      lastActivity: Date.now()
    };

    if (sessionData.data && sessionData.data.testResults) {
      session.data.testResults = new Map(Object.entries(sessionData.data.testResults));
    }

    this.sessions.set(sessionId, session);
    this.saveSessionData(sessionId, session);

    return session;
  }

  generateSessionId(tabId) {
    const timestamp = Date.now();
    const random = Math.random().toString(36).substr(2, 9);
    return `session_${tabId}_${timestamp}_${random}`;
  }

  validateStateTransition(currentState, newState) {
    const validTransitions = {
      'CREATED': ['INITIALIZING', 'ERROR'],
      'INITIALIZING': ['BASELINE', 'ERROR'],
      'BASELINE': ['READY', 'ERROR'],
      'READY': ['TESTING', 'PAUSED', 'COMPLETED', 'ERROR'],
      'TESTING': ['READY', 'PAUSED', 'COMPLETED', 'ERROR'],
      'PAUSED': ['TESTING', 'READY', 'COMPLETED', 'ERROR'],
      'COMPLETED': ['READY', 'ERROR'],
      'ERROR': ['RECOVERING', 'READY'],
      'RECOVERING': ['READY', 'ERROR'],
      'RECOVERED': ['READY', 'ERROR']
    };

    const allowed = validTransitions[currentState] || [];
    if (!allowed.includes(newState)) {
      throw new Error(`Invalid state transition from ${currentState} to ${newState}`);
    }
  }

  isSessionValid(session) {
    if (!session || !session.id || !session.createdAt) {
      return false;
    }

    const age = Date.now() - session.createdAt;
    if (age > this.sessionTimeout) {
      return false;
    }

    const lastActivity = Date.now() - session.lastActivity;
    if (lastActivity > this.sessionTimeout / 2) {
      return false;
    }

    return true;
  }

  isSessionActive(session) {
    return this.isSessionValid(session) && 
           ['INITIALIZING', 'BASELINE', 'READY', 'TESTING', 'PAUSED'].includes(session.state);
  }

  createRecoveryPoint(session) {
    const recoveryData = {
      id: session.id,
      tabId: session.tabId,
      url: session.url,
      config: session.config,
      data: session.data,
      progress: session.progress,
      metadata: session.metadata,
      createdAt: session.createdAt,
      recoveryCreatedAt: Date.now()
    };

    this.recoveryData.set(session.id, recoveryData);

    setTimeout(() => {
      this.recoveryData.delete(session.id);
    }, 24 * 60 * 60 * 1000);
  }

  calculateSessionMemoryUsage(session) {
    let size = 0;
    
    try {
      size += JSON.stringify(session.data.networkRequests).length;
      size += JSON.stringify(session.data.consoleLogs).length;
      size += JSON.stringify(session.data.performanceMetrics).length;
      size += JSON.stringify(session.data.gpuMetrics).length;
      size += JSON.stringify(session.data.errors).length;
      
      if (session.data.baseline && session.data.baseline.screenshot) {
        size += session.data.baseline.screenshot.length;
      }
      
      for (const result of session.data.testResults.values()) {
        if (result.screenshot) {
          size += result.screenshot.length;
        }
      }
    } catch (e) {
      size = -1;
    }

    return size;
  }

  saveSessionData(sessionId, session) {
    try {
      const storageKey = `session_${sessionId}`;
      const sessionData = {
        ...session,
        data: {
          ...session.data,
          testResults: Object.fromEntries(session.data.testResults || new Map())
        }
      };
      
      this.sessionStorage.set(storageKey, sessionData);
      
      chrome.storage.local.set({
        [storageKey]: sessionData
      }).catch(error => {
        console.warn('Failed to save session to chrome.storage:', error);
      });
    } catch (error) {
      console.error('Failed to save session data:', error);
    }
  }

  loadSessionData(sessionId) {
    try {
      const storageKey = `session_${sessionId}`;
      
      const memoryData = this.sessionStorage.get(storageKey);
      if (memoryData) {
        if (memoryData.data && memoryData.data.testResults) {
          memoryData.data.testResults = new Map(Object.entries(memoryData.data.testResults));
        }
        return memoryData;
      }

      return new Promise((resolve) => {
        chrome.storage.local.get([storageKey], (result) => {
          const sessionData = result[storageKey];
          if (sessionData && sessionData.data && sessionData.data.testResults) {
            sessionData.data.testResults = new Map(Object.entries(sessionData.data.testResults));
          }
          resolve(sessionData || null);
        });
      });
    } catch (error) {
      console.error('Failed to load session data:', error);
      return null;
    }
  }

  deleteSessionData(sessionId) {
    try {
      const storageKey = `session_${sessionId}`;
      this.sessionStorage.delete(storageKey);
      
      chrome.storage.local.remove([storageKey]).catch(error => {
        console.warn('Failed to remove session from chrome.storage:', error);
      });
    } catch (error) {
      console.error('Failed to delete session data:', error);
    }
  }

  cleanupOldSessions() {
    const currentTime = Date.now();
    const sessionsToDelete = [];

    for (const [sessionId, session] of this.sessions) {
      if (!this.isSessionValid(session)) {
        sessionsToDelete.push(sessionId);
      }
    }

    for (const sessionId of sessionsToDelete) {
      this.deleteSession(sessionId);
    }

    if (this.sessions.size > this.maxSessions) {
      const sortedSessions = Array.from(this.sessions.entries())
        .sort(([,a], [,b]) => a.lastActivity - b.lastActivity);
      
      const toDelete = sortedSessions.slice(0, this.sessions.size - this.maxSessions);
      for (const [sessionId] of toDelete) {
        this.deleteSession(sessionId);
      }
    }
  }

  setupPeriodicCleanup() {
    setInterval(() => {
      this.cleanupOldSessions();
    }, 5 * 60 * 1000);
  }

  getAllSessions() {
    return Array.from(this.sessions.values());
  }

  getSessionCount() {
    return this.sessions.size;
  }

  clearAllSessions() {
    for (const sessionId of this.sessions.keys()) {
      this.deleteSession(sessionId);
    }
    return true;
  }
}

if (typeof module !== 'undefined' && module.exports) {
  module.exports = SessionManager;
} else if (typeof window !== 'undefined') {
  window.SessionManager = SessionManager;
}