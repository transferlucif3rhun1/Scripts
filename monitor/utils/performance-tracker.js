class PerformanceTracker {
  constructor() {
    this.isTracking = false;
    this.metrics = new Map();
    this.observations = [];
    this.baselines = new Map();
    this.comparisons = [];
    this.observers = [];
    this.intervals = [];
    this.startTime = null;
    this.trackingOptions = {
      navigation: true,
      resources: true,
      paint: true,
      layout: true,
      memory: true,
      network: true,
      userTiming: true,
      longTasks: true
    };
    this.thresholds = {
      fcp: 1800,
      lcp: 2500,
      fid: 100,
      cls: 0.1,
      ttfb: 600,
      domContentLoaded: 2000,
      loadComplete: 3000
    };
  }

  startTracking(options = {}) {
    if (this.isTracking) {
      throw new Error('Performance tracking already in progress');
    }

    this.isTracking = true;
    this.startTime = performance.now();
    this.trackingOptions = { ...this.trackingOptions, ...options };
    this.resetMetrics();

    this.setupPerformanceObservers();
    this.startContinuousMonitoring();
    this.captureInitialMetrics();

    return {
      success: true,
      startTime: this.startTime,
      trackingId: this.generateTrackingId()
    };
  }

  stopTracking() {
    if (!this.isTracking) {
      return { success: false, error: 'No tracking in progress' };
    }

    this.isTracking = false;
    this.cleanup();

    const finalMetrics = this.getFinalMetrics();
    const duration = performance.now() - this.startTime;

    return {
      success: true,
      duration: duration,
      metrics: finalMetrics,
      observations: this.observations.length,
      comparisons: this.comparisons.length
    };
  }

  captureBaseline(name = 'default') {
    const baseline = this.captureCurrentState();
    this.baselines.set(name, baseline);
    
    return {
      success: true,
      baseline: baseline,
      timestamp: Date.now()
    };
  }

  compareWithBaseline(name = 'default') {
    const baseline = this.baselines.get(name);
    if (!baseline) {
      throw new Error(`Baseline '${name}' not found`);
    }

    const current = this.captureCurrentState();
    const comparison = this.generateComparison(baseline, current);
    this.comparisons.push(comparison);

    return comparison;
  }

  captureCurrentState() {
    return {
      timestamp: Date.now(),
      navigation: this.getNavigationMetrics(),
      paint: this.getPaintMetrics(),
      layout: this.getLayoutMetrics(),
      memory: this.getMemoryMetrics(),
      network: this.getNetworkMetrics(),
      resources: this.getResourceMetrics(),
      userTiming: this.getUserTimingMetrics(),
      vitals: this.getWebVitals(),
      custom: this.getCustomMetrics()
    };
  }

  getNavigationMetrics() {
    if (!performance.getEntriesByType) return null;

    const navigation = performance.getEntriesByType('navigation')[0];
    if (!navigation) return null;

    return {
      navigationStart: navigation.startTime,
      unloadEventStart: navigation.unloadEventStart,
      unloadEventEnd: navigation.unloadEventEnd,
      redirectStart: navigation.redirectStart,
      redirectEnd: navigation.redirectEnd,
      fetchStart: navigation.fetchStart,
      domainLookupStart: navigation.domainLookupStart,
      domainLookupEnd: navigation.domainLookupEnd,
      connectStart: navigation.connectStart,
      connectEnd: navigation.connectEnd,
      secureConnectionStart: navigation.secureConnectionStart,
      requestStart: navigation.requestStart,
      responseStart: navigation.responseStart,
      responseEnd: navigation.responseEnd,
      domInteractive: navigation.domInteractive,
      domContentLoadedEventStart: navigation.domContentLoadedEventStart,
      domContentLoadedEventEnd: navigation.domContentLoadedEventEnd,
      domComplete: navigation.domComplete,
      loadEventStart: navigation.loadEventStart,
      loadEventEnd: navigation.loadEventEnd,
      type: navigation.type,
      redirectCount: navigation.redirectCount,
      
      calculated: {
        ttfb: navigation.responseStart - navigation.requestStart,
        domProcessing: navigation.domComplete - navigation.domInteractive,
        pageLoad: navigation.loadEventEnd - navigation.startTime,
        dns: navigation.domainLookupEnd - navigation.domainLookupStart,
        tcp: navigation.connectEnd - navigation.connectStart,
        request: navigation.responseEnd - navigation.requestStart,
        response: navigation.responseEnd - navigation.responseStart
      }
    };
  }

  getPaintMetrics() {
    if (!performance.getEntriesByType) return null;

    const paintEntries = performance.getEntriesByType('paint');
    const metrics = {};

    paintEntries.forEach(entry => {
      metrics[entry.name] = {
        startTime: entry.startTime,
        duration: entry.duration
      };
    });

    const fcpEntry = paintEntries.find(entry => entry.name === 'first-contentful-paint');
    const fpEntry = paintEntries.find(entry => entry.name === 'first-paint');

    return {
      firstPaint: fpEntry ? fpEntry.startTime : null,
      firstContentfulPaint: fcpEntry ? fcpEntry.startTime : null,
      ...metrics
    };
  }

  getLayoutMetrics() {
    if (!performance.getEntriesByType) return null;

    const layoutShifts = performance.getEntriesByType('layout-shift');
    let cumulativeLayoutShift = 0;
    const shifts = [];

    layoutShifts.forEach(entry => {
      if (!entry.hadRecentInput) {
        cumulativeLayoutShift += entry.value;
        shifts.push({
          value: entry.value,
          startTime: entry.startTime,
          duration: entry.duration,
          sources: entry.sources ? Array.from(entry.sources).map(source => ({
            node: source.node?.tagName,
            currentRect: source.currentRect,
            previousRect: source.previousRect
          })) : []
        });
      }
    });

    return {
      cumulativeLayoutShift: cumulativeLayoutShift,
      shifts: shifts,
      shiftCount: shifts.length
    };
  }

  getMemoryMetrics() {
    if (!performance.memory) return null;

    return {
      usedJSHeapSize: performance.memory.usedJSHeapSize,
      totalJSHeapSize: performance.memory.totalJSHeapSize,
      jsHeapSizeLimit: performance.memory.jsHeapSizeLimit,
      calculated: {
        heapUsagePercentage: (performance.memory.usedJSHeapSize / performance.memory.jsHeapSizeLimit) * 100,
        availableHeap: performance.memory.jsHeapSizeLimit - performance.memory.usedJSHeapSize
      }
    };
  }

  getNetworkMetrics() {
    if (!navigator.connection) return null;

    return {
      effectiveType: navigator.connection.effectiveType,
      downlink: navigator.connection.downlink,
      rtt: navigator.connection.rtt,
      saveData: navigator.connection.saveData,
      type: navigator.connection.type
    };
  }

  getResourceMetrics() {
    if (!performance.getEntriesByType) return null;

    const resources = performance.getEntriesByType('resource');
    const summary = {
      totalResources: resources.length,
      totalTransferSize: 0,
      totalEncodedSize: 0,
      totalDecodedSize: 0,
      byType: {},
      byDomain: {},
      slowest: [],
      largest: []
    };

    const typeStats = {};
    const domainStats = {};

    resources.forEach(resource => {
      const type = resource.initiatorType || 'other';
      const domain = this.extractDomain(resource.name);

      summary.totalTransferSize += resource.transferSize || 0;
      summary.totalEncodedSize += resource.encodedBodySize || 0;
      summary.totalDecodedSize += resource.decodedBodySize || 0;

      if (!typeStats[type]) {
        typeStats[type] = { count: 0, size: 0, duration: 0 };
      }
      typeStats[type].count++;
      typeStats[type].size += resource.transferSize || 0;
      typeStats[type].duration += resource.duration || 0;

      if (!domainStats[domain]) {
        domainStats[domain] = { count: 0, size: 0 };
      }
      domainStats[domain].count++;
      domainStats[domain].size += resource.transferSize || 0;
    });

    summary.byType = typeStats;
    summary.byDomain = domainStats;

    summary.slowest = resources
      .sort((a, b) => (b.duration || 0) - (a.duration || 0))
      .slice(0, 10)
      .map(resource => ({
        name: resource.name,
        duration: resource.duration,
        type: resource.initiatorType
      }));

    summary.largest = resources
      .sort((a, b) => (b.transferSize || 0) - (a.transferSize || 0))
      .slice(0, 10)
      .map(resource => ({
        name: resource.name,
        size: resource.transferSize,
        type: resource.initiatorType
      }));

    return summary;
  }

  getUserTimingMetrics() {
    if (!performance.getEntriesByType) return null;

    const measures = performance.getEntriesByType('measure');
    const marks = performance.getEntriesByType('mark');

    return {
      marks: marks.map(mark => ({
        name: mark.name,
        startTime: mark.startTime,
        duration: mark.duration
      })),
      measures: measures.map(measure => ({
        name: measure.name,
        startTime: measure.startTime,
        duration: measure.duration
      }))
    };
  }

  getWebVitals() {
    const vitals = {};

    if (performance.getEntriesByType) {
      const navigation = performance.getEntriesByType('navigation')[0];
      const paint = performance.getEntriesByType('paint');
      const layoutShifts = performance.getEntriesByType('layout-shift');
      const longTasks = performance.getEntriesByType('longtask');

      if (navigation) {
        vitals.ttfb = navigation.responseStart - navigation.requestStart;
      }

      const fcpEntry = paint.find(entry => entry.name === 'first-contentful-paint');
      if (fcpEntry) {
        vitals.fcp = fcpEntry.startTime;
      }

      const lcpEntries = performance.getEntriesByType('largest-contentful-paint');
      if (lcpEntries.length > 0) {
        vitals.lcp = lcpEntries[lcpEntries.length - 1].startTime;
      }

      const fidEntries = performance.getEntriesByType('first-input');
      if (fidEntries.length > 0) {
        vitals.fid = fidEntries[0].processingStart - fidEntries[0].startTime;
      }

      let cls = 0;
      layoutShifts.forEach(entry => {
        if (!entry.hadRecentInput) {
          cls += entry.value;
        }
      });
      vitals.cls = cls;

      vitals.tbt = this.calculateTotalBlockingTime(longTasks);
    }

    vitals.scores = this.calculateVitalScores(vitals);
    return vitals;
  }

  calculateTotalBlockingTime(longTasks) {
    return longTasks.reduce((total, task) => {
      const blockingTime = Math.max(0, task.duration - 50);
      return total + blockingTime;
    }, 0);
  }

  calculateVitalScores(vitals) {
    const scores = {};

    if (vitals.fcp !== undefined) {
      scores.fcp = vitals.fcp <= 1800 ? 'good' : vitals.fcp <= 3000 ? 'needs-improvement' : 'poor';
    }

    if (vitals.lcp !== undefined) {
      scores.lcp = vitals.lcp <= 2500 ? 'good' : vitals.lcp <= 4000 ? 'needs-improvement' : 'poor';
    }

    if (vitals.fid !== undefined) {
      scores.fid = vitals.fid <= 100 ? 'good' : vitals.fid <= 300 ? 'needs-improvement' : 'poor';
    }

    if (vitals.cls !== undefined) {
      scores.cls = vitals.cls <= 0.1 ? 'good' : vitals.cls <= 0.25 ? 'needs-improvement' : 'poor';
    }

    if (vitals.ttfb !== undefined) {
      scores.ttfb = vitals.ttfb <= 600 ? 'good' : vitals.ttfb <= 1500 ? 'needs-improvement' : 'poor';
    }

    return scores;
  }

  getCustomMetrics() {
    const custom = {};

    if (window.performance && window.performance.mark) {
      try {
        const customMarks = performance.getEntriesByName('custom-start');
        if (customMarks.length > 0) {
          custom.customTiming = performance.now() - customMarks[0].startTime;
        }
      } catch (e) {}
    }

    custom.domElementCount = document.querySelectorAll('*').length;
    custom.scriptCount = document.querySelectorAll('script').length;
    custom.styleSheetCount = document.querySelectorAll('link[rel="stylesheet"], style').length;
    custom.imageCount = document.querySelectorAll('img').length;
    custom.iframeCount = document.querySelectorAll('iframe').length;

    custom.viewportSize = {
      width: window.innerWidth,
      height: window.innerHeight
    };

    custom.scrollPosition = {
      x: window.scrollX,
      y: window.scrollY
    };

    return custom;
  }

  setupPerformanceObservers() {
    if (!window.PerformanceObserver) return;

    try {
      if (this.trackingOptions.paint) {
        const paintObserver = new PerformanceObserver((list) => {
          this.handlePerformanceEntries(list.getEntries(), 'paint');
        });
        paintObserver.observe({ entryTypes: ['paint'] });
        this.observers.push(paintObserver);
      }

      if (this.trackingOptions.layout) {
        const layoutObserver = new PerformanceObserver((list) => {
          this.handlePerformanceEntries(list.getEntries(), 'layout-shift');
        });
        layoutObserver.observe({ entryTypes: ['layout-shift'] });
        this.observers.push(layoutObserver);
      }

      if (this.trackingOptions.longTasks) {
        const longTaskObserver = new PerformanceObserver((list) => {
          this.handlePerformanceEntries(list.getEntries(), 'longtask');
        });
        longTaskObserver.observe({ entryTypes: ['longtask'] });
        this.observers.push(longTaskObserver);
      }

      if (this.trackingOptions.resources) {
        const resourceObserver = new PerformanceObserver((list) => {
          this.handlePerformanceEntries(list.getEntries(), 'resource');
        });
        resourceObserver.observe({ entryTypes: ['resource'] });
        this.observers.push(resourceObserver);
      }

      if (this.trackingOptions.navigation) {
        const navigationObserver = new PerformanceObserver((list) => {
          this.handlePerformanceEntries(list.getEntries(), 'navigation');
        });
        navigationObserver.observe({ entryTypes: ['navigation'] });
        this.observers.push(navigationObserver);
      }

      const userTimingObserver = new PerformanceObserver((list) => {
        this.handlePerformanceEntries(list.getEntries(), 'user-timing');
      });
      userTimingObserver.observe({ entryTypes: ['mark', 'measure'] });
      this.observers.push(userTimingObserver);

    } catch (error) {
      console.warn('Failed to setup performance observers:', error);
    }
  }

  handlePerformanceEntries(entries, type) {
    entries.forEach(entry => {
      const observation = {
        type: type,
        entry: this.serializePerformanceEntry(entry),
        timestamp: Date.now(),
        sessionTime: performance.now() - this.startTime
      };

      this.observations.push(observation);
      this.processObservation(observation);
    });

    if (this.observations.length > 1000) {
      this.observations = this.observations.slice(-500);
    }
  }

  serializePerformanceEntry(entry) {
    const serialized = {
      name: entry.name,
      entryType: entry.entryType,
      startTime: entry.startTime,
      duration: entry.duration
    };

    if (entry.entryType === 'resource') {
      serialized.transferSize = entry.transferSize;
      serialized.encodedBodySize = entry.encodedBodySize;
      serialized.decodedBodySize = entry.decodedBodySize;
      serialized.initiatorType = entry.initiatorType;
    }

    if (entry.entryType === 'layout-shift') {
      serialized.value = entry.value;
      serialized.hadRecentInput = entry.hadRecentInput;
    }

    if (entry.entryType === 'largest-contentful-paint') {
      serialized.size = entry.size;
      serialized.element = entry.element?.tagName;
    }

    if (entry.entryType === 'first-input') {
      serialized.processingStart = entry.processingStart;
      serialized.processingEnd = entry.processingEnd;
    }

    return serialized;
  }

  processObservation(observation) {
    if (observation.type === 'layout-shift' && observation.entry.value > 0.1) {
      this.flagPerformanceIssue('large-layout-shift', observation);
    }

    if (observation.type === 'longtask' && observation.entry.duration > 200) {
      this.flagPerformanceIssue('long-task', observation);
    }

    if (observation.type === 'resource' && observation.entry.duration > 3000) {
      this.flagPerformanceIssue('slow-resource', observation);
    }
  }

  flagPerformanceIssue(issueType, observation) {
    if (!this.metrics.has('issues')) {
      this.metrics.set('issues', []);
    }

    this.metrics.get('issues').push({
      type: issueType,
      observation: observation,
      timestamp: Date.now(),
      severity: this.calculateIssueSeverity(issueType, observation)
    });
  }

  calculateIssueSeverity(issueType, observation) {
    const severityMap = {
      'large-layout-shift': observation.entry.value > 0.25 ? 'high' : 'medium',
      'long-task': observation.entry.duration > 500 ? 'high' : 'medium',
      'slow-resource': observation.entry.duration > 5000 ? 'high' : 'medium'
    };

    return severityMap[issueType] || 'low';
  }

  startContinuousMonitoring() {
    const memoryInterval = setInterval(() => {
      if (!this.isTracking) return;
      this.recordMemorySnapshot();
    }, 5000);

    const networkInterval = setInterval(() => {
      if (!this.isTracking) return;
      this.recordNetworkSnapshot();
    }, 10000);

    this.intervals.push(memoryInterval, networkInterval);
  }

  recordMemorySnapshot() {
    const memory = this.getMemoryMetrics();
    if (memory) {
      if (!this.metrics.has('memorySnapshots')) {
        this.metrics.set('memorySnapshots', []);
      }
      
      this.metrics.get('memorySnapshots').push({
        timestamp: Date.now(),
        sessionTime: performance.now() - this.startTime,
        ...memory
      });

      const snapshots = this.metrics.get('memorySnapshots');
      if (snapshots.length > 50) {
        snapshots.splice(0, snapshots.length - 25);
      }
    }
  }

  recordNetworkSnapshot() {
    const network = this.getNetworkMetrics();
    if (network) {
      if (!this.metrics.has('networkSnapshots')) {
        this.metrics.set('networkSnapshots', []);
      }
      
      this.metrics.get('networkSnapshots').push({
        timestamp: Date.now(),
        sessionTime: performance.now() - this.startTime,
        ...network
      });

      const snapshots = this.metrics.get('networkSnapshots');
      if (snapshots.length > 20) {
        snapshots.splice(0, snapshots.length - 10);
      }
    }
  }

  generateComparison(baseline, current) {
    const comparison = {
      id: this.generateComparisonId(),
      timestamp: Date.now(),
      baseline: baseline,
      current: current,
      differences: {},
      improvements: {},
      regressions: {},
      summary: {}
    };

    comparison.differences.navigation = this.compareNavigationMetrics(
      baseline.navigation, 
      current.navigation
    );

    comparison.differences.paint = this.comparePaintMetrics(
      baseline.paint, 
      current.paint
    );

    comparison.differences.memory = this.compareMemoryMetrics(
      baseline.memory, 
      current.memory
    );

    comparison.differences.vitals = this.compareWebVitals(
      baseline.vitals, 
      current.vitals
    );

    comparison.differences.resources = this.compareResourceMetrics(
      baseline.resources, 
      current.resources
    );

    comparison.summary = this.generateComparisonSummary(comparison.differences);

    return comparison;
  }

  compareNavigationMetrics(baseline, current) {
    if (!baseline || !current) return null;

    const differences = {};
    const keys = ['ttfb', 'domProcessing', 'pageLoad', 'dns', 'tcp', 'request', 'response'];

    keys.forEach(key => {
      if (baseline.calculated[key] !== undefined && current.calculated[key] !== undefined) {
        const diff = current.calculated[key] - baseline.calculated[key];
        const percentChange = (diff / baseline.calculated[key]) * 100;
        
        differences[key] = {
          baseline: baseline.calculated[key],
          current: current.calculated[key],
          difference: diff,
          percentChange: percentChange,
          improved: diff < 0
        };
      }
    });

    return differences;
  }

  comparePaintMetrics(baseline, current) {
    if (!baseline || !current) return null;

    const differences = {};
    const keys = ['firstPaint', 'firstContentfulPaint'];

    keys.forEach(key => {
      if (baseline[key] !== undefined && current[key] !== undefined) {
        const diff = current[key] - baseline[key];
        const percentChange = (diff / baseline[key]) * 100;
        
        differences[key] = {
          baseline: baseline[key],
          current: current[key],
          difference: diff,
          percentChange: percentChange,
          improved: diff < 0
        };
      }
    });

    return differences;
  }

  compareMemoryMetrics(baseline, current) {
    if (!baseline || !current) return null;

    const differences = {};
    const keys = ['usedJSHeapSize', 'totalJSHeapSize'];

    keys.forEach(key => {
      if (baseline[key] !== undefined && current[key] !== undefined) {
        const diff = current[key] - baseline[key];
        const percentChange = (diff / baseline[key]) * 100;
        
        differences[key] = {
          baseline: baseline[key],
          current: current[key],
          difference: diff,
          percentChange: percentChange,
          improved: diff < 0
        };
      }
    });

    return differences;
  }

  compareWebVitals(baseline, current) {
    if (!baseline || !current) return null;

    const differences = {};
    const keys = ['fcp', 'lcp', 'fid', 'cls', 'ttfb'];

    keys.forEach(key => {
      if (baseline[key] !== undefined && current[key] !== undefined) {
        const diff = current[key] - baseline[key];
        const percentChange = baseline[key] > 0 ? (diff / baseline[key]) * 100 : 0;
        
        differences[key] = {
          baseline: baseline[key],
          current: current[key],
          difference: diff,
          percentChange: percentChange,
          improved: diff < 0,
          baselineScore: baseline.scores?.[key],
          currentScore: current.scores?.[key],
          scoreImproved: this.isScoreImproved(baseline.scores?.[key], current.scores?.[key])
        };
      }
    });

    return differences;
  }

  compareResourceMetrics(baseline, current) {
    if (!baseline || !current) return null;

    return {
      totalResources: {
        baseline: baseline.totalResources,
        current: current.totalResources,
        difference: current.totalResources - baseline.totalResources,
        improved: current.totalResources < baseline.totalResources
      },
      totalTransferSize: {
        baseline: baseline.totalTransferSize,
        current: current.totalTransferSize,
        difference: current.totalTransferSize - baseline.totalTransferSize,
        percentChange: baseline.totalTransferSize > 0 ? 
          ((current.totalTransferSize - baseline.totalTransferSize) / baseline.totalTransferSize) * 100 : 0,
        improved: current.totalTransferSize < baseline.totalTransferSize
      }
    };
  }

  isScoreImproved(baselineScore, currentScore) {
    const scoreValues = { 'good': 3, 'needs-improvement': 2, 'poor': 1 };
    const baselineValue = scoreValues[baselineScore] || 0;
    const currentValue = scoreValues[currentScore] || 0;
    return currentValue > baselineValue;
  }

  generateComparisonSummary(differences) {
    const summary = {
      totalMetrics: 0,
      improved: 0,
      regressed: 0,
      unchanged: 0,
      significantChanges: [],
      overallImprovement: false
    };

    Object.entries(differences).forEach(([category, metrics]) => {
      if (metrics && typeof metrics === 'object') {
        Object.entries(metrics).forEach(([metric, data]) => {
          if (data && typeof data === 'object' && 'improved' in data) {
            summary.totalMetrics++;
            
            if (data.improved) {
              summary.improved++;
            } else if (data.difference !== 0) {
              summary.regressed++;
            } else {
              summary.unchanged++;
            }

            if (Math.abs(data.percentChange || 0) > 10) {
              summary.significantChanges.push({
                category: category,
                metric: metric,
                change: data.percentChange,
                improved: data.improved
              });
            }
          }
        });
      }
    });

    summary.overallImprovement = summary.improved > summary.regressed;
    summary.improvementRatio = summary.totalMetrics > 0 ? 
      summary.improved / summary.totalMetrics : 0;

    return summary;
  }

  getFinalMetrics() {
    return {
      tracking: {
        duration: performance.now() - this.startTime,
        observations: this.observations.length,
        comparisons: this.comparisons.length
      },
      current: this.captureCurrentState(),
      baselines: Object.fromEntries(this.baselines),
      comparisons: this.comparisons,
      issues: this.metrics.get('issues') || [],
      snapshots: {
        memory: this.metrics.get('memorySnapshots') || [],
        network: this.metrics.get('networkSnapshots') || []
      }
    };
  }

  extractDomain(url) {
    try {
      return new URL(url).hostname;
    } catch {
      return 'unknown';
    }
  }

  generateTrackingId() {
    return `track_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`;
  }

  generateComparisonId() {
    return `comp_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`;
  }

  resetMetrics() {
    this.metrics.clear();
    this.observations = [];
    this.comparisons = [];
  }

  cleanup() {
    this.observers.forEach(observer => {
      try {
        observer.disconnect();
      } catch (e) {}
    });
    this.observers = [];

    this.intervals.forEach(interval => {
      clearInterval(interval);
    });
    this.intervals = [];
  }

  exportMetrics() {
    return {
      metadata: {
        exportedAt: new Date().toISOString(),
        trackingDuration: this.isTracking ? performance.now() - this.startTime : 0,
        isTracking: this.isTracking
      },
      metrics: this.getFinalMetrics(),
      options: this.trackingOptions,
      thresholds: this.thresholds
    };
  }

  setThresholds(newThresholds) {
    this.thresholds = { ...this.thresholds, ...newThresholds };
  }

  getThresholds() {
    return { ...this.thresholds };
  }

  getObservations() {
    return [...this.observations];
  }

  getComparisons() {
    return [...this.comparisons];
  }

  getBaselines() {
    return new Map(this.baselines);
  }

  clearHistory() {
    this.observations = [];
    this.comparisons = [];
    this.baselines.clear();
    this.metrics.clear();
  }
}

if (typeof module !== 'undefined' && module.exports) {
  module.exports = PerformanceTracker;
} else if (typeof window !== 'undefined') {
  window.PerformanceTracker = PerformanceTracker;
}