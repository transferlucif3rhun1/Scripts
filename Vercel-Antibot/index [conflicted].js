const cluster = require("cluster");
const os = require("os");

if (cluster.isMaster) {
  console.log(`[MASTER] Starting TLS service...`);
  
  let isLibraryReady = false;
  let numWorkers = 0;
  const targetWorkers = os.cpus().length;
  
  const createWorker = () => {
    const worker = cluster.fork();
    numWorkers++;
    
    worker.on('message', (msg) => {
      if (msg.type === 'LIBRARY_READY') {
        isLibraryReady = true;
        console.log(`[MASTER] Service ready, starting ${targetWorkers - 1} additional workers`);
        
        for (let i = 1; i < targetWorkers; i++) {
          createWorker();
        }
      }
    });
    
    worker.on('error', (error) => {
      console.error(`[WORKER-${worker.process.pid}] Error: ${error.message}`);
      numWorkers--;
      if (isLibraryReady || numWorkers === 0) {
        createWorker();
      }
    });
    
    worker.on('exit', (code, signal) => {
      console.log(`[WORKER-${worker.process.pid}] Restarting worker`);
      numWorkers--;
      createWorker();
    });
  };
  
  createWorker();
  
  process.on('SIGTERM', () => {
    console.log('[MASTER] Shutting down');
    for (const id in cluster.workers) {
      cluster.workers[id].kill();
    }
    process.exit(0);
  });
  
} else {
  const fs = require("node:fs");
  const { Go } = require("./wasm");
  const express = require("express");
  const koffi = require("koffi");
  const path = require("path");
  const axios = require('axios');
  
  let wasmData = null;
  let tlsRequest = null;
  const sessions = new Map();
  
  const CONFIG = {
    TIMEOUT: 20000,
    CLEANUP_INTERVAL: 30000,
    SESSION_TIMEOUT: 60000
  };
  
  const app = express();
  app.use(express.json({ limit: '1mb' }));
  
  const detectPlatform = () => {
    const platform = os.platform();
    const arch = os.arch();
    
    let filename, extension;
    
    switch (platform) {
      case 'win32':
        extension = '.dll';
        filename = arch === 'x64' ? 'tls-client-windows-64' : 'tls-client-windows-32';
        break;
      case 'darwin':
        extension = '.dylib';
        filename = arch === 'arm64' ? 'tls-client-darwin-arm64' : 'tls-client-darwin-amd64';
        break;
      case 'linux':
        extension = '.so';
        filename = arch === 'arm64' ? 'tls-client-linux-arm64' : 'tls-client-linux-amd64';
        break;
      default:
        throw new Error(`Unsupported platform: ${platform}-${arch}`);
    }
    
    return { filename, extension, platform, arch };
  };
  
  const createHttpClient = () => {
    return axios.create({
      timeout: CONFIG.TIMEOUT,
      headers: {
        'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:139.0) Gecko/20100101 Firefox/139.0',
        'Accept': 'application/vnd.github+json',
        'Accept-Language': 'en-GB,en;q=0.5',
        'Accept-Encoding': 'gzip, deflate, br',
        'DNT': '1',
        'Sec-GPC': '1',
        'Connection': 'keep-alive',
        'Sec-Fetch-Dest': 'empty',
        'Sec-Fetch-Mode': 'cors',
        'Sec-Fetch-Site': 'cross-site',
        'Priority': 'u=0, i',
        'X-GitHub-Api-Version': '2022-11-28'
      },
      validateStatus: () => true
    });
  };
  
  const getLatestRelease = async () => {
    try {
      const client = createHttpClient();
      const response = await client.get('https://api.github.com/repos/bogdanfinn/tls-client/releases/latest');
      
      if (response.status === 403) {
        throw new Error('GitHub API rate limited');
      }
      
      if (response.status !== 200) {
        throw new Error(`GitHub API error ${response.status}`);
      }
      
      if (!response.data.assets || !Array.isArray(response.data.assets)) {
        throw new Error('No assets found');
      }
      
      return response.data;
    } catch (error) {
      if (error.code === 'ECONNABORTED' || error.code === 'ECONNRESET') {
        throw new Error('GitHub API timeout');
      }
      throw error;
    }
  };
  
  const downloadFile = async (url, dest) => {
    try {
      const client = createHttpClient();
      client.defaults.headers['Accept'] = 'application/octet-stream,*/*;q=0.9';
      client.defaults.headers['Sec-Fetch-Dest'] = 'document';
      client.defaults.headers['Sec-Fetch-Mode'] = 'navigate';
      client.defaults.headers['Sec-Fetch-Site'] = 'cross-site';
      
      const response = await client.get(url, {
        responseType: 'arraybuffer',
        maxRedirects: 5
      });
      
      if (response.status !== 200) {
        throw new Error(`Download failed: ${response.status}`);
      }
      
      fs.writeFileSync(dest, Buffer.from(response.data));
      
    } catch (error) {
      if (fs.existsSync(dest)) fs.unlinkSync(dest);
      if (error.code === 'ECONNABORTED' || error.code === 'ECONNRESET') {
        throw new Error('Download timeout');
      }
      throw error;
    }
  };
  
  const downloadTLSClient = async () => {
    const platform = detectPlatform();
    const tmpDir = path.join(os.tmpdir(), 'tls-client');
    
    if (!fs.existsSync(tmpDir)) {
      fs.mkdirSync(tmpDir, { recursive: true });
    }
    
    const existingFiles = fs.readdirSync(tmpDir).filter(file => 
      file.startsWith(platform.filename) && file.endsWith(platform.extension)
    );
    
    try {
      console.log(`[MASTER] Checking for updates using axios...`);
      const release = await getLatestRelease();
      const remoteVersion = release.tag_name;
      
      const asset = release.assets.find(asset => 
        asset.name.startsWith(platform.filename) && 
        asset.name.endsWith(platform.extension)
      );
      
      if (!asset) {
        throw new Error(`Binary not found for ${platform.filename}*${platform.extension}`);
      }
      
      if (existingFiles.length > 0) {
        const existingFile = existingFiles[0];
        const existingVersion = existingFile.match(/-(\d+\.\d+\.\d+)/)?.[1];
        const currentVersion = remoteVersion.replace('v', '');
        
        if (existingVersion === currentVersion) {
          console.log(`[MASTER] Using cached library v${currentVersion}`);
          return path.join(tmpDir, existingFile);
        } else {
          console.log(`[MASTER] Updating library v${existingVersion} → v${currentVersion}`);
          existingFiles.forEach(file => {
            fs.unlinkSync(path.join(tmpDir, file));
          });
        }
      }
      
      const newLibPath = path.join(tmpDir, asset.name);
      console.log(`[MASTER] Downloading library v${remoteVersion}`);
      await downloadFile(asset.browser_download_url, newLibPath);
      return newLibPath;
      
    } catch (error) {
      if (existingFiles.length > 0) {
        const existingFile = existingFiles[0];
        const existingVersion = existingFile.match(/-(\d+\.\d+\.\d+)/)?.[1] || 'unknown';
        console.log(`[MASTER] Using cached library v${existingVersion} (GitHub unavailable)`);
        return path.join(tmpDir, existingFile);
      } else {
        throw new Error(`No cached library available and GitHub API failed`);
      }
    }
  };
  
  const loadTLSLibrary = async () => {
    const libPath = await downloadTLSClient();
    const lib = koffi.load(libPath);
    tlsRequest = lib.func("__stdcall", "request", "string", ["string"]);
    console.log(`[MASTER] Library loaded successfully`);
    return true;
  };
  
  const makeRequest = (payload) => new Promise((resolve, reject) => {
    const timeoutId = setTimeout(() => {
      reject(new Error('Request timeout'));
    }, CONFIG.TIMEOUT);
    
    try {
      tlsRequest.async(JSON.stringify(payload), (err, response) => {
        clearTimeout(timeoutId);
        if (err) return reject(new Error(`TLS error: ${err}`));
        try {
          resolve(JSON.parse(response));
        } catch (e) {
          reject(new Error(`Parse error: ${e.message}`));
        }
      });
    } catch (error) {
      clearTimeout(timeoutId);
      reject(error);
    }
  });
  
  const createPayload = (config, sessionId) => ({
    tlsClientIdentifier: "chrome_131",
    followRedirects: false,
    insecureSkipVerify: false,
    withoutCookieJar: true,
    withDefaultCookieJar: false,
    forceHttp1: false,
    withRandomTLSExtensionOrder: true,
    timeoutSeconds: Math.floor(CONFIG.TIMEOUT / 1000),
    sessionId,
    proxyUrl: config.proxy || "",
    isRotatingProxy: false,
    certificatePinningHosts: {},
    ...config
  });
  
  const solveChallenge = async (token) => {
    try {
      const go = new Go();
      const { instance } = await WebAssembly.instantiate(wasmData, go.importObject);
      
      const timeout = setTimeout(() => {
        throw new Error('WASM timeout');
      }, 10000);
      
      go.run(instance);
      const result = JSON.parse(await go._values[go._values.length - 1](token));
      clearTimeout(timeout);
      
      try {
        if (go.exit) go.exit();
      } catch (e) {}
      
      return result;
    } catch (error) {
      throw new Error(`Challenge solving failed`);
    }
  };
  
  const formatProxy = (proxy) => {
    if (!proxy) return "";
    if (proxy.startsWith("http")) return proxy;
    
    const parts = proxy.split(":");
    if (parts.length < 2) return "";
    
    const [ip, port, user, pass] = parts;
    return user && pass ? `http://${user}:${pass}@${ip}:${port}` : `http://${ip}:${port}`;
  };
  
  const cleanup = () => {
    const now = Date.now();
    let cleaned = 0;
    
    for (const [id, data] of sessions.entries()) {
      if (now - data.lastUsed > CONFIG.SESSION_TIMEOUT) {
        sessions.delete(id);
        cleaned++;
      }
    }
  };
  
  setInterval(cleanup, CONFIG.CLEANUP_INTERVAL);
  
  app.post("/challenge", async (req, res) => {
    let sessionId = null;
    
    try {
      const { proxy } = req.body || {};
      
      const proxyFormatted = formatProxy(proxy);
      sessionId = `s_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`;
      
      sessions.set(sessionId, { lastUsed: Date.now(), requests: 0 });
      
      const useragent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36";
      
      const baseHeaders = {
        "cache-control": "max-age=0",
        "sec-ch-ua": '"Google Chrome";v="131", "Chromium";v="131", "Not_A Brand";v="24"',
        "sec-ch-ua-mobile": "?0",
        "sec-ch-ua-platform": '"Windows"',
        "upgrade-insecure-requests": "1",
        "user-agent": useragent,
        "accept": "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7",
        "sec-gpc": "1",
        "accept-language": "en-US,en;q=0.9",
        "sec-fetch-site": "same-origin",
        "sec-fetch-mode": "navigate",
        "sec-fetch-user": "?1",
        "sec-fetch-dest": "document",
        "accept-encoding": "gzip, deflate, br, zstd",
        "priority": "u=0, i"
      };
      
      const headerOrder = [
        "cache-control", "sec-ch-ua", "sec-ch-ua-mobile", "sec-ch-ua-platform",
        "upgrade-insecure-requests", "user-agent", "accept", "sec-gpc",
        "accept-language", "sec-fetch-site", "sec-fetch-mode", "sec-fetch-user",
        "sec-fetch-dest", "accept-encoding", "priority"
      ];
      
      // Make initial request to get challenge token
      const initResponse = await makeRequest(createPayload({
        headers: baseHeaders,
        headerOrder,
        requestUrl: "https://packdraw.com/en",
        requestMethod: "GET",
        requestBody: "",
        requestCookies: [],
        proxy: proxyFormatted
      }, sessionId));
      
      if (initResponse.status === 403) {
        throw new Error("Access blocked by security system");
      }
      
      const challengeToken = initResponse.headers?.["X-Vercel-Challenge-Token"];
      if (!challengeToken?.[0]) {
        if (initResponse.status === 429) {
          throw new Error("Rate limited by security system");
        }
        throw new Error("Security challenge required");
      }
      
      const token = challengeToken[0];
      const challengeResult = await solveChallenge(token);
      if (!challengeResult?.solution) {
        throw new Error("Security challenge failed");
      }
      
      // Solve the challenge to get cookies
      const solveResponse = await makeRequest(createPayload({
        headers: {
          ...baseHeaders,
          "sec-ch-ua": '"Google Chrome";v="131", "Chromium";v="131", "Not_A Brand";v="24"',
          "accept": "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8",
          "accept-language": "en-US,en;q=0.7",
          "referer": "https://packdraw.com/.well-known/vercel/security/static/challenge.v2.min.js",
          "x-vercel-challenge-token": token,
          "x-vercel-challenge-solution": challengeResult.solution,
          "x-vercel-challenge-version": "2"
        },
        headerOrder: [
          "cache-control", "sec-ch-ua", "sec-ch-ua-mobile", "sec-ch-ua-platform",
          "upgrade-insecure-requests", "user-agent", "accept", "sec-gpc",
          "accept-language", "sec-fetch-site", "sec-fetch-mode", "sec-fetch-user",
          "sec-fetch-dest", "referer", "accept-encoding", "priority"
        ],
        requestUrl: "https://packdraw.com/.well-known/vercel/security/request-challenge",
        requestMethod: "POST",
        requestBody: "",
        requestCookies: [],
        proxy: proxyFormatted
      }, sessionId));
      
      if (solveResponse.status === 403) {
        throw new Error("Security challenge rejected");
      }
      
      const cookies = solveResponse.headers?.["Set-Cookie"];
      if (!Array.isArray(cookies)) {
        throw new Error("Authentication failed");
      }
      
      const vcrcsMatch = cookies.find(cookie => cookie.includes('_vcrcs='));
      if (!vcrcsMatch) {
        throw new Error("Authentication cookie missing");
      }
      
      const vcrcs = vcrcsMatch.split('_vcrcs=')[1].split(';')[0];
      if (!vcrcs) {
        throw new Error("Invalid authentication cookie");
      }
      
      res.status(200).json({
        cookies: {
          _vcrcs: vcrcs
        }
      });
      
    } catch (error) {
      if (sessionId) {
        sessions.delete(sessionId);
      }
      
      let statusCode = 500;
      let errorType = "UNKNOWN";
      
      if (error.message.includes('timeout')) {
        statusCode = 408;
        errorType = "TIMEOUT";
      } else if (error.message.includes('blocked') || error.message.includes('403')) {
        statusCode = 403;
        errorType = "ACCESS_BLOCKED";
      } else if (error.message.includes('Rate limited') || error.message.includes('429')) {
        statusCode = 429;
        errorType = "RATE_LIMITED";
      } else if (error.message.includes('challenge') || error.message.includes('Security')) {
        statusCode = 400;
        errorType = "CHALLENGE_FAILED";
      } else if (error.message.includes('Authentication')) {
        statusCode = 401;
        errorType = "AUTH_FAILED";
      }
      
      res.status(statusCode).json({ 
        error: error.message,
        type: errorType
      });
    }
  });
  
  const initWorker = async () => {
    try {
      if (!fs.existsSync("./data/challenge.v2.wasm")) {
        throw new Error("WASM file not found: ./data/challenge.v2.wasm");
      }
      wasmData = fs.readFileSync("./data/challenge.v2.wasm");
      
      if (!tlsRequest && cluster.worker.id === 1) {
        await loadTLSLibrary();
        process.send({ type: 'LIBRARY_READY' });
      } else if (cluster.worker.id !== 1) {
        const platform = detectPlatform();
        const tmpDir = path.join(os.tmpdir(), 'tls-client');
        
        if (fs.existsSync(tmpDir)) {
          const existingFiles = fs.readdirSync(tmpDir).filter(file => 
            file.startsWith(platform.filename) && file.endsWith(platform.extension)
          );
          
          if (existingFiles.length > 0) {
            const libPath = path.join(tmpDir, existingFiles[0]);
            const lib = koffi.load(libPath);
            tlsRequest = lib.func("__stdcall", "request", "string", ["string"]);
          }
        }
      }
      
      const port = process.env.PORT || 3000;
      app.listen(port, () => {
        if (cluster.worker.id === 1) {
          console.log(`[WORKER-${process.pid}] Service ready on port ${port}`);
        }
      });
      
    } catch (error) {
      console.error(`[WORKER-${process.pid}] Failed to start: ${error.message}`);
      process.exit(1);
    }
  };
  
  process.on('uncaughtException', (error) => {
    console.error(`[WORKER-${process.pid}] Critical error: ${error.message}`);
    process.exit(1);
  });
  
  process.on('unhandledRejection', (reason) => {
    console.error(`[WORKER-${process.pid}] Unhandled error: ${reason}`);
    process.exit(1);
  });
  
  process.on('SIGTERM', () => {
    sessions.clear();
    process.exit(0);
  });
  
  initWorker();
}