const express = require("express");
const axios = require("axios");
const fs = require("fs");
const path = require("path");
const os = require("os");
const { spawn } = require("child_process");

const NODE_PORT = process.env.PORT || 3000;
const TLS_BASE_PORT = 8080;
const TLS_INSTANCE_COUNT = 10;
const TLS_API_KEY = "my-auth-key-1";

const TLS_INSTANCES = [];
for (let i = 0; i < TLS_INSTANCE_COUNT; i++) {
  const apiPort = TLS_BASE_PORT + i * 2;
  const healthPort = apiPort + 1;
  TLS_INSTANCES.push({
    id: i,
    apiPort,
    healthPort,
    configFile: `tls-config-${apiPort}.yml`,
    process: null,
    baseUrl: `http://127.0.0.1:${apiPort}`,
    healthUrl: `http://127.0.0.1:${healthPort}`,
  });
}

const app = express();
const tempDir = path.join(os.tmpdir(), "tls-client-api");
const sessionTracking = new Map();
let currentInstanceIndex = 0;

function log(message) {
  console.log(`[${new Date().toISOString()}] ${message}`);
}

function logError(message, error) {
  console.error(
    `[${new Date().toISOString()}] ERROR: ${message}`,
    error?.message || error || "",
  );
}

app.use(express.raw({ 
  type: "*/*", 
  limit: "50mb",
  verify: (req, res, buf, encoding) => {
    if (buf && buf.length === 0) {
      req.body = Buffer.alloc(0);
    }
  }
}));

function getNextTlsInstance() {
  const instance = TLS_INSTANCES[currentInstanceIndex];
  currentInstanceIndex = (currentInstanceIndex + 1) % TLS_INSTANCES.length;
  return instance;
}

function getArchitecturePattern() {
  const platform = os.platform();
  const arch = os.arch();

  if (platform === "win32") {
    return arch === "x64" ? "windows-64" : "windows-32";
  } else if (platform === "linux") {
    return "linux-amd64";
  } else if (platform === "darwin") {
    return arch === "arm64" ? "darwin-arm64" : "darwin-amd64";
  }
  throw new Error(`Unsupported platform: ${platform} (${arch})`);
}

async function getLatestRelease() {
  const response = await axios.get(
    "https://api.github.com/repos/bogdanfinn/tls-client-api/releases/latest",
    { timeout: 10000 }
  );

  const release = response.data;
  const version = release.tag_name;
  const assets = release.assets;
  const archPattern = getArchitecturePattern();
  const asset = assets.find(
    (asset) =>
      asset.name.includes("tls-client-api") && asset.name.includes(archPattern),
  );

  if (!asset) {
    const availableAssets = assets.map((a) => a.name).join(", ");
    throw new Error(
      `No asset found for architecture pattern '${archPattern}'. Available: ${availableAssets}`,
    );
  }

  log(`Latest release: ${version} - ${asset.name}`);

  return {
    version,
    filename: asset.name,
    downloadUrl: asset.browser_download_url,
  };
}

async function downloadFile(url, filePath, description) {
  const response = await axios.get(url, {
    responseType: "stream",
    timeout: 30000,
  });

  const writer = fs.createWriteStream(filePath);
  response.data.pipe(writer);

  await new Promise((resolve, reject) => {
    writer.on("finish", resolve);
    writer.on("error", reject);
  });

  log(`Downloaded: ${description}`);
}

async function downloadTlsClient(releaseInfo) {
  const { filename, downloadUrl, version } = releaseInfo;
  const binaryPath = path.join(tempDir, filename);
  const configTemplatePath = path.join(tempDir, "config.dist.yml");

  if (!fs.existsSync(tempDir)) {
    fs.mkdirSync(tempDir, { recursive: true });
  }

  await downloadFile(downloadUrl, binaryPath, filename);

  if (!fs.existsSync(configTemplatePath)) {
    const configUrl = `https://github.com/bogdanfinn/tls-client-api/releases/download/${version}/config.dist.yml`;
    await downloadFile(configUrl, configTemplatePath, "config.dist.yml");
  }

  try {
    fs.chmodSync(binaryPath, "755");
  } catch (error) {}

  return binaryPath;
}

function cleanupOldFiles() {
  if (!fs.existsSync(tempDir)) {
    return;
  }

  const files = fs.readdirSync(tempDir);
  
  const oldFiles = ["version.txt", "filename.txt", "config.yml"];
  oldFiles.forEach((oldFile) => {
    const oldPath = path.join(tempDir, oldFile);
    if (fs.existsSync(oldPath)) {
      try {
        fs.unlinkSync(oldPath);
        log(`Cleaned up old file: ${oldFile}`);
      } catch (error) {
        logError(`Failed to delete old file: ${oldFile}`, error);
      }
    }
  });

  const oldConfigFiles = files.filter(file => file.startsWith("tls-config-") && file.endsWith(".yml"));
  oldConfigFiles.forEach((configFile) => {
    const configPath = path.join(tempDir, configFile);
    try {
      fs.unlinkSync(configPath);
      log(`Cleaned up old config: ${configFile}`);
    } catch (error) {
      logError(`Failed to delete old config: ${configFile}`, error);
    }
  });
}

function getAllTlsBinaries() {
  if (!fs.existsSync(tempDir)) {
    return [];
  }

  const files = fs.readdirSync(tempDir);
  return files.filter((file) => {
    if (!file.startsWith("tls-client-api")) return false;
    if (file.endsWith(".exe")) return true;
    if (file.includes("linux-") || file.includes("darwin-")) {
      return !file.includes(".") || file.endsWith(".tar.gz");
    }
    return false;
  }).map(filename => ({
    filename,
    path: path.join(tempDir, filename)
  }));
}

function getCurrentBinary() {
  cleanupOldFiles();
  
  const allBinaries = getAllTlsBinaries();
  if (allBinaries.length === 0) {
    return null;
  }

  const validBinary = allBinaries.find(binary => {
    return fs.existsSync(binary.path) && !binary.filename.endsWith(".tar.gz");
  });

  return validBinary || null;
}

function cleanupAllOldBinaries(excludeFilename = null) {
  const allBinaries = getAllTlsBinaries();
  
  allBinaries.forEach(binary => {
    if (excludeFilename && binary.filename === excludeFilename) {
      return;
    }
    
    try {
      if (fs.existsSync(binary.path)) {
        fs.unlinkSync(binary.path);
        log(`Deleted old binary: ${binary.filename}`);
      }
    } catch (error) {
      logError(`Failed to delete old binary: ${binary.filename}`, error);
    }
  });
}

async function ensureTlsClient() {
  const releaseInfo = await getLatestRelease();
  const { filename: latestFilename } = releaseInfo;
  const currentBinary = getCurrentBinary();
  const configTemplatePath = path.join(tempDir, "config.dist.yml");
  const configExists = fs.existsSync(configTemplatePath);

  if (currentBinary && currentBinary.filename === latestFilename && configExists) {
    log(`Using existing: ${currentBinary.filename}`);
    return currentBinary.path;
  }

  if (currentBinary && currentBinary.filename === latestFilename && !configExists) {
    const configUrl = `https://github.com/bogdanfinn/tls-client-api/releases/download/${releaseInfo.version}/config.dist.yml`;
    await downloadFile(configUrl, configTemplatePath, "config.dist.yml");
    return currentBinary.path;
  }

  if (currentBinary) {
    log(`Updating: ${currentBinary.filename} -> ${latestFilename}`);
    fs.unlinkSync(currentBinary.path);
  }

  return await downloadTlsClient(releaseInfo);
}

function createTlsInstanceConfig(instance) {
  const configTemplatePath = path.join(tempDir, "config.dist.yml");
  const configContent = fs.readFileSync(configTemplatePath, "utf8");

  const updatedConfig = configContent
    .replace(/port:\s*8080/, `port: ${instance.apiPort}`)
    .replace(/port:\s*8081/, `port: ${instance.healthPort}`)
    .replace(/api_auth_keys:.*/, `api_auth_keys: ["${TLS_API_KEY}"]`);

  const instanceConfigPath = path.join(tempDir, instance.configFile);
  fs.writeFileSync(instanceConfigPath, updatedConfig);

  log(
    `Created config: ${instance.configFile} (API: ${instance.apiPort}, Health: ${instance.healthPort})`,
  );
  return instanceConfigPath;
}

async function startTlsInstance(binaryPath, instance) {
  const configPath = createTlsInstanceConfig(instance);

  return new Promise((resolve, reject) => {
    instance.process = spawn(binaryPath, ["--config", configPath], {
      cwd: tempDir,
      stdio: ["pipe", "pipe", "pipe"],
    });

    let started = false;

    instance.process.stdout.on("data", (data) => {
      const chunk = data.toString();
      if (
        (chunk.includes(`:${instance.apiPort}`) ||
          chunk.includes("listening") ||
          chunk.includes("started")) &&
        !started
      ) {
        started = true;
        setTimeout(() => resolve(), 1000);
      }
    });

    instance.process.stderr.on("data", (data) => {
      const chunk = data.toString();
      if (chunk.includes("error") || chunk.includes("Error")) {
        log(`TLS Instance ${instance.id} Error: ${chunk.trim()}`);
      }
    });

    instance.process.on("error", (error) => {
      logError(`TLS Instance ${instance.id} process error`, error);
      reject(error);
    });

    instance.process.on("exit", (code, signal) => {
      log(
        `TLS Instance ${instance.id} exited (code: ${code}, signal: ${signal})`,
      );
      if (!started) {
        reject(
          new Error(`TLS Instance ${instance.id} exited early (code: ${code})`),
        );
      }
    });

    setTimeout(() => {
      if (!started) {
        reject(new Error(`TLS Instance ${instance.id} startup timeout`));
      }
    }, 15000);
  });
}

async function waitForTlsInstancesReady() {
  log("Waiting for all TLS instances to become ready...");

  const healthChecks = TLS_INSTANCES.map(async (instance) => {
    for (let i = 0; i < 30; i++) {
      try {
        await axios.get(`${instance.healthUrl}/health`, {
          headers: { "x-api-key": TLS_API_KEY },
          timeout: 2000,
        });
        log(`TLS Instance ${instance.id} (port ${instance.apiPort}) is ready`);
        return true;
      } catch (error) {
        await new Promise((resolve) => setTimeout(resolve, 1000));
      }
    }
    throw new Error(`TLS Instance ${instance.id} failed to become ready`);
  });

  await Promise.all(healthChecks);
  return true;
}

function parseProxyUrl(proxy) {
  if (!proxy) return null;
  if (proxy.includes("://")) return proxy;

  const parts = proxy.split(":");
  if (parts.length === 2) {
    return `http://${proxy}`;
  } else if (parts.length === 4) {
    return `http://${parts[2]}:${parts[3]}@${parts[0]}:${parts[1]}`;
  }

  return `http://${proxy}`;
}

function parseCookies(cookieHeader) {
  if (!cookieHeader || typeof cookieHeader !== 'string') return [];

  return cookieHeader.split(";").map((cookie) => {
    const [name, ...valueParts] = cookie.trim().split("=");
    return { 
      name: (name || "").trim(), 
      value: (valueParts.join("=") || "").trim() 
    };
  }).filter(cookie => cookie.name);
}

function detectHttpVersion(headers) {
  const headerNames = Object.keys(headers);
  const hasLowerCase = headerNames.some((name) => name !== name.toLowerCase());
  return hasLowerCase ? 1 : 2;
}

function normalizeValue(value) {
  if (value === null || value === undefined) return null;
  if (typeof value === 'string' || typeof value === 'number') return String(value);
  if (Array.isArray(value)) {
    return value.length === 1 ? String(value[0]) : value.map(v => String(v)).join(", ");
  }
  return String(value);
}

function processHeaders(headers, method, body, httpVersion) {
  const processedHeaders = {};
  const headerOrder = [];
  const internalHeaders = [
    "x-client", "x-url", "x-proxy", "x-redirect", "x-sid", "x-http",
  ];

  let cookieHeader = null;

  Object.keys(headers).forEach((key) => {
    const lowerKey = key.toLowerCase();

    if (internalHeaders.includes(lowerKey)) return;

    if (lowerKey === "cookie") {
      cookieHeader = headers[key];
      return;
    }

    if (httpVersion === 1 && ["host", "connection"].includes(lowerKey)) {
      const index = Object.keys(headers).indexOf(key);
      if (index <= 1) return;
    }

    processedHeaders[key] = headers[key];
    headerOrder.push(key);
  });

  if (httpVersion === 2 && body && hasValidBody(body) && 
      !processedHeaders["content-length"] && !processedHeaders["Content-Length"]) {
    const contentLength = getBodyLength(body);
    if (contentLength > 0) {
      processedHeaders["content-length"] = contentLength.toString();
      headerOrder.unshift("content-length");
    }
  }

  return { headers: processedHeaders, headerOrder, cookieHeader };
}

function hasValidBody(body) {
  if (!body) return false;
  if (Buffer.isBuffer(body)) return body.length > 0;
  if (typeof body === 'string') return body.length > 0;
  return false;
}

function getBodyLength(body) {
  if (Buffer.isBuffer(body)) return body.length;
  if (typeof body === 'string') return Buffer.byteLength(body, "utf8");
  return 0;
}

function processRequestBody(body) {
  if (!body) return null;
  
  if (Buffer.isBuffer(body)) {
    return body.length > 0 ? body.toString() : null;
  }
  
  if (typeof body === 'string') {
    return body.length > 0 ? body : null;
  }
  
  if (typeof body === 'object' && body.constructor === Object && Object.keys(body).length === 0) {
    return null;
  }
  
  try {
    return String(body);
  } catch (e) {
    return null;
  }
}

function buildPayload(req) {
  const headers = req.headers;
  const method = req.method;
  const body = req.body;

  const client = headers["x-client"] || "chrome_133";
  const url = headers["x-url"];
  const proxy = parseProxyUrl(headers["x-proxy"]);
  const redirect = headers["x-redirect"];
  const sessionId = headers["x-sid"];
  const forceHttp1 = headers["x-http"] === "1" || headers["x-http"] === "true";

  if (!url) {
    throw new Error("x-url header is required");
  }

  const httpVersion = forceHttp1 ? 1 : detectHttpVersion(headers);
  const { headers: processedHeaders, headerOrder, cookieHeader } = 
    processHeaders(headers, method, body, httpVersion);

  let followRedirects = true;
  if (redirect !== undefined) {
    followRedirects = redirect === "true" || redirect === "1";
  }

  const requestCookies = parseCookies(cookieHeader);
  const requestBody = processRequestBody(body);

  return {
    followRedirects,
    forceHttp1,
    headerOrder,
    headers: processedHeaders,
    insecureSkipVerify: false,
    isRotatingProxy: false,
    proxyUrl: proxy,
    requestBody,
    requestCookies,
    requestMethod: method,
    requestUrl: url,
    sessionId,
    timeoutSeconds: 30,
    tlsClientIdentifier: client,
    withDefaultCookieJar: true,
    withRandomTLSExtensionOrder: true,
  };
}

function trackSession(sessionId) {
  if (!sessionId) return;

  if (sessionTracking.has(sessionId)) {
    clearTimeout(sessionTracking.get(sessionId));
  }

  const timeout = setTimeout(async () => {
    try {
      const instance = getNextTlsInstance();
      await axios.post(
        `${instance.baseUrl}/api/free-session`,
        { sessionId },
        {
          headers: {
            "x-api-key": TLS_API_KEY,
            "Content-Type": "application/json",
          },
          timeout: 5000,
        },
      );
      sessionTracking.delete(sessionId);
    } catch (error) {
      logError(`Failed to free session ${sessionId}`, error);
    }
  }, 60000);

  sessionTracking.set(sessionId, timeout);
}

async function makeTlsRequest(payload) {
  const instance = getNextTlsInstance();
  return await axios.post(`${instance.baseUrl}/api/forward`, payload, {
    headers: {
      "x-api-key": TLS_API_KEY,
      "Content-Type": "application/json",
    },
    timeout: 30000,
  });
}

function setResponseHeaders(res, headers) {
  if (!headers || typeof headers !== "object") return;
  
  Object.entries(headers).forEach(([key, value]) => {
    try {
      const headerValue = normalizeValue(value);
      if (headerValue !== null) {
        res.set(key, headerValue);
      }
    } catch (error) {
      logError(`Header error: ${key}`, error);
    }
  });
}

function setResponseCookies(res, cookies) {
  if (!cookies || typeof cookies !== "object") return;
  
  Object.entries(cookies).forEach(([name, value]) => {
    try {
      const cookieValue = normalizeValue(value);
      if (cookieValue !== null) {
        res.cookie(name, cookieValue);
      }
    } catch (error) {
      logError(`Cookie error: ${name}`, error);
    }
  });
}

function sendResponseBody(res, body) {
  if (body === null || body === undefined) {
    res.send("");
    return;
  }
  
  if (typeof body === "string") {
    res.send(body);
    return;
  }
  
  try {
    res.json(body);
  } catch (error) {
    res.send(String(body));
  }
}

function formatCookiesString(cookies) {
  if (!cookies || typeof cookies !== "object") return "";
  
  const cookieStrings = [];
  Object.entries(cookies).forEach(([name, value]) => {
    try {
      const cookieValue = normalizeValue(value);
      if (cookieValue !== null && name) {
        cookieStrings.push(`${name}=${cookieValue}`);
      }
    } catch (error) {
      logError(`Cookie formatting error: ${name}`, error);
    }
  });
  
  return cookieStrings.join("; ");
}

app.all("/tls", async (req, res) => {
  try {
    const payload = buildPayload(req);

    if (payload.sessionId) {
      trackSession(payload.sessionId);
    }

    const response = await makeTlsRequest(payload);
    const tlsResponse = response.data;

    const statusCode = tlsResponse.status || 200;
    res.status(statusCode);

    setResponseHeaders(res, tlsResponse.headers);
    setResponseCookies(res, tlsResponse.cookies);
    sendResponseBody(res, tlsResponse.body);

  } catch (error) {
    logError("Request failed", error);

    if (error.response?.data) {
      const errorData = error.response.data;
      const statusCode = errorData.status || error.response.status || 500;
      res.status(statusCode);
      
      setResponseHeaders(res, errorData.headers);
      
      const errorBody = errorData.body || errorData.error || "Request failed";
      res.send(String(errorBody));
    } else {
      res.status(500).send("Internal server error");
    }
  }
});

app.all("/tls/raw", async (req, res) => {
  try {
    const payload = buildPayload(req);
    const response = await makeTlsRequest(payload);

    res.setHeader("Content-Type", "application/json");
    res.send(response.data);
  } catch (error) {
    logError("Raw endpoint error", error);

    if (error.response?.data) {
      res.status(error.response.status || 500);
      res.setHeader("Content-Type", "application/json");
      res.send(error.response.data);
    } else {
      res.status(500).json({
        success: false,
        error: error.message,
      });
    }
  }
});

app.all("/tls/payload", (req, res) => {
  try {
    const payload = buildPayload(req);
    res.json(payload);
  } catch (error) {
    logError("Payload error", error);
    res.status(400).json({
      success: false,
      error: error.message,
    });
  }
});

app.all("/tls/cookies", async (req, res) => {
  try {
    const payload = buildPayload(req);

    if (payload.sessionId) {
      trackSession(payload.sessionId);
    }

    const response = await makeTlsRequest(payload);
    const tlsResponse = response.data;

    const cookieString = formatCookiesString(tlsResponse.cookies);
    const statusCode = tlsResponse.status || 200;
    
    res.setHeader("Content-Type", "application/json");
    res.json({ 
      cookie: cookieString,
      status: statusCode
    });

  } catch (error) {
    logError("Cookies endpoint error", error);

    if (error.response?.data) {
      const errorData = error.response.data;
      const statusCode = errorData.status || error.response.status || 500;
      res.status(error.response.status || 500);
      res.setHeader("Content-Type", "application/json");
      res.json({ 
        cookie: "",
        status: statusCode
      });
    } else {
      res.status(500).json({ 
        cookie: "",
        status: 500
      });
    }
  }
});

function cleanup() {
  log("Shutting down TLS instances...");
  TLS_INSTANCES.forEach((instance) => {
    if (instance.process) {
      instance.process.kill();
    }
  });
  
  sessionTracking.forEach((timeout) => {
    clearTimeout(timeout);
  });
  sessionTracking.clear();
  
  cleanupOldFiles();
  
  process.exit(0);
}

process.on("SIGINT", cleanup);
process.on("SIGTERM", cleanup);

async function initialize() {
  try {
    log("TLS-Client-API Manager starting...");
    log(`Platform: ${os.platform()} ${os.arch()}`);
    log(
      `TLS Instances: ${TLS_INSTANCE_COUNT} (ports ${TLS_BASE_PORT}-${TLS_BASE_PORT + TLS_INSTANCE_COUNT * 2 - 1})`,
    );

    const binaryPath = await ensureTlsClient();
    log("Binary ready");

    log("Starting TLS instances...");
    const startPromises = TLS_INSTANCES.map((instance) =>
      startTlsInstance(binaryPath, instance),
    );
    await Promise.all(startPromises);
    log("All TLS instances started");

    await waitForTlsInstancesReady();
    log("All TLS instances ready");

    app.listen(NODE_PORT, () => {
      log(`Node.js server listening on port ${NODE_PORT}`);
      log("Endpoints: /tls, /tls/raw, /tls/payload, /tls/cookies");
      log(`Load balancing across ${TLS_INSTANCE_COUNT} TLS instances`);
      log("Ready for maximum performance requests");
    });
  } catch (error) {
    logError("Initialization failed", error);
    
    TLS_INSTANCES.forEach((instance) => {
      if (instance.process) {
        instance.process.kill();
      }
    });
    
    cleanupOldFiles();
    process.exit(1);
  }
}

initialize();