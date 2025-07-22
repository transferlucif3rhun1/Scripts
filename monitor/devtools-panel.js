class UltimateAnalyzerUI {
  constructor() {
    this.sessionId = null;
    this.currentSession = null;
    this.testQueue = ['analytics', 'social-widgets', 'advertising', 'fonts', 'images', 'javascript', 'css'];
    this.currentTestIndex = 0;
    this.testResults = new Map();
    this.baseline = null;
    this.currentState = null;
    this.sessionStartTime = null;
    this.sessionTimer = null;
    this.networkRequests = [];
    this.consoleMessages = [];
    this.gpuMetrics = {};
    this.performanceData = {};
    
    this.initializeUI();
    this.setupEventListeners();
    this.checkExistingSession();
    this.updateStatus();
  }

  async checkExistingSession() {
    try {
      const tabId = chrome.devtools.inspectedWindow.tabId;
      
      // Check for existing session in background
      const response = await this.sendToBackground('get-session', { tabId });
      
      if (response.success && response.session) {
        this.sessionId = response.session.id;
        this.currentSession = response.session;
        this.sessionStartTime = response.session.createdAt;
        
        // Update UI to show active session
        this.elements.startBtn.disabled = true;
        this.elements.stopBtn.disabled = false;
        this.elements.pauseBtn.disabled = false;
        this.elements.sessionBanner.style.display = 'flex';
        this.elements.testSingleFeature.disabled = false;
        
        this.startSessionTimer();
        this.updateStatus('Session active', 'success');
        this.updateProgress(50, 'Session recovered - analysis in progress');
        
        // Load session data
        if (response.session.baseline) {
          this.baseline = response.session.baseline;
          this.displayBaseline();
        }
        
        return true;
      }
    } catch (error) {
      console.log('No existing session found');
    }
    
    return false;
  }

  initializeUI() {
    this.elements = {
      startBtn: document.getElementById('start-session'),
      stopBtn: document.getElementById('stop-session'),
      pauseBtn: document.getElementById('pause-session'),
      testResourceBtn: document.getElementById('test-resource'),
      sessionBanner: document.getElementById('session-banner'),
      stopBannerBtn: document.getElementById('stop-session-banner'),
      sessionStatus: document.getElementById('session-status'),
      sessionTime: document.getElementById('session-time'),
      progressFill: document.getElementById('progress-fill'),
      progressText: document.getElementById('progress-text'),
      currentDomain: document.getElementById('current-domain'),
      lastTested: document.getElementById('last-tested'),
      testFeatureSelect: document.getElementById('test-feature-select'),
      testSingleFeature: document.getElementById('test-single-feature'),
      sessionTime: document.getElementById('session-time'),
      progressFill: document.getElementById('progress-fill'),
      progressText: document.getElementById('progress-text'),
      beforeScreenshot: document.getElementById('before-screenshot'),
      afterScreenshot: document.getElementById('after-screenshot'),
      beforeMetrics: document.getElementById('before-metrics'),
      afterMetrics: document.getElementById('after-metrics'),
      feedbackSection: document.getElementById('feedback-section'),
      currentFeature: document.getElementById('current-feature'),
      feedbackBtns: document.querySelectorAll('.feedback-btn'),
      feedbackNotes: document.getElementById('feedback-notes'),
      networkPanel: document.getElementById('network-panel'),
      networkCount: document.getElementById('network-count'),
      networkList: document.getElementById('network-requests'),
      networkFilter: document.getElementById('network-filter'),
      networkSearch: document.getElementById('network-search'),
      gpuPanel: document.getElementById('gpu-panel'),
      gpuCount: document.getElementById('gpu-count'),
      webglContexts: document.getElementById('webgl-contexts'),
      canvasOperations: document.getElementById('canvas-operations'),
      gpuMemory: document.getElementById('gpu-memory'),
      frameRate: document.getElementById('frame-rate'),
      gpuDetails: document.getElementById('gpu-details'),
      consolePanel: document.getElementById('console-panel'),
      consoleCount: document.getElementById('console-count'),
      consoleOutput: document.getElementById('console-output'),
      consoleFilters: document.querySelectorAll('.console-filter'),
      resultsSection: document.getElementById('results-section'),
      performanceImprovement: document.getElementById('performance-improvement'),
      eliminatedFeatures: document.getElementById('eliminated-features'),
      requiredFeatures: document.getElementById('required-features'),
      optimizationScore: document.getElementById('optimization-score'),
      currentUrl: document.getElementById('current-url'),
      extensionStatus: document.getElementById('extension-status'),
      exportResults: document.getElementById('export-results'),
      applyOptimizations: document.getElementById('apply-optimizations')
    };

    this.updateCurrentURL();
    this.loadDomainMemory();
  }

  async loadDomainMemory() {
    try {
      const url = await this.getCurrentURL();
      const response = await this.sendToBackground('get-domain-settings', { url });
      
      if (response.success && response.settings) {
        const settings = response.settings;
        this.elements.currentDomain.textContent = settings.domain;
        
        const lastTest = new Date(settings.lastTested).toLocaleString();
        const lastFeature = settings.settings?.blockedFeature || 'unknown';
        this.elements.lastTested.textContent = `Last tested: ${lastTest} (${lastFeature})`;
        
        // Show test history if available
        if (settings.testHistory && settings.testHistory.length > 0) {
          const historyText = settings.testHistory.slice(-3).map(test => {
            const date = new Date(test.timestamp).toLocaleDateString();
            const feature = test.settings?.blockedFeature || 'unknown';
            return `${date}: ${feature}`;
          }).join(', ');
          
          this.elements.lastTested.innerHTML += `<br><small>Recent: ${historyText}</small>`;
        }
      } else {
        const url = await this.getCurrentURL();
        const domain = this.extractDomain(url);
        this.elements.currentDomain.textContent = domain;
        this.elements.lastTested.textContent = 'No previous tests for this domain';
      }
    } catch (error) {
      console.error('Failed to load domain memory:', error);
      this.elements.currentDomain.textContent = 'Unknown';
      this.elements.lastTested.textContent = 'Error loading domain data';
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

  async testSingleFeature() {
    if (!this.sessionId) {
      this.showError('No active session. Please start analysis first.');
      return;
    }

    const feature = this.elements.testFeatureSelect.value;
    if (!feature) {
      this.showError('Please select a feature to test.');
      return;
    }

    try {
      this.elements.testSingleFeature.disabled = true;
      this.elements.testSingleFeature.textContent = 'Testing...';
      this.updateStatus('Testing feature with fresh page reload...', 'loading');

      const response = await this.sendToBackground('test-resource-blocking', {
        sessionId: this.sessionId,
        feature: feature
      });

      if (response.success) {
        this.updateStatus(`Testing: ${this.formatFeatureName(feature)}`, 'success');
        this.updateProgress(75, `Testing ${this.formatFeatureName(feature)} - Page reloading...`);
        
        // Show testing indicator
        this.showTestingIndicator(feature);
        
        // Reload domain memory after test
        setTimeout(() => {
          this.loadDomainMemory();
        }, 2000);
      } else {
        throw new Error(response.error || 'Test failed');
      }
    } catch (error) {
      this.showError(`Test failed: ${error.message}`);
    } finally {
      this.elements.testSingleFeature.disabled = false;
      this.elements.testSingleFeature.textContent = '🧪 Test & Reload';
    }
  }

  showTestingIndicator(feature) {
    // Create a temporary indicator
    const indicator = document.createElement('div');
    indicator.className = 'testing-indicator';
    indicator.innerHTML = `
      <div style="
        background: linear-gradient(90deg, #17a2b8, #20c997);
        color: white;
        padding: 12px 16px;
        border-radius: 6px;
        margin: 8px 0;
        font-size: 12px;
        font-weight: 500;
        text-align: center;
        animation: pulse 1.5s ease-in-out infinite;
      ">
        🧪 Testing ${this.formatFeatureName(feature)} - Page reloading with fresh data...
      </div>
    `;
    
    this.elements.progressText.parentNode.appendChild(indicator);
    
    // Remove after 10 seconds
    setTimeout(() => {
      if (indicator.parentNode) {
        indicator.remove();
      }
    }, 10000);
  }

  showError(message) {
    // Simple error display
    const errorDiv = document.createElement('div');
    errorDiv.style.cssText = `
      background: #f8d7da;
      color: #721c24;
      padding: 8px 12px;
      border-radius: 4px;
      margin: 8px 0;
      font-size: 12px;
      border: 1px solid #f5c6cb;
    `;
    errorDiv.textContent = message;
    
    this.elements.progressText.parentNode.appendChild(errorDiv);
    
    setTimeout(() => {
      if (errorDiv.parentNode) {
        errorDiv.remove();
      }
    }, 5000);
  }

  setupEventListeners() {
    this.elements.startBtn.addEventListener('click', () => this.startSession());
    this.elements.stopBtn.addEventListener('click', () => this.stopSession());
    this.elements.pauseBtn.addEventListener('click', () => this.pauseSession());
    this.elements.stopBannerBtn.addEventListener('click', () => this.stopSession());
    this.elements.testSingleFeature.addEventListener('click', () => this.testSingleFeature());

    this.elements.testFeatureSelect.addEventListener('change', () => {
      this.elements.testSingleFeature.disabled = !this.elements.testFeatureSelect.value;
    });

    this.elements.feedbackBtns.forEach(btn => {
      btn.addEventListener('click', (e) => this.handleUserFeedback(e.target.dataset.response));
    });

    document.querySelectorAll('.toggle-btn').forEach(btn => {
      btn.addEventListener('click', (e) => this.togglePanel(e.target));
    });

    this.elements.networkFilter.addEventListener('change', () => this.filterNetworkRequests());
    this.elements.networkSearch.addEventListener('input', () => this.filterNetworkRequests());

    this.elements.consoleFilters.forEach(filter => {
      filter.addEventListener('click', (e) => this.filterConsoleMessages(e.target.dataset.level));
    });

    this.elements.exportResults.addEventListener('click', () => this.exportResults());
    this.elements.applyOptimizations.addEventListener('click', () => this.applyOptimizations());

    chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
      if (message.target === 'devtools') {
        this.handleBackgroundMessage(message);
      }
    });

    chrome.devtools.network.onRequestFinished.addListener((request) => {
      this.handleNetworkRequest(request);
    });

    setInterval(() => this.updateSessionTime(), 1000);
  }

  async startSession() {
    try {
      this.updateStatus('Starting session...', 'loading');
      this.elements.startBtn.disabled = true;

      const response = await this.sendToBackground('start-session', {
        tabId: chrome.devtools.inspectedWindow.tabId,
        url: await this.getCurrentURL()
      });

      if (response.success) {
        this.sessionId = response.sessionId;
        this.sessionStartTime = Date.now();
        this.currentTestIndex = 0;
        this.testResults.clear();
        this.networkRequests = [];
        this.consoleMessages = [];

        this.elements.startBtn.disabled = true;
        this.elements.stopBtn.disabled = false;
        this.elements.pauseBtn.disabled = false;
        this.elements.sessionBanner.style.display = 'flex';
        this.elements.testSingleFeature.disabled = false;

        this.updateProgress(10, 'Session started, capturing baseline...');
        this.startSessionTimer();
        this.updateStatus('Capturing baseline...', 'success');
      } else {
        throw new Error(response.error || 'Failed to start session');
      }
    } catch (error) {
      this.updateStatus(`Error: ${error.message}`, 'error');
      this.elements.startBtn.disabled = false;
    }
  }

  async stopSession() {
    try {
      this.updateStatus('Stopping session...', 'loading');
      
      if (this.sessionId) {
        const response = await this.sendToBackground('stop-session', {
          sessionId: this.sessionId
        });

        if (!response.success) {
          console.warn('Failed to stop session cleanly:', response.error);
        }
      }

      this.sessionId = null;
      this.currentSession = null;
      this.stopSessionTimer();

      this.elements.startBtn.disabled = false;
      this.elements.stopBtn.disabled = true;
      this.elements.pauseBtn.disabled = true;
      this.elements.feedbackSection.style.display = 'none';
      this.elements.sessionBanner.style.display = 'none';
      this.elements.testSingleFeature.disabled = true;

      this.updateProgress(0, 'Session stopped');
      this.updateStatus('Ready', 'success');

      if (this.testResults.size > 0) {
        this.showResults();
      }
    } catch (error) {
      this.updateStatus(`Error stopping session: ${error.message}`, 'error');
    }
  }

  pauseSession() {
    if (this.sessionTimer) {
      this.stopSessionTimer();
      this.updateStatus('Session paused', 'warning');
      this.elements.pauseBtn.textContent = '▶ Resume';
    } else {
      this.startSessionTimer();
      this.updateStatus('Session running', 'success');
      this.elements.pauseBtn.textContent = '⏸ Pause';
    }
  }

  startSessionTimer() {
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
      this.elements.sessionTime.textContent = `${minutes.toString().padStart(2, '0')}:${seconds.toString().padStart(2, '0')}`;
    }
  }

  async getCurrentURL() {
    return new Promise((resolve) => {
      chrome.devtools.inspectedWindow.eval('window.location.href', (result) => {
        resolve(result);
      });
    });
  }

  updateCurrentURL() {
    this.getCurrentURL().then(url => {
      this.elements.currentUrl.textContent = url || 'Unknown URL';
    });
  }

  handleBackgroundMessage(message) {
    switch (message.type) {
      case 'session-started':
        this.currentSession = message.data;
        break;

      case 'baseline-captured':
        this.baseline = message.data;
        this.displayBaseline();
        this.startProgressiveTests();
        break;

      case 'feature-disabled':
        this.handleFeatureDisabled(message.data);
        break;

      case 'test-state-captured':
        this.handleTestStateCaptured(message.data);
        break;

      case 'network-request':
        this.addNetworkRequest(message.data);
        break;

      case 'console-message':
        this.addConsoleMessage(message.data);
        break;

      case 'exception':
        this.addConsoleMessage({
          ...message.data,
          level: 'error',
          text: message.data.message
        });
        break;

      case 'performance-metrics':
        this.updatePerformanceMetrics(message.data);
        break;

      case 'session-state-changed':
        this.currentSession = message.data;
        if (message.data.state === 'PAUSED') {
          this.updateStatus('Session paused', 'warning');
          this.elements.pauseBtn.textContent = '▶ Resume';
        } else if (message.data.state === 'RUNNING') {
          this.updateStatus('Session running', 'success');
          this.elements.pauseBtn.textContent = '⏸ Pause';
        }
        break;

      case 'test-started':
        this.handleTestStarted(message.data);
        break;
    }
  }

  displayBaseline() {
    if (this.baseline && this.baseline.screenshot) {
      this.elements.beforeScreenshot.src = this.baseline.screenshot;
      this.updateMetricsDisplay('before', this.baseline);
    }
  }

  updateMetricsDisplay(type, data) {
    const prefix = type === 'before' ? 'before' : 'after';
    
    if (data.performanceMetrics && data.performanceMetrics.navigation) {
      const nav = data.performanceMetrics.navigation;
      const loadTime = nav.loadEventEnd - nav.navigationStart;
      document.getElementById(`${prefix}-load-time`).textContent = `${Math.round(loadTime)}ms`;
    }

    if (data.networkRequests) {
      document.getElementById(`${prefix}-requests`).textContent = data.networkRequests.length;
      
      const totalSize = data.networkRequests.reduce((sum, req) => {
        return sum + (req.transferSize || req.size || 0);
      }, 0);
      document.getElementById(`${prefix}-size`).textContent = this.formatBytes(totalSize);
    }

    if (data.gpuMetrics) {
      const gpuInfo = `${data.gpuMetrics.webglContexts || 0} WebGL`;
      document.getElementById(`${prefix}-gpu`).textContent = gpuInfo;
    }
  }

  async startProgressiveTests() {
    this.updateProgress(20, 'Starting progressive feature testing...');
    
    if (this.currentTestIndex < this.testQueue.length) {
      await this.testNextFeature();
    } else {
      await this.completeAnalysis();
    }
  }

  async testNextFeature() {
    const feature = this.testQueue[this.currentTestIndex];
    const progress = 20 + (this.currentTestIndex / this.testQueue.length) * 60;
    
    this.updateProgress(progress, `Testing: ${feature}...`);
    this.elements.currentFeature.textContent = this.formatFeatureName(feature);

    try {
      const response = await this.sendToBackground('disable-feature', {
        sessionId: this.sessionId,
        feature: feature
      });

      if (response.success) {
        setTimeout(() => {
          this.reloadAndCapture();
        }, 1000);
      } else {
        throw new Error(response.error || 'Failed to disable feature');
      }
    } catch (error) {
      this.updateStatus(`Error testing ${feature}: ${error.message}`, 'error');
      this.skipCurrentTest();
    }
  }

  async reloadAndCapture() {
    try {
      await this.sendToBackground('reload-and-capture', {
        sessionId: this.sessionId
      });
    } catch (error) {
      console.error('Failed to reload and capture:', error);
      this.skipCurrentTest();
    }
  }

  handleFeatureDisabled(data) {
    this.updateStatus(`Feature disabled: ${data.feature}`, 'success');
  }

  handleTestStateCaptured(data) {
    this.currentState = data;
    this.displayCurrentState();
    this.showUserFeedback();
  }

  displayCurrentState() {
    if (this.currentState && this.currentState.screenshot) {
      this.elements.afterScreenshot.src = this.currentState.screenshot;
      this.updateMetricsDisplay('after', this.currentState);
      this.highlightDifferences();
    }
  }

  highlightDifferences() {
    if (!this.baseline || !this.currentState) return;

    const beforeLoad = this.getLoadTime(this.baseline);
    const afterLoad = this.getLoadTime(this.currentState);
    const beforeRequests = this.baseline.networkRequests?.length || 0;
    const afterRequests = this.currentState.networkRequests?.length || 0;

    if (afterLoad < beforeLoad) {
      document.getElementById('after-load-time').classList.add('improvement');
    } else if (afterLoad > beforeLoad) {
      document.getElementById('after-load-time').classList.add('regression');
    }

    if (afterRequests < beforeRequests) {
      document.getElementById('after-requests').classList.add('improvement');
    }
  }

  getLoadTime(data) {
    if (data.performanceMetrics && data.performanceMetrics.navigation) {
      const nav = data.performanceMetrics.navigation;
      return nav.loadEventEnd - nav.navigationStart;
    }
    return 0;
  }

  showUserFeedback() {
    this.elements.feedbackSection.style.display = 'block';
    this.elements.feedbackSection.scrollIntoView({ behavior: 'smooth' });
  }

  async handleUserFeedback(response) {
    const feature = this.testQueue[this.currentTestIndex];
    const notes = this.elements.feedbackNotes.value;

    const testResult = {
      feature: feature,
      userResponse: response,
      notes: notes,
      baseline: this.baseline,
      testState: this.currentState,
      timestamp: Date.now()
    };

    this.testResults.set(feature, testResult);

    if (response === 'different' || response === 'broken') {
      try {
        await this.sendToBackground('enable-feature', {
          sessionId: this.sessionId,
          feature: feature
        });
      } catch (error) {
        console.error('Failed to re-enable feature:', error);
      }
    }

    this.elements.feedbackSection.style.display = 'none';
    this.elements.feedbackNotes.value = '';

    this.currentTestIndex++;
    
    if (this.currentTestIndex < this.testQueue.length) {
      setTimeout(() => this.testNextFeature(), 1000);
    } else {
      await this.completeAnalysis();
    }
  }

  skipCurrentTest() {
    this.currentTestIndex++;
    if (this.currentTestIndex < this.testQueue.length) {
      setTimeout(() => this.testNextFeature(), 1000);
    } else {
      this.completeAnalysis();
    }
  }

  async completeAnalysis() {
    this.updateProgress(100, 'Analysis complete!');
    this.updateStatus('Analysis completed', 'success');
    this.showResults();
  }

  showResults() {
    const eliminated = [];
    const required = [];
    let performanceGain = 0;

    for (const [feature, result] of this.testResults) {
      if (result.userResponse === 'same') {
        eliminated.push(feature);
      } else {
        required.push(feature);
      }
    }

    if (this.baseline && this.currentState) {
      const beforeLoad = this.getLoadTime(this.baseline);
      const afterLoad = this.getLoadTime(this.currentState);
      if (beforeLoad > 0) {
        performanceGain = Math.round(((beforeLoad - afterLoad) / beforeLoad) * 100);
      }
    }

    this.elements.performanceImprovement.textContent = `${Math.max(0, performanceGain)}%`;
    
    this.elements.eliminatedFeatures.innerHTML = eliminated.map(feature => 
      `<div class="result-list-item eliminated">${this.formatFeatureName(feature)}</div>`
    ).join('');

    this.elements.requiredFeatures.innerHTML = required.map(feature => 
      `<div class="result-list-item required">${this.formatFeatureName(feature)}</div>`
    ).join('');

    const score = this.calculateOptimizationScore(eliminated, required, performanceGain);
    this.elements.optimizationScore.textContent = `${score}/100`;

    this.elements.resultsSection.style.display = 'block';
    this.elements.resultsSection.scrollIntoView({ behavior: 'smooth' });
  }

  calculateOptimizationScore(eliminated, required, performanceGain) {
    let score = 0;
    score += eliminated.length * 10;
    score += Math.min(performanceGain, 50);
    score = Math.min(score, 100);
    return score;
  }

  formatFeatureName(feature) {
    const names = {
      'analytics': 'Analytics Scripts',
      'social-widgets': 'Social Media Widgets',
      'advertising': 'Advertisement Scripts',
      'fonts': 'Web Fonts',
      'images': 'Images',
      'javascript': 'JavaScript',
      'css': 'Stylesheets'
    };
    return names[feature] || feature;
  }

  addNetworkRequest(request) {
    this.networkRequests.push(request);
    this.updateNetworkCount();
    this.renderNetworkRequest(request);
  }

  updateNetworkCount() {
    this.elements.networkCount.textContent = this.networkRequests.length;
  }

  renderNetworkRequest(request) {
    const row = document.createElement('div');
    row.className = 'network-request';
    
    const status = request.status || (request.error ? 'ERR' : 'PENDING');
    const statusClass = request.error ? 'request-failed' : 
                       request.blocked ? 'request-blocked' : 'request-success';
    
    row.className += ` ${statusClass}`;
    
    row.innerHTML = `
      <div class="col-method">${request.method || 'GET'}</div>
      <div class="col-url" title="${request.url}">${this.truncateUrl(request.url)}</div>
      <div class="col-status">${status}</div>
      <div class="col-size">${this.formatBytes(request.size || 0)}</div>
      <div class="col-time">${request.duration || 0}ms</div>
      <div class="col-type">${request.type || 'other'}</div>
    `;

    this.elements.networkList.appendChild(row);
    
    if (this.elements.networkList.children.length > 100) {
      this.elements.networkList.removeChild(this.elements.networkList.firstChild);
    }
  }

  filterNetworkRequests() {
    const filter = this.elements.networkFilter.value;
    const search = this.elements.networkSearch.value.toLowerCase();
    
    Array.from(this.elements.networkList.children).forEach(row => {
      const url = row.querySelector('.col-url').textContent.toLowerCase();
      const type = row.querySelector('.col-type').textContent;
      const status = row.querySelector('.col-status').textContent;
      
      let visible = true;
      
      if (filter !== 'all') {
        if (filter === 'blocked' && !row.classList.contains('request-blocked')) visible = false;
        if (filter === 'failed' && !row.classList.contains('request-failed')) visible = false;
        if (filter !== 'blocked' && filter !== 'failed' && type !== filter) visible = false;
      }
      
      if (search && !url.includes(search)) visible = false;
      
      row.style.display = visible ? 'grid' : 'none';
    });
  }

  addConsoleMessage(message) {
    this.consoleMessages.push(message);
    this.updateConsoleCount();
    this.renderConsoleMessage(message);
  }

  updateConsoleCount() {
    this.elements.consoleCount.textContent = this.consoleMessages.length;
  }

  renderConsoleMessage(message) {
    const div = document.createElement('div');
    div.className = `console-message ${message.level || 'log'}`;
    
    const timestamp = new Date(message.timestamp).toLocaleTimeString();
    
    div.innerHTML = `
      <div class="console-timestamp">${timestamp}</div>
      <div class="console-content">${this.escapeHtml(message.text || message.message || '')}</div>
    `;

    this.elements.consoleOutput.appendChild(div);
    
    if (this.elements.consoleOutput.children.length > 100) {
      this.elements.consoleOutput.removeChild(this.elements.consoleOutput.firstChild);
    }
    
    this.elements.consoleOutput.scrollTop = this.elements.consoleOutput.scrollHeight;
  }

  filterConsoleMessages(level) {
    this.elements.consoleFilters.forEach(filter => {
      filter.classList.toggle('active', filter.dataset.level === level);
    });

    Array.from(this.elements.consoleOutput.children).forEach(message => {
      if (level === 'all') {
        message.style.display = 'flex';
      } else {
        message.style.display = message.classList.contains(level) ? 'flex' : 'none';
      }
    });
  }

  updatePerformanceMetrics(data) {
    this.performanceData = data;
    this.updateGPUMetrics(data);
  }

  updateGPUMetrics(data) {
    if (data.webglStats) {
      this.elements.webglContexts.textContent = data.webglStats.contexts || 0;
      this.elements.canvasOperations.textContent = data.webglStats.drawCalls || 0;
    }

    if (data.canvasStats) {
      const totalOps = (data.canvasStats.drawOperations || 0) + 
                      (data.canvasStats.imageOperations || 0) + 
                      (data.canvasStats.textOperations || 0);
      this.elements.canvasOperations.textContent = totalOps;
    }

    if (data.frameStats) {
      this.elements.frameRate.textContent = `${Math.round(data.frameStats.fps || 0)} FPS`;
    }

    if (data.memory && data.memory.usedJSHeapSize) {
      const memoryMB = Math.round(data.memory.usedJSHeapSize / 1024 / 1024);
      this.elements.gpuMemory.textContent = `${memoryMB} MB`;
    }

    this.elements.gpuCount.textContent = (data.webglStats?.contexts || 0) + (data.canvasStats?.contexts || 0);
  }

  togglePanel(button) {
    const panel = button.closest('.panel-section');
    const content = panel.querySelector('.panel-content');
    const isCollapsed = content.style.display === 'none';
    
    content.style.display = isCollapsed ? 'block' : 'none';
    button.textContent = isCollapsed ? '▼' : '▶';
  }

  async exportResults() {
    const results = {
      sessionId: this.sessionId,
      url: await this.getCurrentURL(),
      timestamp: new Date().toISOString(),
      baseline: this.baseline,
      testResults: Object.fromEntries(this.testResults),
      networkRequests: this.networkRequests,
      consoleMessages: this.consoleMessages,
      performanceData: this.performanceData
    };

    const blob = new Blob([JSON.stringify(results, null, 2)], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    
    const a = document.createElement('a');
    a.href = url;
    a.download = `website-analysis-${Date.now()}.json`;
    a.click();
    
    URL.revokeObjectURL(url);
  }

  async applyOptimizations() {
    try {
      this.updateStatus('Applying optimizations...', 'loading');
      
      const eliminated = [];
      for (const [feature, result] of this.testResults) {
        if (result.userResponse === 'same') {
          eliminated.push(feature);
        }
      }

      for (const feature of eliminated) {
        await this.sendToBackground('disable-feature', {
          sessionId: this.sessionId,
          feature: feature
        });
      }

      this.updateStatus('Optimizations applied!', 'success');
    } catch (error) {
      this.updateStatus(`Error applying optimizations: ${error.message}`, 'error');
    }
  }

  handleNetworkRequest(request) {
    this.addNetworkRequest({
      url: request.request.url,
      method: request.request.method,
      status: request.response.status,
      size: request.response.bodySize,
      duration: request.time,
      type: request.request.type || 'other',
      timestamp: Date.now()
    });
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

  updateProgress(percentage, text) {
    this.elements.progressFill.style.width = `${percentage}%`;
    this.elements.progressText.textContent = text;
  }

  updateStatus(text, type = 'default') {
    this.elements.sessionStatus.textContent = text;
    this.elements.sessionStatus.className = `session-status ${type}`;
    this.elements.extensionStatus.textContent = text;
  }

  formatBytes(bytes) {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
  }

  truncateUrl(url) {
    if (url.length <= 50) return url;
    return url.substring(0, 47) + '...';
  }

  escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
  }
}

document.addEventListener('DOMContentLoaded', () => {
  window.analyzerUI = new UltimateAnalyzerUI();
});