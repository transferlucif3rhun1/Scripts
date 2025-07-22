class AnalyzerPopup {
  constructor() {
    this.currentTab = null;
    this.sessionData = null;
    this.statsInterval = null;
    this.sessionStartTime = null;
    
    this.initializePopup();
  }

  async initializePopup() {
    this.setupElements();
    this.setupEventListeners();
    await this.getCurrentTab();
    await this.checkPageStatus();
    await this.loadDomainMemory();
    this.startStatsUpdater();
  }

  setupElements() {
    this.elements = {
      statusIndicator: document.getElementById('status-indicator'),
      pageUrl: document.getElementById('page-url'),
      elementCount: document.getElementById('element-count'),
      requestCount: document.getElementById('request-count'),
      domainMemory: document.getElementById('domain-memory'),
      popupDomainName: document.getElementById('popup-domain-name'),
      popupLastTest: document.getElementById('popup-last-test'),
      startQuickAnalysis: document.getElementById('start-quick-analysis'),
      openDevtools: document.getElementById('open-devtools'),
      captureBaseline: document.getElementById('capture-baseline'),
      sessionInfo: document.getElementById('session-info'),
      sessionDuration: document.getElementById('session-duration'),
      popupProgress: document.getElementById('popup-progress'),
      popupProgressText: document.getElementById('popup-progress-text'),
      pauseSession: document.getElementById('pause-session'),
      stopSession: document.getElementById('stop-session'),
      quickStats: document.getElementById('quick-stats'),
      gpuContexts: document.getElementById('gpu-contexts'),
      networkRequests: document.getElementById('network-requests'),
      consoleErrors: document.getElementById('console-errors'),
      performanceScore: document.getElementById('performance-score'),
      recentResults: document.getElementById('recent-results'),
      lastAnalysisTime: document.getElementById('last-analysis-time'),
      improvementBadge: document.getElementById('improvement-badge'),
      featuresSummary: document.getElementById('features-summary'),
      viewHistory: document.getElementById('view-history'),
      exportData: document.getElementById('export-data'),
      settings: document.getElementById('settings')
    };
  }

  setupEventListeners() {
    this.elements.startQuickAnalysis.addEventListener('click', () => this.startQuickAnalysis());
    this.elements.openDevtools.addEventListener('click', () => this.openDevTools());
    this.elements.captureBaseline.addEventListener('click', () => this.captureBaseline());
    this.elements.pauseSession.addEventListener('click', () => this.pauseSession());
    this.elements.stopSession.addEventListener('click', () => this.stopSession());
    this.elements.viewHistory.addEventListener('click', () => this.viewHistory());
    this.elements.exportData.addEventListener('click', () => this.exportData());
    this.elements.settings.addEventListener('click', () => this.openSettings());

    chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
      this.handleBackgroundMessage(message);
    });

    chrome.storage.onChanged.addListener((changes) => {
      this.handleStorageChanges(changes);
    });
  }

  async getCurrentTab() {
    try {
      const tabs = await chrome.tabs.query({ active: true, currentWindow: true });
      this.currentTab = tabs[0];
      this.updatePageInfo();
    } catch (error) {
      console.error('Failed to get current tab:', error);
      this.showError('Failed to access current tab');
    }
  }

  updatePageInfo() {
    if (this.currentTab) {
      const url = new URL(this.currentTab.url);
      this.elements.pageUrl.textContent = url.hostname + url.pathname;
      this.elements.pageUrl.title = this.currentTab.url;
    }
  }

  async checkPageStatus() {
    if (!this.currentTab) return;

    try {
      const isValidPage = this.currentTab.url.startsWith('http://') || this.currentTab.url.startsWith('https://');
      
      if (!isValidPage) {
        this.showError('Cannot analyze this page type');
        return;
      }

      this.updateStatus('ready');
      this.elements.startQuickAnalysis.disabled = false;
      this.elements.captureBaseline.disabled = false;

      await this.checkExistingSession();
      await this.getPageStats();
      
    } catch (error) {
      console.error('Failed to check page status:', error);
      this.showError('Page check failed');
    }
  }

  async checkExistingSession() {
    try {
      // First check background for active session
      const response = await this.sendToBackground('get-session', {
        tabId: this.currentTab.id
      });

      if (response.success && response.session) {
        this.sessionData = response.session;
        this.sessionStartTime = response.session.createdAt || Date.now();
        this.showSessionInfo();
        this.updateSessionProgress();
        return;
      }

      // Check popup storage for session reference
      const result = await chrome.storage.local.get([
        `popup_session_${this.currentTab.id}`,
        `active_session_${this.currentTab.id}`
      ]);
      
      const popupSession = result[`popup_session_${this.currentTab.id}`];
      const activeSessionId = result[`active_session_${this.currentTab.id}`];
      
      if (popupSession || activeSessionId) {
        // Try to get the actual session data
        const sessionId = popupSession?.id || activeSessionId;
        if (sessionId) {
          const sessionResult = await chrome.storage.local.get([`session_${sessionId}`]);
          const sessionData = sessionResult[`session_${sessionId}`];
          
          if (sessionData && sessionData.state !== 'COMPLETED' && sessionData.state !== 'ERROR') {
            this.sessionData = sessionData;
            this.sessionStartTime = sessionData.createdAt || Date.now();
            this.showSessionInfo();
            this.updateSessionProgress();
          }
        }
      }
    } catch (error) {
      console.log('No existing session found');
    }
  }

  async getPageStats() {
    if (!this.currentTab?.id) return;

    try {
      const response = await chrome.tabs.sendMessage(this.currentTab.id, {
        action: 'get-current-state'
      });

      if (response) {
        this.updatePageStats(response);
        this.elements.quickStats.style.display = 'block';
      }
    } catch (error) {
      console.log('Page stats not available (content script may not be ready)');
    }
  }

  updatePageStats(stats) {
    this.elements.elementCount.textContent = stats.domElementCount || 0;
    this.elements.gpuContexts.textContent = stats.gpuContexts || 0;
    this.elements.networkRequests.textContent = stats.performanceData?.length || 0;
    
    const errorCount = stats.performanceData?.filter(entry => 
      entry.level === 'error' || entry.type === 'error'
    ).length || 0;
    this.elements.consoleErrors.textContent = errorCount;

    if (stats.memoryUsage) {
      const memoryMB = Math.round(stats.memoryUsage.usedJSHeapSize / 1024 / 1024);
      this.elements.performanceScore.textContent = `${memoryMB}MB`;
    }
  }

  async startQuickAnalysis() {
    try {
      this.updateStatus('starting');
      this.elements.startQuickAnalysis.disabled = true;
      this.elements.startQuickAnalysis.querySelector('.btn-text').textContent = 'Starting...';

      const response = await this.sendToBackground('start-session', {
        tabId: this.currentTab.id,
        url: this.currentTab.url
      });

      if (response.success) {
        this.sessionData = { 
          id: response.sessionId,
          createdAt: Date.now(),
          tabId: this.currentTab.id,
          persistent: true
        };
        this.sessionStartTime = Date.now();
        
        // Save session reference to storage
        chrome.storage.local.set({
          [`popup_session_${this.currentTab.id}`]: this.sessionData
        });
        
        this.showSessionInfo();
        this.updateStatus('analyzing');
        
        // Don't auto-open DevTools, let user decide
        this.showSuccess('Analysis started! Open DevTools → Website Analyzer to view progress');
      } else {
        throw new Error(response.error || 'Failed to start analysis');
      }
    } catch (error) {
      console.error('Failed to start quick analysis:', error);
      this.showError(error.message);
      this.elements.startQuickAnalysis.disabled = false;
      this.elements.startQuickAnalysis.querySelector('.btn-text').textContent = 'Quick Analysis';
    }
  }

  openDevTools() {
    const tabId = this.currentTab?.id;
    if (tabId) {
      chrome.tabs.sendMessage(tabId, { action: 'open-devtools' });
    }
    window.close();
  }

  async captureBaseline() {
    try {
      this.elements.captureBaseline.disabled = true;
      this.elements.captureBaseline.querySelector('.btn-text').textContent = 'Capturing...';

      const response = await this.sendToBackground('capture-screenshot', {
        tabId: this.currentTab.id
      });

      if (response.screenshot) {
        await chrome.storage.local.set({
          [`baseline_${this.currentTab.id}`]: {
            screenshot: response.screenshot,
            timestamp: Date.now(),
            url: this.currentTab.url
          }
        });

        this.showSuccess('Baseline captured!');
      }
    } catch (error) {
      console.error('Failed to capture baseline:', error);
      this.showError('Capture failed');
    } finally {
      this.elements.captureBaseline.disabled = false;
      this.elements.captureBaseline.querySelector('.btn-text').textContent = 'Capture Baseline';
    }
  }

  async pauseSession() {
    if (!this.sessionData) return;

    try {
      const isPaused = this.elements.pauseSession.textContent === '▶';
      
      if (isPaused) {
        this.sessionStartTime = Date.now() - this.getElapsedTime();
        this.startSessionTimer();
        this.elements.pauseSession.textContent = '⏸';
        this.updateStatus('analyzing');
      } else {
        this.stopSessionTimer();
        this.elements.pauseSession.textContent = '▶';
        this.updateStatus('paused');
      }
    } catch (error) {
      console.error('Failed to pause session:', error);
    }
  }

  async stopSession() {
    if (!this.sessionData) return;

    try {
      await this.sendToBackground('stop-session', {
        sessionId: this.sessionData.id
      });

      // Clean up local state
      chrome.storage.local.remove([
        `popup_session_${this.currentTab.id}`,
        `active_session_${this.currentTab.id}`
      ]);

      this.sessionData = null;
      this.sessionStartTime = null;
      this.hideSessionInfo();
      this.updateStatus('ready');
      this.elements.startQuickAnalysis.disabled = false;
      this.elements.startQuickAnalysis.querySelector('.btn-text').textContent = 'Quick Analysis';
      
      this.showSuccess('Analysis stopped successfully');
    } catch (error) {
      console.error('Failed to stop session:', error);
      this.showError('Failed to stop session');
    }
  }

  showSessionInfo() {
    this.elements.sessionInfo.style.display = 'block';
    this.elements.startQuickAnalysis.style.display = 'none';
    this.startSessionTimer();
  }

  hideSessionInfo() {
    this.elements.sessionInfo.style.display = 'none';
    this.elements.startQuickAnalysis.style.display = 'block';
    this.stopSessionTimer();
  }

  startSessionTimer() {
    this.stopSessionTimer();
    this.updateSessionTime();
    this.sessionTimer = setInterval(() => this.updateSessionTime(), 1000);
  }

  stopSessionTimer() {
    if (this.sessionTimer) {
      clearInterval(this.sessionTimer);
      this.sessionTimer = null;
    }
  }

  updateSessionTime() {
    if (this.sessionStartTime) {
      const elapsed = Math.floor((Date.now() - this.sessionStartTime) / 1000);
      const minutes = Math.floor(elapsed / 60);
      const seconds = elapsed % 60;
      this.elements.sessionDuration.textContent = 
        `${minutes.toString().padStart(2, '0')}:${seconds.toString().padStart(2, '0')}`;
    }
  }

  getElapsedTime() {
    return this.sessionStartTime ? Date.now() - this.sessionStartTime : 0;
  }

  updateSessionProgress() {
    if (this.sessionData) {
      const progress = this.sessionData.currentTestIndex || 0;
      const total = this.sessionData.testQueue?.length || 7;
      const percentage = (progress / total) * 100;
      
      this.elements.popupProgress.style.width = `${percentage}%`;
      this.elements.popupProgressText.textContent = 
        this.sessionData.currentTest ? `Testing: ${this.sessionData.currentTest}` : 'Initializing...';
    }
  }

  startStatsUpdater() {
    this.updateStats();
    this.statsInterval = setInterval(() => this.updateStats(), 5000);
  }

  async updateStats() {
    if (!this.currentTab) return;

    try {
      await this.getPageStats();
    } catch (error) {
      console.error('Failed to update stats:', error);
    }
  }

  async viewHistory() {
    try {
      const result = await chrome.storage.local.get(null);
      const historyItems = Object.entries(result)
        .filter(([key]) => key.startsWith('analysis_'))
        .map(([key, value]) => value)
        .sort((a, b) => b.timestamp - a.timestamp)
        .slice(0, 10);

      if (historyItems.length === 0) {
        this.showInfo('No analysis history found');
        return;
      }

      const historyHtml = historyItems.map(item => `
        <div class="history-item">
          <div class="history-url">${new URL(item.url).hostname}</div>
          <div class="history-date">${new Date(item.timestamp).toLocaleDateString()}</div>
          <div class="history-improvement">${item.improvement || 0}% faster</div>
        </div>
      `).join('');

      this.showModal('Analysis History', historyHtml);
    } catch (error) {
      console.error('Failed to view history:', error);
      this.showError('Failed to load history');
    }
  }

  async exportData() {
    try {
      const result = await chrome.storage.local.get(null);
      const exportData = {
        timestamp: new Date().toISOString(),
        version: '1.0.0',
        data: result
      };

      const blob = new Blob([JSON.stringify(exportData, null, 2)], { 
        type: 'application/json' 
      });
      const url = URL.createObjectURL(blob);
      
      const a = document.createElement('a');
      a.href = url;
      a.download = `website-analyzer-export-${Date.now()}.json`;
      a.click();
      
      URL.revokeObjectURL(url);
      this.showSuccess('Data exported successfully!');
    } catch (error) {
      console.error('Failed to export data:', error);
      this.showError('Export failed');
    }
  }

  openSettings() {
    chrome.runtime.openOptionsPage();
    window.close();
  }

  updateStatus(status) {
    this.elements.statusIndicator.className = `status-indicator ${status}`;
    
    const statusText = {
      'ready': 'Ready to analyze',
      'starting': 'Starting analysis...',
      'analyzing': 'Analysis in progress',
      'paused': 'Analysis paused',
      'completed': 'Analysis completed',
      'error': 'Error occurred'
    };

    this.elements.statusIndicator.title = statusText[status] || status;
  }

  showError(message) {
    this.updateStatus('error');
    this.showToast(message, 'error');
  }

  showSuccess(message) {
    this.showToast(message, 'success');
  }

  showInfo(message) {
    this.showToast(message, 'info');
  }

  showToast(message, type = 'info') {
    const toast = document.createElement('div');
    toast.className = `toast toast-${type}`;
    toast.textContent = message;
    
    document.body.appendChild(toast);
    
    setTimeout(() => {
      toast.classList.add('show');
    }, 100);
    
    setTimeout(() => {
      toast.classList.remove('show');
      setTimeout(() => {
        if (toast.parentNode) {
          toast.parentNode.removeChild(toast);
        }
      }, 300);
    }, 3000);
  }

  showModal(title, content) {
    const modal = document.createElement('div');
    modal.className = 'modal-overlay';
    modal.innerHTML = `
      <div class="modal">
        <div class="modal-header">
          <h4>${title}</h4>
          <button class="modal-close">&times;</button>
        </div>
        <div class="modal-content">${content}</div>
      </div>
    `;

    document.body.appendChild(modal);

    modal.querySelector('.modal-close').addEventListener('click', () => {
      modal.remove();
    });

    modal.addEventListener('click', (e) => {
      if (e.target === modal) {
        modal.remove();
      }
    });
  }

  handleBackgroundMessage(message) {
    if (message.target === 'popup') {
      switch (message.type) {
        case 'session-progress':
          this.updateSessionProgress();
          break;
        case 'session-completed':
          this.handleSessionCompleted(message.data);
          break;
        case 'session-error':
          this.showError(message.data.error);
          break;
      }
    }
  }

  handleSessionCompleted(data) {
    this.elements.recentResults.style.display = 'block';
    this.elements.lastAnalysisTime.textContent = new Date().toLocaleTimeString();
    
    if (data.improvement) {
      this.elements.improvementBadge.textContent = `${data.improvement}% faster`;
      this.elements.improvementBadge.className = 'improvement-badge positive';
    }

    if (data.eliminatedFeatures) {
      const count = data.eliminatedFeatures.length;
      this.elements.featuresSummary.textContent = 
        count > 0 ? `${count} features eliminated` : 'No features eliminated';
    }

    this.hideSessionInfo();
    this.updateStatus('completed');
  }

  handleStorageChanges(changes) {
    if (changes.recentAnalysis) {
      this.updateRecentResults(changes.recentAnalysis.newValue);
    }
  }

  updateRecentResults(analysisData) {
    if (analysisData) {
      this.elements.recentResults.style.display = 'block';
      this.elements.lastAnalysisTime.textContent = 
        new Date(analysisData.timestamp).toLocaleTimeString();
      
      if (analysisData.improvement) {
        this.elements.improvementBadge.textContent = `${analysisData.improvement}% faster`;
      }
      
      if (analysisData.eliminatedFeatures) {
        this.elements.featuresSummary.textContent = 
          `${analysisData.eliminatedFeatures.length} features eliminated`;
      }
    }
  }

  async sendToBackground(action, data) {
    return new Promise((resolve) => {
      chrome.runtime.sendMessage({
        action: action,
        ...data
      }, (response) => {
        resolve(response || { success: false, error: 'No response' });
      });
    });
  }

  async loadDomainMemory() {
    if (!this.currentTab || !this.currentTab.url) return;

    try {
      const response = await this.sendToBackground('get-domain-settings', { 
        url: this.currentTab.url 
      });
      
      if (response.success && response.settings) {
        const settings = response.settings;
        this.elements.popupDomainName.textContent = settings.domain;
        
        const lastTest = new Date(settings.lastTested).toLocaleDateString();
        const lastFeature = settings.settings?.blockedFeature || 'unknown';
        this.elements.popupLastTest.textContent = `${lastTest} (${lastFeature})`;
        
        this.elements.domainMemory.style.display = 'block';
      } else {
        const domain = this.extractDomain(this.currentTab.url);
        this.elements.popupDomainName.textContent = domain;
        this.elements.popupLastTest.textContent = 'No tests recorded';
        this.elements.domainMemory.style.display = 'block';
      }
    } catch (error) {
      console.log('Failed to load domain memory:', error);
    }
  }

  extractDomain(url) {
    try {
      const urlObj = new URL(url);
      return urlObj.hostname.replace(/^www\./, '');
    } catch (error) {
      return 'unknown';
    }
  }

  cleanup() {
    if (this.statsInterval) {
      clearInterval(this.statsInterval);
    }
    if (this.sessionTimer) {
      clearInterval(this.sessionTimer);
    }
  }
}

document.addEventListener('DOMContentLoaded', () => {
  window.analyzerPopup = new AnalyzerPopup();
});

window.addEventListener('beforeunload', () => {
  if (window.analyzerPopup) {
    window.analyzerPopup.cleanup();
  }
});