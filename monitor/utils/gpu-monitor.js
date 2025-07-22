class GPUMonitor {
  constructor() {
    this.isMonitoring = false;
    this.webglContexts = new Map();
    this.canvasContexts = new Map();
    this.gpuStats = {
      webgl: {
        contexts: 0,
        drawCalls: 0,
        bufferOperations: 0,
        textureOperations: 0,
        shaderOperations: 0,
        memoryUsage: 0
      },
      canvas2d: {
        contexts: 0,
        drawOperations: 0,
        imageOperations: 0,
        textOperations: 0,
        pixelOperations: 0
      },
      performance: {
        frameRate: 0,
        frameTime: 0,
        gpuMemory: 0,
        gpuUtilization: 0
      }
    };
    this.performanceHistory = [];
    this.maxHistoryLength = 100;
    this.monitoringInterval = null;
    this.frameMonitoringId = null;
    this.lastFrameTime = 0;
    this.frameCount = 0;
    this.callbacks = new Set();
  }

  startMonitoring() {
    if (this.isMonitoring) return;
    
    this.isMonitoring = true;
    this.resetStats();
    this.setupWebGLMonitoring();
    this.setupCanvas2DMonitoring();
    this.setupPerformanceMonitoring();
    this.startFrameMonitoring();
    this.startPeriodicReporting();
    
    return true;
  }

  stopMonitoring() {
    if (!this.isMonitoring) return;
    
    this.isMonitoring = false;
    this.stopFrameMonitoring();
    this.stopPeriodicReporting();
    this.cleanup();
    
    return true;
  }

  setupWebGLMonitoring() {
    if (typeof window === 'undefined') return;

    const originalGetContext = HTMLCanvasElement.prototype.getContext;
    const self = this;

    HTMLCanvasElement.prototype.getContext = function(contextType, ...args) {
      const context = originalGetContext.apply(this, arguments);
      
      if (context && (contextType === 'webgl' || contextType === 'experimental-webgl' || contextType === 'webgl2')) {
        self.registerWebGLContext(this, context, contextType);
      }
      
      return context;
    };
  }

  registerWebGLContext(canvas, context, type) {
    const contextId = this.generateContextId();
    
    const contextInfo = {
      id: contextId,
      canvas: canvas,
      context: context,
      type: type,
      createdAt: Date.now(),
      stats: {
        drawCalls: 0,
        bufferOperations: 0,
        textureOperations: 0,
        shaderOperations: 0
      }
    };

    this.webglContexts.set(contextId, contextInfo);
    this.gpuStats.webgl.contexts++;
    
    this.instrumentWebGLContext(contextInfo);
    this.analyzeWebGLCapabilities(contextInfo);
    
    this.notifyCallbacks('webgl-context-created', contextInfo);
  }

  instrumentWebGLContext(contextInfo) {
    const context = contextInfo.context;
    const stats = contextInfo.stats;

    const drawMethods = ['drawArrays', 'drawElements', 'drawArraysInstanced', 'drawElementsInstanced'];
    drawMethods.forEach(method => {
      if (context[method]) {
        const original = context[method];
        context[method] = function(...args) {
          stats.drawCalls++;
          this.gpuStats.webgl.drawCalls++;
          return original.apply(this, args);
        }.bind(this);
      }
    });

    const bufferMethods = ['bufferData', 'bufferSubData', 'createBuffer', 'deleteBuffer'];
    bufferMethods.forEach(method => {
      if (context[method]) {
        const original = context[method];
        context[method] = function(...args) {
          stats.bufferOperations++;
          this.gpuStats.webgl.bufferOperations++;
          if (method === 'bufferData' && args[1]) {
            this.gpuStats.webgl.memoryUsage += this.estimateBufferSize(args[1]);
          }
          return original.apply(this, args);
        }.bind(this);
      }
    });

    const textureMethods = ['texImage2D', 'texSubImage2D', 'createTexture', 'deleteTexture'];
    textureMethods.forEach(method => {
      if (context[method]) {
        const original = context[method];
        context[method] = function(...args) {
          stats.textureOperations++;
          this.gpuStats.webgl.textureOperations++;
          if (method === 'texImage2D') {
            this.gpuStats.webgl.memoryUsage += this.estimateTextureSize(args);
          }
          return original.apply(this, args);
        }.bind(this);
      }
    });

    const shaderMethods = ['createShader', 'compileShader', 'createProgram', 'linkProgram'];
    shaderMethods.forEach(method => {
      if (context[method]) {
        const original = context[method];
        context[method] = function(...args) {
          stats.shaderOperations++;
          this.gpuStats.webgl.shaderOperations++;
          return original.apply(this, args);
        }.bind(this);
      }
    });
  }

  analyzeWebGLCapabilities(contextInfo) {
    const context = contextInfo.context;
    
    try {
      contextInfo.capabilities = {
        vendor: context.getParameter(context.VENDOR),
        renderer: context.getParameter(context.RENDERER),
        version: context.getParameter(context.VERSION),
        shadingLanguageVersion: context.getParameter(context.SHADING_LANGUAGE_VERSION),
        maxTextureSize: context.getParameter(context.MAX_TEXTURE_SIZE),
        maxRenderbufferSize: context.getParameter(context.MAX_RENDERBUFFER_SIZE),
        maxViewportDims: context.getParameter(context.MAX_VIEWPORT_DIMS),
        maxVertexAttribs: context.getParameter(context.MAX_VERTEX_ATTRIBS),
        maxVertexUniformVectors: context.getParameter(context.MAX_VERTEX_UNIFORM_VECTORS),
        maxFragmentUniformVectors: context.getParameter(context.MAX_FRAGMENT_UNIFORM_VECTORS),
        maxVaryingVectors: context.getParameter(context.MAX_VARYING_VECTORS),
        aliasedLineWidthRange: context.getParameter(context.ALIASED_LINE_WIDTH_RANGE),
        aliasedPointSizeRange: context.getParameter(context.ALIASED_POINT_SIZE_RANGE)
      };

      if (contextInfo.type === 'webgl2') {
        contextInfo.capabilities.maxDrawBuffers = context.getParameter(context.MAX_DRAW_BUFFERS);
        contextInfo.capabilities.maxColorAttachments = context.getParameter(context.MAX_COLOR_ATTACHMENTS);
        contextInfo.capabilities.maxSamples = context.getParameter(context.MAX_SAMPLES);
      }

      const debugInfo = context.getExtension('WEBGL_debug_renderer_info');
      if (debugInfo) {
        contextInfo.capabilities.unmaskedVendor = context.getParameter(debugInfo.UNMASKED_VENDOR_WEBGL);
        contextInfo.capabilities.unmaskedRenderer = context.getParameter(debugInfo.UNMASKED_RENDERER_WEBGL);
      }

    } catch (error) {
      contextInfo.capabilities = { error: error.message };
    }
  }

  setupCanvas2DMonitoring() {
    if (typeof window === 'undefined') return;

    const originalGetContext = HTMLCanvasElement.prototype.getContext;
    const self = this;

    HTMLCanvasElement.prototype.getContext = function(contextType, ...args) {
      const context = originalGetContext.apply(this, arguments);
      
      if (context && contextType === '2d') {
        self.registerCanvas2DContext(this, context);
      }
      
      return context;
    };
  }

  registerCanvas2DContext(canvas, context) {
    const contextId = this.generateContextId();
    
    const contextInfo = {
      id: contextId,
      canvas: canvas,
      context: context,
      type: '2d',
      createdAt: Date.now(),
      stats: {
        drawOperations: 0,
        imageOperations: 0,
        textOperations: 0,
        pixelOperations: 0
      }
    };

    this.canvasContexts.set(contextId, contextInfo);
    this.gpuStats.canvas2d.contexts++;
    
    this.instrumentCanvas2DContext(contextInfo);
    
    this.notifyCallbacks('canvas-2d-context-created', contextInfo);
  }

  instrumentCanvas2DContext(contextInfo) {
    const context = contextInfo.context;
    const stats = contextInfo.stats;

    const drawMethods = ['fillRect', 'strokeRect', 'clearRect', 'fill', 'stroke', 'clip'];
    drawMethods.forEach(method => {
      if (context[method]) {
        const original = context[method];
        context[method] = function(...args) {
          stats.drawOperations++;
          this.gpuStats.canvas2d.drawOperations++;
          return original.apply(this, args);
        }.bind(this);
      }
    });

    const imageMethods = ['drawImage', 'createPattern'];
    imageMethods.forEach(method => {
      if (context[method]) {
        const original = context[method];
        context[method] = function(...args) {
          stats.imageOperations++;
          this.gpuStats.canvas2d.imageOperations++;
          return original.apply(this, args);
        }.bind(this);
      }
    });

    const textMethods = ['fillText', 'strokeText', 'measureText'];
    textMethods.forEach(method => {
      if (context[method]) {
        const original = context[method];
        context[method] = function(...args) {
          stats.textOperations++;
          this.gpuStats.canvas2d.textOperations++;
          return original.apply(this, args);
        }.bind(this);
      }
    });

    const pixelMethods = ['getImageData', 'putImageData', 'createImageData'];
    pixelMethods.forEach(method => {
      if (context[method]) {
        const original = context[method];
        context[method] = function(...args) {
          stats.pixelOperations++;
          this.gpuStats.canvas2d.pixelOperations++;
          return original.apply(this, args);
        }.bind(this);
      }
    });
  }

  setupPerformanceMonitoring() {
    if (typeof performance !== 'undefined' && performance.memory) {
      this.monitorMemoryUsage();
    }
    
    this.monitorGPUPerformance();
  }

  monitorMemoryUsage() {
    const updateMemoryStats = () => {
      if (performance.memory) {
        this.gpuStats.performance.gpuMemory = performance.memory.usedJSHeapSize;
      }
    };

    setInterval(updateMemoryStats, 1000);
  }

  monitorGPUPerformance() {
    let lastTime = performance.now();
    let frameCount = 0;
    
    const measurePerformance = (currentTime) => {
      frameCount++;
      const deltaTime = currentTime - lastTime;
      
      if (deltaTime >= 1000) {
        this.gpuStats.performance.frameRate = Math.round((frameCount * 1000) / deltaTime);
        this.gpuStats.performance.frameTime = deltaTime / frameCount;
        
        this.addPerformanceHistory({
          timestamp: currentTime,
          frameRate: this.gpuStats.performance.frameRate,
          frameTime: this.gpuStats.performance.frameTime,
          gpuMemory: this.gpuStats.performance.gpuMemory
        });
        
        frameCount = 0;
        lastTime = currentTime;
      }
      
      if (this.isMonitoring) {
        this.frameMonitoringId = requestAnimationFrame(measurePerformance);
      }
    };
    
    if (typeof requestAnimationFrame !== 'undefined') {
      this.frameMonitoringId = requestAnimationFrame(measurePerformance);
    }
  }

  startFrameMonitoring() {
    if (typeof requestAnimationFrame === 'undefined') return;
    
    let lastFrameTime = performance.now();
    
    const frameCallback = (currentTime) => {
      this.frameCount++;
      const frameDuration = currentTime - lastFrameTime;
      
      this.gpuStats.performance.frameTime = frameDuration;
      this.gpuStats.performance.frameRate = 1000 / frameDuration;
      
      lastFrameTime = currentTime;
      
      if (this.isMonitoring) {
        this.frameMonitoringId = requestAnimationFrame(frameCallback);
      }
    };
    
    this.frameMonitoringId = requestAnimationFrame(frameCallback);
  }

  stopFrameMonitoring() {
    if (this.frameMonitoringId) {
      cancelAnimationFrame(this.frameMonitoringId);
      this.frameMonitoringId = null;
    }
  }

  startPeriodicReporting() {
    this.monitoringInterval = setInterval(() => {
      this.updateStats();
      this.notifyCallbacks('stats-update', this.getStats());
    }, 1000);
  }

  stopPeriodicReporting() {
    if (this.monitoringInterval) {
      clearInterval(this.monitoringInterval);
      this.monitoringInterval = null;
    }
  }

  updateStats() {
    this.gpuStats.performance.gpuUtilization = this.calculateGPUUtilization();
    
    if (typeof performance !== 'undefined' && performance.memory) {
      this.gpuStats.performance.gpuMemory = performance.memory.usedJSHeapSize;
    }
  }

  calculateGPUUtilization() {
    const totalOperations = this.gpuStats.webgl.drawCalls + 
                           this.gpuStats.canvas2d.drawOperations + 
                           this.gpuStats.canvas2d.imageOperations;
    
    const timeWindow = 1000;
    const maxOperationsPerSecond = 1000;
    
    return Math.min(100, (totalOperations / maxOperationsPerSecond) * 100);
  }

  addPerformanceHistory(data) {
    this.performanceHistory.push(data);
    
    if (this.performanceHistory.length > this.maxHistoryLength) {
      this.performanceHistory.shift();
    }
  }

  estimateBufferSize(data) {
    if (data instanceof ArrayBuffer) {
      return data.byteLength;
    } else if (data && data.byteLength) {
      return data.byteLength;
    } else if (typeof data === 'number') {
      return data;
    }
    return 0;
  }

  estimateTextureSize(args) {
    if (args.length >= 9) {
      const width = args[3];
      const height = args[4];
      const format = args[6];
      const type = args[7];
      
      let bytesPerPixel = 4;
      
      if (format === WebGLRenderingContext.RGB) {
        bytesPerPixel = 3;
      } else if (format === WebGLRenderingContext.LUMINANCE) {
        bytesPerPixel = 1;
      } else if (format === WebGLRenderingContext.LUMINANCE_ALPHA) {
        bytesPerPixel = 2;
      }
      
      if (type === WebGLRenderingContext.UNSIGNED_SHORT) {
        bytesPerPixel *= 2;
      } else if (type === WebGLRenderingContext.FLOAT) {
        bytesPerPixel *= 4;
      }
      
      return width * height * bytesPerPixel;
    }
    return 0;
  }

  generateContextId() {
    return `ctx_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`;
  }

  getStats() {
    return {
      ...this.gpuStats,
      contextCounts: {
        webgl: this.webglContexts.size,
        canvas2d: this.canvasContexts.size
      },
      performanceHistory: this.performanceHistory.slice(-10),
      isMonitoring: this.isMonitoring,
      timestamp: Date.now()
    };
  }

  getDetailedStats() {
    return {
      ...this.getStats(),
      webglContexts: Array.from(this.webglContexts.values()).map(ctx => ({
        id: ctx.id,
        type: ctx.type,
        createdAt: ctx.createdAt,
        stats: ctx.stats,
        capabilities: ctx.capabilities,
        canvasSize: {
          width: ctx.canvas.width,
          height: ctx.canvas.height
        }
      })),
      canvas2dContexts: Array.from(this.canvasContexts.values()).map(ctx => ({
        id: ctx.id,
        type: ctx.type,
        createdAt: ctx.createdAt,
        stats: ctx.stats,
        canvasSize: {
          width: ctx.canvas.width,
          height: ctx.canvas.height
        }
      })),
      performanceHistory: this.performanceHistory
    };
  }

  resetStats() {
    this.gpuStats = {
      webgl: {
        contexts: 0,
        drawCalls: 0,
        bufferOperations: 0,
        textureOperations: 0,
        shaderOperations: 0,
        memoryUsage: 0
      },
      canvas2d: {
        contexts: 0,
        drawOperations: 0,
        imageOperations: 0,
        textOperations: 0,
        pixelOperations: 0
      },
      performance: {
        frameRate: 0,
        frameTime: 0,
        gpuMemory: 0,
        gpuUtilization: 0
      }
    };
    this.performanceHistory = [];
    this.frameCount = 0;
  }

  addCallback(callback) {
    this.callbacks.add(callback);
  }

  removeCallback(callback) {
    this.callbacks.delete(callback);
  }

  notifyCallbacks(event, data) {
    for (const callback of this.callbacks) {
      try {
        callback(event, data);
      } catch (error) {
        console.error('GPU Monitor callback error:', error);
      }
    }
  }

  cleanup() {
    this.webglContexts.clear();
    this.canvasContexts.clear();
    this.callbacks.clear();
    this.performanceHistory = [];
  }

  getWebGLContexts() {
    return Array.from(this.webglContexts.values());
  }

  getCanvas2DContexts() {
    return Array.from(this.canvasContexts.values());
  }

  getPerformanceHistory() {
    return this.performanceHistory;
  }

  isGPUAccelerated() {
    return this.webglContexts.size > 0 || this.hasHardwareAcceleration();
  }

  hasHardwareAcceleration() {
    if (typeof window === 'undefined') return false;
    
    try {
      const canvas = document.createElement('canvas');
      const gl = canvas.getContext('webgl') || canvas.getContext('experimental-webgl');
      
      if (gl) {
        const debugInfo = gl.getExtension('WEBGL_debug_renderer_info');
        if (debugInfo) {
          const renderer = gl.getParameter(debugInfo.UNMASKED_RENDERER_WEBGL);
          return !renderer.includes('Software') && !renderer.includes('SwiftShader');
        }
      }
      
      return false;
    } catch (error) {
      return false;
    }
  }
}

if (typeof module !== 'undefined' && module.exports) {
  module.exports = GPUMonitor;
} else if (typeof window !== 'undefined') {
  window.GPUMonitor = GPUMonitor;
}