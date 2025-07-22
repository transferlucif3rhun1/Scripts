(() => {
  'use strict';

  class DeepPageMonitor {
    constructor() {
      this.webglStats = {
        contexts: 0,
        drawCalls: 0,
        bufferUploads: 0,
        textureUploads: 0,
        shaderCompilations: 0,
        programLinks: 0
      };

      this.canvasStats = {
        contexts: 0,
        drawOperations: 0,
        imageOperations: 0,
        textOperations: 0,
        pixelManipulations: 0
      };

      this.performanceStats = {
        frameTimes: [],
        memoryUsage: [],
        cpuUsage: []
      };

      this.networkStats = {
        requests: 0,
        bytesTransferred: 0,
        connections: new Map()
      };

      this.init();
    }

    init() {
      this.interceptWebGLAPIs();
      this.interceptCanvasAPIs();
      this.interceptNetworkAPIs();
      this.interceptStorageAPIs();
      this.interceptMediaAPIs();
      this.startPerformanceMonitoring();
      this.interceptErrorHandling();
    }

    interceptWebGLAPIs() {
      const webglPrototype = WebGLRenderingContext.prototype;
      const webgl2Prototype = window.WebGL2RenderingContext?.prototype;

      const interceptWebGLMethod = (prototype, methodName, statProperty) => {
        if (prototype && prototype[methodName]) {
          const original = prototype[methodName];
          prototype[methodName] = function(...args) {
            window.deepMonitor?.webglStats[statProperty]++;
            window.deepMonitor?.trackWebGLCall(methodName, args);
            return original.apply(this, args);
          };
        }
      };

      const webglMethods = [
        ['drawArrays', 'drawCalls'],
        ['drawElements', 'drawCalls'],
        ['bufferData', 'bufferUploads'],
        ['bufferSubData', 'bufferUploads'],
        ['texImage2D', 'textureUploads'],
        ['texSubImage2D', 'textureUploads'],
        ['compileShader', 'shaderCompilations'],
        ['linkProgram', 'programLinks']
      ];

      webglMethods.forEach(([method, stat]) => {
        interceptWebGLMethod(webglPrototype, method, stat);
        if (webgl2Prototype) {
          interceptWebGLMethod(webgl2Prototype, method, stat);
        }
      });

      const originalGetContext = HTMLCanvasElement.prototype.getContext;
      HTMLCanvasElement.prototype.getContext = function(contextType, ...args) {
        const context = originalGetContext.apply(this, arguments);
        
        if (context && (contextType === 'webgl' || contextType === 'experimental-webgl' || contextType === 'webgl2')) {
          window.deepMonitor?.webglStats.contexts++;
          window.deepMonitor?.analyzeWebGLContext(this, context, contextType);
        } else if (context && contextType === '2d') {
          window.deepMonitor?.canvasStats.contexts++;
          window.deepMonitor?.analyzeCanvas2DContext(this, context);
        }
        
        return context;
      };
    }

    interceptCanvasAPIs() {
      const canvas2DPrototype = CanvasRenderingContext2D.prototype;

      const interceptCanvas2DMethod = (methodName, statProperty) => {
        if (canvas2DPrototype[methodName]) {
          const original = canvas2DPrototype[methodName];
          canvas2DPrototype[methodName] = function(...args) {
            window.deepMonitor?.canvasStats[statProperty]++;
            window.deepMonitor?.trackCanvas2DCall(methodName, args);
            return original.apply(this, args);
          };
        }
      };

      const canvas2DMethods = [
        ['fillRect', 'drawOperations'],
        ['strokeRect', 'drawOperations'],
        ['fillText', 'textOperations'],
        ['strokeText', 'textOperations'],
        ['drawImage', 'imageOperations'],
        ['putImageData', 'pixelManipulations'],
        ['getImageData', 'pixelManipulations'],
        ['createImageData', 'pixelManipulations']
      ];

      canvas2DMethods.forEach(([method, stat]) => {
        interceptCanvas2DMethod(method, stat);
      });
    }

    interceptNetworkAPIs() {
      const originalFetch = window.fetch;
      window.fetch = async (...args) => {
        const startTime = performance.now();
        window.deepMonitor?.networkStats.requests++;
        
        try {
          const response = await originalFetch.apply(window, args);
          const endTime = performance.now();
          
          window.deepMonitor?.trackNetworkRequest({
            type: 'fetch',
            url: args[0],
            duration: endTime - startTime,
            status: response.status,
            size: response.headers.get('content-length'),
            timestamp: Date.now()
          });
          
          return response;
        } catch (error) {
          window.deepMonitor?.trackNetworkError({
            type: 'fetch',
            url: args[0],
            error: error.message,
            timestamp: Date.now()
          });
          throw error;
        }
      };

      const originalXMLHttpRequest = window.XMLHttpRequest;
      window.XMLHttpRequest = function() {
        const xhr = new originalXMLHttpRequest();
        const originalSend = xhr.send;
        const originalOpen = xhr.open;
        
        let requestData = {};
        
        xhr.open = function(method, url, ...args) {
          requestData = { method, url, startTime: performance.now() };
          return originalOpen.call(this, method, url, ...args);
        };
        
        xhr.send = function(...args) {
          window.deepMonitor?.networkStats.requests++;
          
          xhr.addEventListener('loadend', () => {
            const endTime = performance.now();
            window.deepMonitor?.trackNetworkRequest({
              type: 'xhr',
              method: requestData.method,
              url: requestData.url,
              duration: endTime - requestData.startTime,
              status: xhr.status,
              responseSize: xhr.responseText?.length || 0,
              timestamp: Date.now()
            });
          });
          
          return originalSend.apply(this, args);
        };
        
        return xhr;
      };

      Object.setPrototypeOf(window.XMLHttpRequest, originalXMLHttpRequest);
      Object.setPrototypeOf(window.XMLHttpRequest.prototype, originalXMLHttpRequest.prototype);
    }

    interceptStorageAPIs() {
      const storageOperations = ['setItem', 'getItem', 'removeItem', 'clear'];
      
      [localStorage, sessionStorage].forEach(storage => {
        const storageType = storage === localStorage ? 'localStorage' : 'sessionStorage';
        
        storageOperations.forEach(operation => {
          const original = storage[operation];
          storage[operation] = function(...args) {
            window.deepMonitor?.trackStorageOperation({
              type: storageType,
              operation: operation,
              key: args[0],
              valueSize: args[1]?.length || 0,
              timestamp: Date.now()
            });
            return original.apply(this, args);
          };
        });
      });

      if (window.indexedDB) {
        const originalOpen = window.indexedDB.open;
        window.indexedDB.open = function(...args) {
          window.deepMonitor?.trackDatabaseOperation({
            type: 'indexedDB',
            operation: 'open',
            database: args[0],
            timestamp: Date.now()
          });
          return originalOpen.apply(this, args);
        };
      }
    }

    interceptMediaAPIs() {
      const originalGetUserMedia = navigator.mediaDevices?.getUserMedia;
      if (originalGetUserMedia) {
        navigator.mediaDevices.getUserMedia = async function(constraints) {
          window.deepMonitor?.trackMediaAccess({
            type: 'getUserMedia',
            constraints: constraints,
            timestamp: Date.now()
          });
          return originalGetUserMedia.call(this, constraints);
        };
      }

      const originalPlay = HTMLVideoElement.prototype.play;
      HTMLVideoElement.prototype.play = function() {
        window.deepMonitor?.trackMediaOperation({
          type: 'video',
          operation: 'play',
          src: this.src,
          duration: this.duration,
          timestamp: Date.now()
        });
        return originalPlay.call(this);
      };

      const originalAudioPlay = HTMLAudioElement.prototype.play;
      HTMLAudioElement.prototype.play = function() {
        window.deepMonitor?.trackMediaOperation({
          type: 'audio',
          operation: 'play',
          src: this.src,
          duration: this.duration,
          timestamp: Date.now()
        });
        return originalAudioPlay.call(this);
      };
    }

    interceptErrorHandling() {
      const originalOnError = window.onerror;
      window.onerror = function(message, source, lineno, colno, error) {
        window.deepMonitor?.trackError({
          type: 'javascript',
          message: message,
          source: source,
          line: lineno,
          column: colno,
          stack: error?.stack,
          timestamp: Date.now()
        });
        
        if (originalOnError) {
          return originalOnError.apply(this, arguments);
        }
      };

      const originalOnUnhandledRejection = window.onunhandledrejection;
      window.onunhandledrejection = function(event) {
        window.deepMonitor?.trackError({
          type: 'promise',
          message: event.reason?.message || event.reason,
          stack: event.reason?.stack,
          timestamp: Date.now()
        });
        
        if (originalOnUnhandledRejection) {
          return originalOnUnhandledRejection.call(this, event);
        }
      };
    }

    startPerformanceMonitoring() {
      let frameCount = 0;
      let lastFrameTime = performance.now();
      
      const measureFrame = () => {
        const currentTime = performance.now();
        const frameDuration = currentTime - lastFrameTime;
        
        frameCount++;
        this.performanceStats.frameTimes.push(frameDuration);
        
        if (this.performanceStats.frameTimes.length > 60) {
          this.performanceStats.frameTimes.shift();
        }
        
        if (frameCount % 60 === 0) {
          this.collectPerformanceMetrics();
        }
        
        lastFrameTime = currentTime;
        requestAnimationFrame(measureFrame);
      };
      
      requestAnimationFrame(measureFrame);
      
      setInterval(() => {
        this.collectPerformanceMetrics();
      }, 5000);
    }

    collectPerformanceMetrics() {
      const metrics = {
        timestamp: Date.now(),
        memory: performance.memory ? {
          usedJSHeapSize: performance.memory.usedJSHeapSize,
          totalJSHeapSize: performance.memory.totalJSHeapSize,
          jsHeapSizeLimit: performance.memory.jsHeapSizeLimit
        } : null,
        timing: performance.timing ? {
          navigationStart: performance.timing.navigationStart,
          loadEventEnd: performance.timing.loadEventEnd,
          domContentLoadedEventEnd: performance.timing.domContentLoadedEventEnd
        } : null,
        navigation: performance.getEntriesByType('navigation')[0] || null,
        frameStats: {
          averageFrameTime: this.getAverageFrameTime(),
          minFrameTime: Math.min(...this.performanceStats.frameTimes),
          maxFrameTime: Math.max(...this.performanceStats.frameTimes),
          fps: this.calculateFPS()
        },
        webglStats: { ...this.webglStats },
        canvasStats: { ...this.canvasStats },
        networkStats: {
          requests: this.networkStats.requests,
          bytesTransferred: this.networkStats.bytesTransferred
        }
      };

      this.sendToContentScript('performance-metrics', metrics);
    }

    getAverageFrameTime() {
      if (this.performanceStats.frameTimes.length === 0) return 0;
      const sum = this.performanceStats.frameTimes.reduce((a, b) => a + b, 0);
      return sum / this.performanceStats.frameTimes.length;
    }

    calculateFPS() {
      const avgFrameTime = this.getAverageFrameTime();
      return avgFrameTime > 0 ? 1000 / avgFrameTime : 0;
    }

    analyzeWebGLContext(canvas, context, type) {
      const contextInfo = {
        canvas: {
          width: canvas.width,
          height: canvas.height,
          id: canvas.id,
          className: canvas.className
        },
        contextType: type,
        vendor: context.getParameter(context.VENDOR),
        renderer: context.getParameter(context.RENDERER),
        version: context.getParameter(context.VERSION),
        shadingLanguageVersion: context.getParameter(context.SHADING_LANGUAGE_VERSION),
        maxTextureSize: context.getParameter(context.MAX_TEXTURE_SIZE),
        maxRenderbufferSize: context.getParameter(context.MAX_RENDERBUFFER_SIZE),
        maxViewportDims: context.getParameter(context.MAX_VIEWPORT_DIMS),
        maxVertexAttribs: context.getParameter(context.MAX_VERTEX_ATTRIBS),
        createdAt: Date.now()
      };

      this.sendToContentScript('webgl-context-analyzed', contextInfo);
    }

    analyzeCanvas2DContext(canvas, context) {
      const contextInfo = {
        canvas: {
          width: canvas.width,
          height: canvas.height,
          id: canvas.id,
          className: canvas.className
        },
        contextType: '2d',
        imageSmoothingEnabled: context.imageSmoothingEnabled,
        globalAlpha: context.globalAlpha,
        globalCompositeOperation: context.globalCompositeOperation,
        createdAt: Date.now()
      };

      this.sendToContentScript('canvas-2d-context-analyzed', contextInfo);
    }

    trackWebGLCall(methodName, args) {
      const callData = {
        method: methodName,
        argsCount: args.length,
        timestamp: Date.now()
      };

      if (methodName === 'drawArrays' || methodName === 'drawElements') {
        callData.drawMode = args[0];
        callData.count = args[1];
      } else if (methodName === 'bufferData') {
        callData.target = args[0];
        callData.usage = args[2];
        callData.size = args[1]?.byteLength || 0;
      } else if (methodName === 'texImage2D') {
        callData.target = args[0];
        callData.level = args[1];
        callData.format = args[2];
      }

      this.sendToContentScript('webgl-call', callData);
    }

    trackCanvas2DCall(methodName, args) {
      const callData = {
        method: methodName,
        argsCount: args.length,
        timestamp: Date.now()
      };

      if (methodName === 'fillRect' || methodName === 'strokeRect') {
        callData.dimensions = { x: args[0], y: args[1], width: args[2], height: args[3] };
      } else if (methodName === 'fillText' || methodName === 'strokeText') {
        callData.textLength = args[0]?.length || 0;
      } else if (methodName === 'drawImage') {
        callData.imageType = args[0]?.constructor?.name;
      }

      this.sendToContentScript('canvas-2d-call', callData);
    }

    trackNetworkRequest(requestData) {
      this.networkStats.bytesTransferred += requestData.size || requestData.responseSize || 0;
      this.sendToContentScript('network-request-deep', requestData);
    }

    trackNetworkError(errorData) {
      this.sendToContentScript('network-error-deep', errorData);
    }

    trackStorageOperation(operationData) {
      this.sendToContentScript('storage-operation', operationData);
    }

    trackDatabaseOperation(operationData) {
      this.sendToContentScript('database-operation', operationData);
    }

    trackMediaAccess(accessData) {
      this.sendToContentScript('media-access', accessData);
    }

    trackMediaOperation(operationData) {
      this.sendToContentScript('media-operation', operationData);
    }

    trackError(errorData) {
      this.sendToContentScript('error-deep', errorData);
    }

    sendToContentScript(type, data) {
      window.postMessage({
        source: 'injected-script',
        type: type,
        data: data,
        timestamp: Date.now()
      }, '*');
    }

    getStats() {
      return {
        webgl: this.webglStats,
        canvas: this.canvasStats,
        performance: {
          frameStats: {
            averageFrameTime: this.getAverageFrameTime(),
            fps: this.calculateFPS()
          }
        },
        network: this.networkStats
      };
    }
  }

  window.deepMonitor = new DeepPageMonitor();

  window.addEventListener('message', (event) => {
    if (event.data.target === 'injected-script') {
      switch (event.data.action) {
        case 'get-stats':
          window.deepMonitor.sendToContentScript('stats-response', window.deepMonitor.getStats());
          break;
        case 'collect-metrics':
          window.deepMonitor.collectPerformanceMetrics();
          break;
      }
    }
  });

})();