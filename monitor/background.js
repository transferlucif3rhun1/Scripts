class UltimateWebsiteAnalyzer {
  constructor() {
    this.activeSessions = new Map();
    this.networkRequests = new Map();
    this.blockedPatterns = new Map();
    this.debuggerAttached = new Set();
    this.setupEventListeners();
    this.recoverSessions();
  }

  async recoverSessions() {
    try {
      const result = await chrome.storage.local.get(null);
      const sessionKeys = Object.keys(result).filter(key => key.startsWith('session_'));
      
      for (const key of sessionKeys) {
        const session = result[key];
        if (session && session.persistent && 
            !['COMPLETED', 'ERROR', 'STOPPED'].includes(session.state)) {
          
          try {
            const tab = await chrome.tabs.get(session.tabId);
            if (tab) {
              this.activeSessions.set(session.id, session);
              this.networkRequests.set(session.tabId, session.networkRequests || []);
              console.log(`Recovered session ${session.id} for tab ${session.tabId}`);
            } else {
              chrome.storage.local.remove([key, `active_session_${session.tabId}`]);
            }
          } catch (e) {
            chrome.storage.local.remove([key, `active_session_${session.tabId}`]);
          }
        }
      }
    } catch (error) {
      console.error('Failed to recover sessions:', error);
    }
  }

  setupEventListeners() {
    chrome.tabs.onUpdated.addListener((tabId, changeInfo, tab) => {
      if (changeInfo.status === 'complete' && tab.url) {
        this.handleTabLoad(tabId, tab.url);
      }
    });

    chrome.tabs.onRemoved.addListener(tabId => {
      this.cleanupSession(tabId);
    });

    chrome.webRequest.onBeforeRequest.addListener(
      details => this.handleBeforeRequest(details),
      { urls: ["<all_urls>"] },
      ["requestBody"]
    );

    chrome.webRequest.onCompleted.addListener(
      details => this.handleRequestCompleted(details),
      { urls: ["<all_urls>"] },
      ["responseHeaders"]
    );

    chrome.webRequest.onErrorOccurred.addListener(
      details => this.handleRequestError(details),
      { urls: ["<all_urls>"] }
    );

    chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
      this.handleMessage(message, sender, sendResponse);
      return true;
    });

    chrome.debugger.onEvent.addListener((source, method, params) => {
      this.handleDebuggerEvent(source, method, params);
    });

    chrome.debugger.onDetach.addListener((source, reason) => {
      this.handleDebuggerDetach(source, reason);
    });
  }

  async startSession(tabId, url) {
    const existing = this.getSessionDataByTabId(tabId);
    if (existing && !['COMPLETED','ERROR'].includes(existing.state)) {
      return { success: true, sessionId: existing.id, existing: true };
    }

    const sessionId = `session_${tabId}_${Date.now()}`;
    const session = {
      id: sessionId,
      tabId,
      url,
      state: 'STARTING',
      startTime: Date.now(),
      createdAt: Date.now(),
      lastActivity: Date.now(),
      networkRequests: [],
      consoleLogs: [],
      disabledFeatures: new Set(),
      testQueue: ['analytics','social-widgets','advertising','fonts','images','javascript','css'],
      currentTest: null,
      testResults: new Map(),
      performanceMetrics: [],
      gpuMetrics: {},
      baseline: null,
      errorLog: [],
      recoveryAttempts: 0,
      persistent: true
    };

    this.activeSessions.set(sessionId, session);
    this.networkRequests.set(tabId, []);
    await chrome.storage.local.set({
      [`session_${sessionId}`]: session,
      [`active_session_${tabId}`]: sessionId
    });

    try {
      await this.attachDebugger(tabId);
      session.state = 'BASELINE';
      session.lastActivity = Date.now();
      await this.establishBaseline(session);
      await chrome.storage.local.set({ [`session_${sessionId}`]: session });
      this.notifyDevTools(sessionId, 'session-started', session);
      return { success: true, sessionId };
    } catch (error) {
      session.errorLog.push({ error: error.message, timestamp: Date.now() });
      session.state = 'ERROR';
      await chrome.storage.local.set({ [`session_${sessionId}`]: session });
      return { success: false, error: error.message };
    }
  }

  async stopSession(sessionId) {
    const session = this.activeSessions.get(sessionId);
    if (!session) return { success: false, error: 'Session not found' };

    try {
      session.state = 'STOPPING';
      await this.restoreAllFeatures(session);
      await this.detachDebugger(session.tabId);
      await chrome.storage.local.remove([
        `session_${sessionId}`,
        `active_session_${session.tabId}`,
        `popup_session_${session.tabId}`
      ]);
      this.cleanupSession(session.tabId);
      this.activeSessions.delete(sessionId);
      this.notifyDevTools(sessionId, 'session-stopped', null);
      return { success: true };
    } catch (error) {
      session.errorLog.push({ error: error.message, timestamp: Date.now() });
      return { success: false, error: error.message };
    }
  }

  async attachDebugger(tabId) {
    if (this.debuggerAttached.has(tabId)) return;
    return new Promise((resolve, reject) => {
      chrome.debugger.attach({ tabId }, "1.3", () => {
        if (chrome.runtime.lastError) reject(new Error(chrome.runtime.lastError.message));
        else {
          this.debuggerAttached.add(tabId);
          Promise.all([
            chrome.debugger.sendCommand({ tabId }, "Network.enable"),
            chrome.debugger.sendCommand({ tabId }, "Runtime.enable"),
            chrome.debugger.sendCommand({ tabId }, "Performance.enable"),
            chrome.debugger.sendCommand({ tabId }, "Page.enable"),
            chrome.debugger.sendCommand({ tabId }, "DOM.enable")
          ]).then(resolve, reject);
        }
      });
    });
  }

  async detachDebugger(tabId) {
    if (!this.debuggerAttached.has(tabId)) return;
    return new Promise(resolve => {
      chrome.debugger.detach({ tabId }, () => {
        this.debuggerAttached.delete(tabId);
        resolve();
      });
    });
  }

  async establishBaseline(session) {
    try {
      await this.captureBaseline(session);
      session.state = 'READY';
      this.notifyDevTools(session.id, 'baseline-captured', session.baseline);
    } catch (error) {
      session.errorLog.push({ error: error.message, timestamp: Date.now() });
      session.state = 'ERROR';
    }
  }

  async captureBaseline(session) {
    const screenshot = await this.captureScreenshot(session.tabId);
    const performanceMetrics = await this.getPerformanceMetrics(session.tabId);
    const networkData = this.networkRequests.get(session.tabId) || [];
    const gpuMetrics = await this.getGPUMetrics(session.tabId);
    session.baseline = {
      timestamp: Date.now(),
      screenshot,
      performanceMetrics,
      networkRequests: [...networkData],
      gpuMetrics,
      url: session.url
    };
  }

  async captureScreenshot(tabId) {
    return new Promise(resolve => {
      chrome.tabs.captureVisibleTab(null, { format: 'png', quality: 90 }, dataUrl => {
        resolve(dataUrl || null);
      });
    });
  }

  async getPerformanceMetrics(tabId) {
    try {
      const metrics = await chrome.debugger.sendCommand({ tabId }, "Performance.getMetrics");
      return {
        timestamp: Date.now(),
        metrics: metrics.metrics || [],
        navigation: await this.getNavigationTiming(tabId)
      };
    } catch (error) {
      return { timestamp: Date.now(), metrics: [], error: error.message };
    }
  }

  async getNavigationTiming(tabId) {
    try {
      const result = await chrome.debugger.sendCommand(
        { tabId },
        "Runtime.evaluate",
        { expression: "JSON.stringify(performance.getEntriesByType('navigation')[0])" }
      );
      return JSON.parse(result.result.value || "{}");
    } catch {
      return {};
    }
  }

  async getGPUMetrics(tabId) {
    try {
      const result = await chrome.debugger.sendCommand(
        { tabId },
        "Runtime.evaluate",
        {
          expression: `
            (() => {
              const c = document.createElement('canvas');
              const gl = c.getContext('webgl') || c.getContext('experimental-webgl');
              const ctx2d = c.getContext('2d');
              return JSON.stringify({
                webglSupported: !!gl,
                canvas2dSupported: !!ctx2d,
                webglContexts: document.querySelectorAll('canvas').length,
                vendor: gl?gl.getParameter(gl.VENDOR):null,
                renderer: gl?gl.getParameter(gl.RENDERER):null,
                version: gl?gl.getParameter(gl.VERSION):null,
                shadingLanguageVersion: gl?gl.getParameter(gl.SHADING_LANGUAGE_VERSION):null,
                maxTextureSize: gl?gl.getParameter(gl.MAX_TEXTURE_SIZE):null,
                maxRenderbufferSize: gl?gl.getParameter(gl.MAX_RENDERBUFFER_SIZE):null
              });
            })()
          `
        }
      );
      return JSON.parse(result.result.value || "{}");
    } catch (error) {
      return { error: error.message };
    }
  }

  async disableFeature(sessionId, feature) {
    const session = this.activeSessions.get(sessionId);
    if (!session) return { success: false, error: 'Session not found' };
    try {
      session.currentTest = feature;
      session.state = 'TESTING';
      const ok = await this.applyFeatureDisable(session.tabId, feature);
      if (ok) {
        session.disabledFeatures.add(feature);
        this.notifyDevTools(sessionId, 'feature-disabled', { feature, session });
        return { success: true };
      }
      return { success: false, error: 'Failed to disable feature' };
    } catch (error) {
      session.errorLog.push({ error: error.message, timestamp: Date.now() });
      return { success: false, error: error.message };
    }
  }

  async enableFeature(sessionId, feature) {
    const session = this.activeSessions.get(sessionId);
    if (!session) return { success: false, error: 'Session not found' };
    try {
      const ok = await this.applyFeatureEnable(session.tabId, feature);
      if (ok) {
        session.disabledFeatures.delete(feature);
        this.notifyDevTools(sessionId, 'feature-enabled', { feature, session });
        return { success: true };
      }
      return { success: false, error: 'Failed to enable feature' };
    } catch (error) {
      session.errorLog.push({ error: error.message, timestamp: Date.now() });
      return { success: false, error: error.message };
    }
  }

  async applyFeatureDisable(tabId, feature) {
    const patterns = this.getBlockingPatterns(feature);
    this.blockedPatterns.set(
      tabId,
      (this.blockedPatterns.get(tabId) || new Set()).add(...patterns)
    );
    switch (feature) {
      case 'javascript':
        return this.disableJavaScript(tabId);
      case 'css':
        return this.blockResourceType(tabId, 'stylesheet');
      case 'images':
        return this.blockResourceType(tabId, 'image');
      case 'fonts':
        return this.blockResourceType(tabId, 'font');
      case 'videos':
        return this.blockResourceType(tabId, 'media');
      case 'analytics':
        return this.blockDomains(tabId, [
          'google-analytics.com',
          'googletagmanager.com',
          'facebook.com'
        ]);
      case 'social-widgets':
        return this.blockDomains(tabId, [
          'facebook.com/plugins',
          'twitter.com/widgets',
          'linkedin.com'
        ]);
      case 'advertising':
        return this.blockDomains(tabId, [
          'googlesyndication.com',
          'doubleclick.net',
          'amazon-adsystem.com'
        ]);
      default:
        return true;
    }
  }

  async applyFeatureEnable(tabId, feature) {
    const patterns = this.getBlockingPatterns(feature);
    const blocked = this.blockedPatterns.get(tabId);
    if (blocked) patterns.forEach(p => blocked.delete(p));
    switch (feature) {
      case 'javascript':
        return this.enableJavaScript(tabId);
      default:
        return true;
    }
  }

  getBlockingPatterns(feature) {
    return {
      javascript: ['*.js','*/js/*','application/javascript'],
      css: ['*.css','*/css/*','text/css'],
      images: ['*.jpg','*.jpeg','*.png','*.gif','*.webp','*.svg'],
      fonts: ['*.woff','*.woff2','*.ttf','*.otf','*.eot'],
      videos: ['*.mp4','*.webm','*.ogg','*.avi'],
      analytics: ['*google-analytics*','*googletagmanager*','*facebook.com/tr*'],
      'social-widgets': ['*facebook.com/plugins*','*twitter.com/widgets*','*linkedin.com*'],
      advertising: ['*googlesyndication*','*doubleclick*','*amazon-adsystem*']
    }[feature] || [];
  }

  async disableJavaScript(tabId) {
    try {
      await chrome.debugger.sendCommand(
        { tabId },
        "Emulation.setScriptExecutionDisabled",
        { value: true }
      );
      return true;
    } catch {
      return false;
    }
  }

  async enableJavaScript(tabId) {
    try {
      await chrome.debugger.sendCommand(
        { tabId },
        "Emulation.setScriptExecutionDisabled",
        { value: false }
      );
      return true;
    } catch {
      return false;
    }
  }

  async blockResourceType(tabId, resourceType) {
    try {
      await chrome.debugger.sendCommand(
        { tabId },
        "Network.setBlockedURLs",
        { urls: ["*"] }
      );
      return true;
    } catch {
      return false;
    }
  }

  async blockDomains(tabId, domains) {
    try {
      const patterns = domains.map(d => `*://${d}/*`);
      await chrome.debugger.sendCommand(
        { tabId },
        "Network.setBlockedURLs",
        { urls: patterns }
      );
      return true;
    } catch {
      return false;
    }
  }

  async restoreAllFeatures(session) {
    for (const f of session.disabledFeatures) {
      await this.applyFeatureEnable(session.tabId, f);
    }
    session.disabledFeatures.clear();
    this.blockedPatterns.delete(session.tabId);
  }

  handleBeforeRequest(details) {
    const blocked = this.blockedPatterns.get(details.tabId);
    if (blocked && this.shouldBlockRequest(details, blocked)) {
      return { cancel: true };
    }
    this.recordNetworkRequest(details);
    return {};
  }

  shouldBlockRequest(details, blockedPatterns) {
    const url = details.url.toLowerCase();
    for (const p of blockedPatterns) {
      if (new RegExp(p.replace(/\*/g, '.*'),'i').test(url)) {
        return true;
      }
    }
    return false;
  }

  recordNetworkRequest(details) {
    const requests = this.networkRequests.get(details.tabId) || [];
    requests.push({
      id: details.requestId,
      url: details.url,
      method: details.method,
      type: details.type,
      timestamp: Date.now(),
      tabId: details.tabId,
      frameId: details.frameId,
      parentFrameId: details.parentFrameId,
      initiator: details.initiator
    });
    this.networkRequests.set(details.tabId, requests);
    this.notifyDevTools(this.getSessionByTabId(details.tabId), 'network-request', requests.at(-1));
  }

  handleRequestCompleted(details) {
    const requests = this.networkRequests.get(details.tabId);
    if (requests) {
      const req = requests.find(r => r.id === details.requestId);
      if (req) {
        req.status = details.statusCode;
        req.responseHeaders = details.responseHeaders;
        req.completedTimestamp = Date.now();
        req.duration = req.completedTimestamp - req.timestamp;
      }
    }
  }

  handleRequestError(details) {
    const requests = this.networkRequests.get(details.tabId);
    if (requests) {
      const req = requests.find(r => r.id === details.requestId);
      if (req) {
        req.error = details.error;
        req.errorTimestamp = Date.now();
      }
    }
  }

  handleDebuggerEvent(source, method, params) {
    const session = this.getSessionDataByTabId(source.tabId);
    if (!session) return;
    switch (method) {
      case 'Runtime.consoleAPICalled':
        this.handleConsoleMessage(session, params);
        break;
      case 'Runtime.exceptionThrown':
        this.handleException(session, params);
        break;
      case 'Performance.metrics':
        this.handlePerformanceMetrics(session, params);
        break;
    }
  }

  handleConsoleMessage(session, params) {
    const entry = {
      timestamp: Date.now(),
      level: params.type,
      text: params.args.map(a => a.value||a.description||'').join(' '),
      source: params.stackTrace?.callFrames[0]||null
    };
    session.consoleLogs.push(entry);
    this.notifyDevTools(session.id, 'console-message', entry);
  }

  handleException(session, params) {
    const err = params.exceptionDetails;
    const entry = {
      timestamp: Date.now(),
      message: err.text,
      source: err.url,
      line: err.lineNumber,
      column: err.columnNumber,
      stack: err.stackTrace
    };
    session.errorLog.push(entry);
    this.notifyDevTools(session.id, 'exception', entry);
  }

  handlePerformanceMetrics(session, params) {
    session.performanceMetrics.push({
      timestamp: Date.now(),
      metrics: params.metrics
    });
  }

  handleDebuggerDetach(source, reason) {
    this.debuggerAttached.delete(source.tabId);
    const session = this.getSessionDataByTabId(source.tabId);
    if (session) {
      session.state = 'ERROR';
      session.errorLog.push({ error: `Debugger detached: ${reason}`, timestamp: Date.now() });
    }
  }

  handleTabLoad(tabId, url) {
    const session = this.getSessionDataByTabId(tabId);
    if (session && session.state === 'TESTING') {
      setTimeout(() => this.captureTestState(session), 2000);
    }
  }

  async captureTestState(session) {
    try {
      const screenshot = await this.captureScreenshot(session.tabId);
      const performanceMetrics = await this.getPerformanceMetrics(session.tabId);
      const networkData = this.networkRequests.get(session.tabId) || [];
      const gpuMetrics = await this.getGPUMetrics(session.tabId);
      const testState = {
        timestamp: Date.now(),
        feature: session.currentTest,
        screenshot,
        performanceMetrics,
        networkRequests: [...networkData],
        gpuMetrics,
        consoleLogs: [...session.consoleLogs]
      };
      this.notifyDevTools(session.id, 'test-state-captured', testState);
    } catch (error) {
      session.errorLog.push({ error: error.message, timestamp: Date.now() });
    }
  }

  cleanupSession(tabId) {
    this.networkRequests.delete(tabId);
    this.blockedPatterns.delete(tabId);
    if (this.debuggerAttached.has(tabId)) this.detachDebugger(tabId);
    for (const [id, s] of this.activeSessions) {
      if (s.tabId === tabId) {
        this.activeSessions.delete(id);
        break;
      }
    }
  }

  getSessionByTabId(tabId) {
    for (const [id, s] of this.activeSessions) {
      if (s.tabId === tabId) return id;
    }
    return null;
  }

  getSessionDataByTabId(tabId) {
    for (const s of this.activeSessions.values()) {
      if (s.tabId === tabId) return s;
    }
    return null;
  }

  notifyDevTools(sessionId, type, data) {
    chrome.runtime.sendMessage({
      target: 'devtools',
      sessionId,
      type,
      data,
      timestamp: Date.now()
    }).catch(() => {});
  }

  extractBaseDomain(url) {
    try {
      const h = new URL(url).hostname.replace(/^www\./,'');
      const parts = h.split('.');
      return parts.length > 2 ? parts.slice(-2).join('.') : h;
    } catch {
      return 'unknown';
    }
  }

  async saveDomainSettings(url, settings) {
    const d = this.extractBaseDomain(url);
    const key = `domain_settings_${d}`;
    const now = Date.now();
    const existing = await chrome.storage.local.get([key]);
    const history = existing[key]?.testHistory||[];
    history.push({ timestamp: now, settings });
    if (history.length>10) history.splice(0, history.length-10);
    const domainData = { domain:d, lastTested:now, settings, testHistory:history };
    await chrome.storage.local.set({ [key]: domainData });
    return domainData;
  }

  async loadDomainSettings(url) {
    const d = this.extractBaseDomain(url);
    const key = `domain_settings_${d}`;
    const res = await chrome.storage.local.get([key]);
    return res[key]||null;
  }

  async testResourceBlocking(sessionId, feature) {
    const session = this.activeSessions.get(sessionId);
    if (!session) return { success:false, error:'Session not found' };
    try {
      await this.clearPageData(session.tabId, session.url);
      await this.disableFeature(sessionId, feature);
      await chrome.tabs.reload(session.tabId);
      await this.saveDomainSettings(session.url, {
        blockedFeature:feature, timestamp:Date.now(), sessionId
      });
      session.currentTest = feature;
      session.lastActivity = Date.now();
      this.notifyDevTools(sessionId, 'test-started', { feature, session });
      return { success:true, feature };
    } catch (error) {
      session.errorLog.push({ error:error.message, timestamp:Date.now() });
      return { success:false, error:error.message };
    }
  }

  async clearPageData(tabId, url) {
    try {
      const origin = new URL(url).origin;
      const domain = new URL(url).hostname;
      const cookies = await chrome.cookies.getAll({ domain });
      for (const c of cookies) {
        await chrome.cookies.remove({
          url: `http${c.secure?'s':''}://${c.domain}${c.path}`,
          name: c.name
        });
      }
      if (this.debuggerAttached.has(tabId)) {
        await chrome.debugger.sendCommand(
          { tabId },
          "Storage.clearDataForOrigin",
          { origin, storageTypes:"local_storage,session_storage,cache_storage,indexeddb,websql" }
        );
      }
      return true;
    } catch (error) {
      console.warn('Failed to clear page data:', error);
      return false;
    }
  }

  async handleMessage(message, sender, sendResponse) {
    (async () => {
      try {
        switch (message.action) {
          case 'start-session': {
            const startResult = await this.startSession(
              sender.tab?.id || message.tabId,
              message.url
            );
            sendResponse(startResult);
            break;
          }
          case 'stop-session': {
            const stopResult = await this.stopSession(message.sessionId);
            sendResponse(stopResult);
            break;
          }
          case 'disable-feature': {
            const disableResult = await this.disableFeature(
              message.sessionId,
              message.feature
            );
            sendResponse(disableResult);
            break;
          }
          case 'enable-feature': {
            const enableResult = await this.enableFeature(
              message.sessionId,
              message.feature
            );
            sendResponse(enableResult);
            break;
          }
          case 'get-session': {
            const sessionData = message.sessionId
              ? this.activeSessions.get(message.sessionId)
              : this.getSessionDataByTabId(message.tabId);
            sendResponse({ success: !!sessionData, session: sessionData });
            break;
          }
          case 'capture-screenshot': {
            const screenshot = await this.captureScreenshot(
              message.tabId || sender.tab?.id
            );
            sendResponse({ screenshot });
            break;
          }
          case 'reload-and-capture': {
            await this.reloadAndCapture(message.sessionId);
            sendResponse({ success: true });
            break;
          }
          default:
            sendResponse({ success: false, error: 'Unknown action' });
        }
      } catch (error) {
        sendResponse({ success: false, error: error.message });
      }
    })();
    return true;
  }

  async reloadAndCapture(sessionId) {
    const session = this.activeSessions.get(sessionId);
    if (!session) return;
    chrome.tabs.reload(session.tabId);
    setTimeout(() => this.captureTestState(session), 3000);
  }
}

const analyzer = new UltimateWebsiteAnalyzer();
