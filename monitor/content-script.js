class PageMonitor {
  constructor() {
    this.observers = [];
    this.performanceData = [];
    this.resourceTimings = [];
    this.gpuContexts = [];
    this.canvasOperations = [];
    this.isMonitoring = false;
    this.injectedScript = null;
    this.init();
  }

  init() {
    if (document.readyState === 'loading') {
      document.addEventListener('DOMContentLoaded', () => this.startMonitoring());
    } else {
      this.startMonitoring();
    }
    window.addEventListener('load', () => this.captureLoadMetrics());
    this.setupMessageHandling();
    this.injectDeepMonitor();
  }

  startMonitoring() {
    this.isMonitoring = true;
    this.setupDOMObserver();
    this.setupPerformanceObserver();
    this.setupResourceObserver();
    this.monitorGPUUsage();
    this.captureInitialState();
  }

  setupDOMObserver() {
    const domObserver = new MutationObserver(mutations => {
      this.processDOMMutations(mutations);
    });
    domObserver.observe(document.body || document.documentElement, {
      childList: true,
      subtree: true,
      attributes: true,
      attributeOldValue: true,
      characterData: true
    });
    this.observers.push(domObserver);
  }

  setupPerformanceObserver() {
    if ('PerformanceObserver' in window) {
      try {
        const perfObserver = new PerformanceObserver(list => {
          this.processPerformanceEntries(list.getEntries());
        });
        perfObserver.observe({
          entryTypes: [
            'navigation',
            'resource',
            'paint',
            'largest-contentful-paint',
            'first-input',
            'layout-shift'
          ]
        });
        this.observers.push(perfObserver);
      } catch (e) {
        // some browsers may throw on certain entry types
      }
    }
  }

  setupResourceObserver() {
    // Intercept fetch
    const originalFetch = window.fetch;
    window.fetch = (...args) => {
      this.trackFetchRequest(args[0], args[1]);
      return originalFetch.apply(this, args);
    };

    // Intercept XHR
    const originalXHROpen = XMLHttpRequest.prototype.open;
    XMLHttpRequest.prototype.open = function(method, url, ...rest) {
      this._monitorURL = url;
      this._monitorMethod = method;
      return originalXHROpen.apply(this, [method, url, ...rest]);
    };
    const originalXHRSend = XMLHttpRequest.prototype.send;
    XMLHttpRequest.prototype.send = function(...args) {
      if (this._monitorURL) {
        window.pageMonitor.trackXHRRequest(this._monitorMethod, this._monitorURL);
      }
      return originalXHRSend.apply(this, args);
    };

    window.pageMonitor = this;
  }

  monitorGPUUsage() {
    this.interceptWebGLContexts();
    this.interceptCanvasContexts();
    this.monitorExistingCanvases();
  }

  interceptWebGLContexts() {
    const orig = HTMLCanvasElement.prototype.getContext;
    HTMLCanvasElement.prototype.getContext = function(type, ...args) {
      const ctx = orig.apply(this, [type, ...args]);
      if (type === 'webgl' || type === 'experimental-webgl' || type === 'webgl2') {
        window.pageMonitor.registerWebGLContext(this, ctx, type);
      } else if (type === '2d') {
        window.pageMonitor.registerCanvas2DContext(this, ctx);
      }
      return ctx;
    };
  }

  interceptCanvasContexts() {
    const webglMethods = [
      'drawArrays', 'drawElements', 'useProgram', 'bindTexture',
      'texImage2D', 'bufferData', 'vertexAttribPointer', 'enableVertexAttribArray'
    ];
    const canvas2DMethods = [
      'drawImage', 'fillRect', 'strokeRect',
      'fillText', 'strokeText', 'putImageData',
      'createImageData', 'getImageData'
    ];
    this.interceptMethods(WebGLRenderingContext.prototype, webglMethods, 'webgl');
    if (window.WebGL2RenderingContext) {
      this.interceptMethods(WebGL2RenderingContext.prototype, webglMethods, 'webgl2');
    }
    this.interceptMethods(CanvasRenderingContext2D.prototype, canvas2DMethods, '2d');
  }

  interceptMethods(proto, methods, contextType) {
    methods.forEach(m => {
      if (proto[m]) {
        const orig = proto[m];
        proto[m] = function(...args) {
          window.pageMonitor.trackCanvasOperation(contextType, m, args);
          return orig.apply(this, args);
        };
      }
    });
  }

  registerWebGLContext(canvas, context, type) {
    const info = {
      canvas, context, type,
      createdAt: Date.now(),
      operations: 0,
      lastActivity: Date.now()
    };
    this.gpuContexts.push(info);
    this.sendToBackground('webgl-context-created', info);
  }

  registerCanvas2DContext(canvas, context) {
    const info = {
      canvas, context,
      type: '2d',
      createdAt: Date.now(),
      operations: 0,
      lastActivity: Date.now()
    };
    this.sendToBackground('canvas-2d-context-created', info);
  }

  trackCanvasOperation(contextType, method, args) {
    const op = {
      contextType,
      method,
      timestamp: Date.now(),
      argsLength: args.length
    };
    this.canvasOperations.push(op);
    if (this.canvasOperations.length > 1000) {
      this.canvasOperations = this.canvasOperations.slice(-500);
    }
    this.sendToBackground('canvas-operation', op);
  }

  monitorExistingCanvases() {
    document.querySelectorAll('canvas').forEach(canvas => {
      const webgl = canvas.getContext('webgl') ||
                    canvas.getContext('experimental-webgl') ||
                    (window.WebGL2RenderingContext && canvas.getContext('webgl2'));
      const ctx2d = canvas.getContext('2d');
      if (webgl) {
        this.registerWebGLContext(canvas, webgl, webgl.constructor.name.toLowerCase());
      } else if (ctx2d) {
        this.registerCanvas2DContext(canvas, ctx2d);
      }
    });
  }

  processDOMMutations(muts) {
    const changes = {
      timestamp: Date.now(),
      addedNodes: 0,
      removedNodes: 0,
      attributeChanges: 0,
      textChanges: 0
    };
    muts.forEach(m => {
      if (m.type === 'childList') {
        changes.addedNodes += m.addedNodes.length;
        changes.removedNodes += m.removedNodes.length;
      } else if (m.type === 'attributes') {
        changes.attributeChanges++;
      } else if (m.type === 'characterData') {
        changes.textChanges++;
      }
    });
    if (changes.addedNodes || changes.removedNodes || changes.attributeChanges) {
      this.sendToBackground('dom-mutation', changes);
    }
  }

  processPerformanceEntries(entries) {
    entries.forEach(entry => {
      const data = {
        timestamp: Date.now(),
        name: entry.name,
        entryType: entry.entryType,
        startTime: entry.startTime,
        duration: entry.duration
      };
      if (entry.entryType === 'resource') {
        data.transferSize = entry.transferSize;
        data.encodedBodySize = entry.encodedBodySize;
        data.decodedBodySize = entry.decodedBodySize;
        data.initiatorType = entry.initiatorType;
      }
      if (entry.entryType === 'largest-contentful-paint') {
        data.size = entry.size;
        data.element = entry.element?.tagName || null;
      }
      if (entry.entryType === 'layout-shift') {
        data.value = entry.value;
        data.hadRecentInput = entry.hadRecentInput;
      }
      this.performanceData.push(data);
      this.sendToBackground('performance-entry', data);
    });
  }

  trackFetchRequest(url, options) {
    const info = {
      timestamp: Date.now(),
      type: 'fetch',
      url: typeof url === 'string' ? url : url.url,
      method: options?.method || 'GET'
    };
    this.sendToBackground('fetch-request', info);
  }

  trackXHRRequest(method, url) {
    const info = {
      timestamp: Date.now(),
      type: 'xhr',
      url,
      method
    };
    this.sendToBackground('xhr-request', info);
  }

  captureInitialState() {
    const state = {
      timestamp: Date.now(),
      url: location.href,
      title: document.title,
      readyState: document.readyState,
      domStats: {
        totalElements: document.querySelectorAll('*').length,
        scripts: document.querySelectorAll('script').length,
        stylesheets: document.querySelectorAll('link[rel="stylesheet"], style').length,
        images: document.querySelectorAll('img').length,
        canvases: document.querySelectorAll('canvas').length,
        videos: document.querySelectorAll('video').length,
        audios: document.querySelectorAll('audio').length,
        iframes: document.querySelectorAll('iframe').length
      },
      viewport: {
        width: innerWidth,
        height: innerHeight,
        devicePixelRatio: devicePixelRatio,
        orientation: screen.orientation?.type || null
      },
      memoryInfo: performance.memory ? {
        usedJSHeapSize: performance.memory.usedJSHeapSize,
        totalJSHeapSize: performance.memory.totalJSHeapSize,
        jsHeapSizeLimit: performance.memory.jsHeapSizeLimit
      } : null,
      connectionInfo: navigator.connection ? {
        effectiveType: navigator.connection.effectiveType,
        downlink: navigator.connection.downlink,
        rtt: navigator.connection.rtt
      } : null
    };
    this.sendToBackground('initial-state', state);
  }

  captureLoadMetrics() {
    const metrics = {
      timestamp: Date.now(),
      loadComplete: true,
      timing: performance.timing ? {
        navigationStart: performance.timing.navigationStart,
        domContentLoadedEventEnd: performance.timing.domContentLoadedEventEnd,
        loadEventEnd: performance.timing.loadEventEnd,
        domComplete: performance.timing.domComplete
      } : null,
      navigation: performance.getEntriesByType('navigation')[0] || null,
      resources: performance.getEntriesByType('resource').map(r => ({
        name: r.name,
        duration: r.duration,
        transferSize: r.transferSize,
        initiatorType: r.initiatorType,
        nextHopProtocol: r.nextHopProtocol
      })),
      paint: performance.getEntriesByType('paint').reduce((acc, p) => {
        acc[p.name] = p.startTime;
        return acc;
      }, {}),
      memoryUsage: performance.memory ? {
        usedJSHeapSize: performance.memory.usedJSHeapSize,
        totalJSHeapSize: performance.memory.totalJSHeapSize
      } : null
    };
    this.sendToBackground('load-metrics', metrics);
  }

  injectDeepMonitor() {
    const script = document.createElement('script');
    script.src = chrome.runtime.getURL('injected-script.js');
    script.onload = () => script.remove();
    (document.head || document.documentElement).appendChild(script);
  }

  getCurrentState() {
    return {
      timestamp: Date.now(),
      url: location.href,
      isMonitoring: this.isMonitoring,
      performanceData: this.performanceData.slice(-100),
      gpuContexts: this.gpuContexts.length,
      canvasOperations: this.canvasOperations.slice(-50),
      domElementCount: document.querySelectorAll('*').length,
      memoryUsage: performance.memory ? {
        usedJSHeapSize: performance.memory.usedJSHeapSize,
        totalJSHeapSize: performance.memory.totalJSHeapSize
      } : null
    };
  }

  setupMessageHandling() {
    chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
      switch (message.action) {
        case 'get-current-state':
          sendResponse(this.getCurrentState());
          break;
        case 'capture-metrics':
          this.captureLoadMetrics();
          sendResponse({ success: true });
          break;
        case 'start-monitoring':
          this.startMonitoring();
          sendResponse({ success: true });
          break;
        case 'stop-monitoring':
          this.stopMonitoring();
          sendResponse({ success: true });
          break;
        case 'open-devtools':
        case 'show-devtools-reminder':
          this.showDevToolsReminder();
          sendResponse({ success: true });
          break;
        default:
          sendResponse({ success: false, error: 'Unknown action' });
      }
      return true;
    });

    window.addEventListener('message', event => {
      if (event.source === window && event.data.source === 'injected-script') {
        this.sendToBackground('injected-data', event.data);
      }
    });
  }

  sendToBackground(type, data) {
    chrome.runtime.sendMessage({
      source: 'content-script',
      type,
      data,
      url: location.href,
      timestamp: Date.now()
    }).catch(() => {});
  }

  stopMonitoring() {
    this.isMonitoring = false;
    this.observers.forEach(obs => {
      try { obs.disconnect(); } catch {}
    });
    this.observers = [];
  }

  cleanup() {
    this.stopMonitoring();
    this.performanceData = [];
    this.resourceTimings = [];
    this.gpuContexts = [];
    this.canvasOperations = [];
  }

  showDevToolsReminder() {
    const overlay = document.createElement('div');
    overlay.style.cssText = `
      position: fixed;
      top: 20px; right: 20px;
      background: linear-gradient(135deg,#667eea,#764ba2);
      color: white; padding: 16px 20px;
      border-radius: 8px; box-shadow: 0 4px 12px rgba(0,0,0,0.3);
      z-index: 999999; font-size: 14px;
      animation: slideInRight 0.3s ease-out;
    `;
    overlay.innerHTML = `
      <div style="display:flex;align-items:center;gap:12px">
        <div style="font-size:24px">🚀</div>
        <div>
          <div style="font-weight:600;margin-bottom:4px">Website Analysis Active</div>
          <div style="font-size:12px;opacity:0.9">
            Press F12 → Website Analyzer tab to view progress
          </div>
        </div>
        <button id="analyzer-reminder-close" style="
          background:rgba(255,255,255,0.2); border:none;
          color:white; width:24px; height:24px;
          border-radius:50%; cursor:pointer; font-size:16px;
          display:flex; align-items:center; justify-content:center;
        ">×</button>
      </div>
    `;
    const style = document.createElement('style');
    style.textContent = `
      @keyframes slideInRight {
        from { transform: translateX(100%); opacity: 0; }
        to   { transform: translateX(0);    opacity: 1; }
      }
    `;
    document.head.appendChild(style);
    document.body.appendChild(overlay);
    document
      .querySelector('#analyzer-reminder-close')
      .addEventListener('click', () => {
        overlay.remove();
        style.remove();
      });
    setTimeout(() => {
      overlay.remove();
      style.remove();
    }, 10000);

    console.log(
      '%c🚀 Website Analyzer Active',
      'background:#667eea;color:white;padding:8px 12px;border-radius:4px;font-weight:bold;font-size:14px;'
    );
    console.log(
      '%cOpen DevTools (F12) → Website Analyzer tab to view analysis progress',
      'color:#667eea;font-weight:500;'
    );
  }
}

window.addEventListener('beforeunload', () => {
  if (window.pageMonitor) {
    window.pageMonitor.cleanup();
  }
});

const pageMonitor = new PageMonitor();
