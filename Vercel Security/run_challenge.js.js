const fs = require('fs');
const https = require('https');
const http2 = require('http2');
const { URL } = require('url');
const { webcrypto } = require('crypto');

global.crypto = webcrypto;
global.self = global;
global.performance = require('perf_hooks').performance;

function log(level, message, data = null) {
  const timestamp = new Date().toISOString();
  const logEntry = `[${timestamp}] [${level.toUpperCase()}] ${message}`;
  console.log(logEntry);
  if (data) {
    console.log(JSON.stringify(data, null, 2));
  }
}

class GoWasmRunner {
  constructor() {
    this._callbackTimeouts = new Map();
    this._nextCallbackTimeoutID = 1;
    this.textEncoder = new TextEncoder("utf-8");
    this.textDecoder = new TextDecoder("utf-8");
    this.dataView = new DataView(new ArrayBuffer(8));
    this.outputBuffer = [];
    this.exited = false;
    
    const self = this;
    
    let loadValue = (addr) => {
      this.dataView.setBigInt64(0, addr, true);
      let f = this.dataView.getFloat64(0, true);
      if (f === 0) return;
      if (!isNaN(f)) return f;
      let id = addr & 0xffffffffn;
      return this._values[id];
    };

    let loadValueAtAddr = (addr) => {
      let encoded = new DataView(this._inst.exports.memory.buffer).getBigUint64(addr, true);
      return loadValue(encoded);
    };

    let storeValue = (value) => {
      if (typeof value == "number") {
        return isNaN(value) ? 9221120237041090560n : value === 0 ? 9221120237041090561n : (this.dataView.setFloat64(0, value, true), this.dataView.getBigInt64(0, true));
      }
      switch (value) {
        case undefined: return 0x0n;
        case null: return 9221120237041090562n;
        case true: return 9221120237041090563n;
        case false: return 9221120237041090564n;
      }
      let id = this._ids.get(value);
      if (id === undefined) {
        id = this._idPool.pop();
        if (id === undefined) {
          id = BigInt(this._values.length);
        }
        this._values[id] = value;
        this._goRefCounts[id] = 0;
        this._ids.set(value, id);
      }
      this._goRefCounts[id]++;
      let typeFlag = 0x1n;
      switch (typeof value) {
        case "string": typeFlag = 0x2n; break;
        case "symbol": typeFlag = 0x3n; break;
        case "function": typeFlag = 0x4n; break;
      }
      return id | (0x7ff80000n | typeFlag) << 0x20n;
    };

    let storeValueAtAddr = (addr, value) => {
      let encoded = storeValue(value);
      new DataView(this._inst.exports.memory.buffer).setBigUint64(addr, encoded, true);
    };

    let loadArgs = (ptr, len, count) => {
      let args = new Array(count);
      for (let i = 0; i < count; i++) {
        args[i] = loadValueAtAddr(ptr + i * 8);
      }
      return args;
    };

    let timeOffset = Date.now() - performance.now();
    
    this.importObject = {
      'wasi_snapshot_preview1': {
        'fd_write': function (fd, iovs, iovsCount, nwritten) {
          let written = 0;
          if (fd == 1) {
            for (let i = 0; i < iovsCount; i++) {
              let ptr = iovs + i * 8;
              let buf = new DataView(self._inst.exports.memory.buffer).getUint32(ptr + 0, true);
              let len = new DataView(self._inst.exports.memory.buffer).getUint32(ptr + 4, true);
              written += len;
              for (let j = 0; j < len; j++) {
                let byte = new DataView(self._inst.exports.memory.buffer).getUint8(buf + j);
                if (byte != 13) {
                  if (byte == 10) {
                    let line = self.textDecoder.decode(new Uint8Array(self.outputBuffer));
                    self.outputBuffer = [];
                    console.log(line);
                  } else {
                    self.outputBuffer.push(byte);
                  }
                }
              }
            }
          } else {
            console.error("invalid file descriptor:", fd);
          }
          new DataView(self._inst.exports.memory.buffer).setUint32(nwritten, written, true);
          return 0;
        },
        'fd_close': () => 0,
        'fd_fdstat_get': () => 0,
        'fd_seek': () => 0,
        'proc_exit': (code) => {
          throw "trying to exit with code " + code;
        },
        'random_get': (buf, bufLen) => {
          crypto.getRandomValues(new Uint8Array(self._inst.exports.memory.buffer, buf, bufLen));
          return 0;
        }
      },
      'gojs': {
        "runtime.ticks": () => timeOffset + performance.now(),
        "runtime.sleepTicks": (timeout) => {
          setTimeout(self._inst.exports.go_scheduler, timeout);
        },
        "syscall/js.finalizeRef": (addr) => {
          console.error("syscall/js.finalizeRef not implemented");
        },
        "syscall/js.stringVal": (ptr, len) => {
          let str = self.textDecoder.decode(new DataView(self._inst.exports.memory.buffer, ptr, len));
          return storeValue(str);
        },
        "syscall/js.valueGet": (addr, objPtr, propLen) => {
          let prop = self.textDecoder.decode(new DataView(self._inst.exports.memory.buffer, objPtr, propLen));
          let obj = loadValue(addr);
          let result = Reflect.get(obj, prop);
          return storeValue(result);
        },
        "syscall/js.valueSet": (objPtr, propPtr, propLen, valuePtr) => {
          let obj = loadValue(objPtr);
          let prop = self.textDecoder.decode(new DataView(self._inst.exports.memory.buffer, propPtr, propLen));
          let value = loadValue(valuePtr);
          Reflect.set(obj, prop, value);
        },
        "syscall/js.valueDelete": (objPtr, propPtr, propLen) => {
          let obj = loadValue(objPtr);
          let prop = self.textDecoder.decode(new DataView(self._inst.exports.memory.buffer, propPtr, propLen));
          Reflect.deleteProperty(obj, prop);
        },
        "syscall/js.valueIndex": (objPtr, index) => storeValue(Reflect.get(loadValue(objPtr), index)),
        "syscall/js.valueSetIndex": (objPtr, index, valuePtr) => {
          Reflect.set(loadValue(objPtr), index, loadValue(valuePtr));
        },
        "syscall/js.valueCall": (retPtr, objPtr, methodPtr, methodLen, argsPtr, argsLen, argsCount) => {
          let obj = loadValue(objPtr);
          let method = self.textDecoder.decode(new DataView(self._inst.exports.memory.buffer, methodPtr, methodLen));
          let args = loadArgs(argsPtr, argsLen, argsCount);
          try {
            let fn = Reflect.get(obj, method);
            storeValueAtAddr(retPtr, Reflect.apply(fn, obj, args));
            new DataView(self._inst.exports.memory.buffer).setUint8(retPtr + 8, 1);
          } catch (err) {
            storeValueAtAddr(retPtr, err);
            new DataView(self._inst.exports.memory.buffer).setUint8(retPtr + 8, 0);
          }
        },
        "syscall/js.valueInvoke": (retPtr, fnPtr, argsPtr, argsLen, argsCount) => {
          try {
            let fn = loadValue(fnPtr);
            let args = loadArgs(argsPtr, argsLen, argsCount);
            storeValueAtAddr(retPtr, Reflect.apply(fn, undefined, args));
            new DataView(self._inst.exports.memory.buffer).setUint8(retPtr + 8, 1);
          } catch (err) {
            storeValueAtAddr(retPtr, err);
            new DataView(self._inst.exports.memory.buffer).setUint8(retPtr + 8, 0);
          }
        },
        "syscall/js.valueNew": (retPtr, constructorPtr, argsPtr, argsLen, argsCount) => {
          let constructor = loadValue(constructorPtr);
          let args = loadArgs(argsPtr, argsLen, argsCount);
          try {
            storeValueAtAddr(retPtr, Reflect.construct(constructor, args));
            new DataView(self._inst.exports.memory.buffer).setUint8(retPtr + 8, 1);
          } catch (err) {
            storeValueAtAddr(retPtr, err);
            new DataView(self._inst.exports.memory.buffer).setUint8(retPtr + 8, 0);
          }
        },
        "syscall/js.valueLength": (objPtr) => loadValue(objPtr).length,
        "syscall/js.valuePrepareString": (retPtr, objPtr) => {
          let str = String(loadValue(objPtr));
          let bytes = self.textEncoder.encode(str);
          storeValueAtAddr(retPtr, bytes);
          new DataView(self._inst.exports.memory.buffer).setInt32(retPtr + 8, bytes.length, true);
        },
        "syscall/js.valueLoadString": (objPtr, ptr, len, cap) => {
          let bytes = loadValue(objPtr);
          new Uint8Array(self._inst.exports.memory.buffer, ptr, len).set(bytes);
        },
        "syscall/js.valueInstanceOf": (objPtr, constructorPtr) => loadValue(objPtr) instanceof loadValue(constructorPtr),
        "syscall/js.copyBytesToGo": (retPtr, destPtr, destLen, destCap, srcPtr) => {
          let retAddr = retPtr + 4;
          let dest = new Uint8Array(self._inst.exports.memory.buffer, destPtr, destLen);
          let src = loadValue(srcPtr);
          if (!(src instanceof Uint8Array || src instanceof Uint8ClampedArray)) {
            new DataView(self._inst.exports.memory.buffer).setUint8(retAddr, 0);
            return;
          }
          let toCopy = src.subarray(0, dest.length);
          dest.set(toCopy);
          new DataView(self._inst.exports.memory.buffer).setUint32(retPtr, toCopy.length, true);
          new DataView(self._inst.exports.memory.buffer).setUint8(retAddr, 1);
        },
        "syscall/js.copyBytesToJS": (retPtr, destPtr, srcPtr, srcLen, srcCap) => {
          let retAddr = retPtr + 4;
          let dest = loadValue(destPtr);
          let src = new Uint8Array(self._inst.exports.memory.buffer, srcPtr, srcLen);
          if (!(dest instanceof Uint8Array || dest instanceof Uint8ClampedArray)) {
            new DataView(self._inst.exports.memory.buffer).setUint8(retAddr, 0);
            return;
          }
          let toCopy = src.subarray(0, dest.length);
          dest.set(toCopy);
          new DataView(self._inst.exports.memory.buffer).setUint32(retPtr, toCopy.length, true);
          new DataView(self._inst.exports.memory.buffer).setUint8(retAddr, 1);
        }
      }
    };
    this.importObject.env = this.importObject.gojs;
  }

  async run(instance) {
    log('debug', 'Starting WASM execution');
    this._inst = instance;
    this._values = [NaN, 0, null, true, false, global, this];
    this._goRefCounts = [];
    this._ids = new Map();
    this._idPool = [];
    this.exited = false;

    for (;;) {
      let callbackPromise = new Promise(resolve => {
        this._resolveCallbackPromise = () => {
          if (this.exited) {
            throw new Error("bad callback: Go program has already exited");
          }
          setTimeout(resolve, 0);
        };
      });
      this._inst.exports._start();
      if (this.exited) {
        break;
      }
      await callbackPromise;
    }
    log('debug', 'WASM execution completed');
  }

  _resume() {
    if (this.exited) {
      throw new Error("Go program has already exited");
    }
    this._inst.exports.resume();
    if (this.exited) {
      this._resolveExitPromise();
    }
  }

  _makeFuncWrapper(id) {
    let self = this;
    return function () {
      let event = {
        'id': id,
        'this': this,
        'args': arguments
      };
      self._pendingEvent = event;
      self._resume();
      return event.result;
    };
  }
}

class ChromeTLSClient {
  constructor() {
    this.sessionCookies = {};
  }

  async makeRequest(url, options = {}) {
    log('debug', `Making ${options.method || 'GET'} request to: ${url}`);

    const urlObj = new URL(url);
    const isHTTP2 = true;

    const defaultHeaders = {
      ':method': options.method || 'GET',
      ':authority': urlObj.host,
      ':scheme': urlObj.protocol.slice(0, -1),
      ':path': urlObj.pathname + urlObj.search,
      'user-agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36',
      'accept': options.method === 'POST' ? 'application/json, text/plain, */*' : 'text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7',
      'accept-language': 'en-US,en;q=0.9',
      'accept-encoding': 'gzip, deflate, br',
      'dnt': '1',
      'sec-ch-ua': '"Not_A Brand";v="8", "Chromium";v="120", "Google Chrome";v="120"',
      'sec-ch-ua-mobile': '?0',
      'sec-ch-ua-platform': '"Windows"',
      'sec-fetch-dest': options.method === 'POST' ? 'empty' : 'document',
      'sec-fetch-mode': options.method === 'POST' ? 'cors' : 'navigate',
      'sec-fetch-site': options.method === 'POST' ? 'same-origin' : 'none',
      'cache-control': 'max-age=0'
    };

    if (options.method === 'GET') {
      defaultHeaders['sec-fetch-user'] = '?1';
      defaultHeaders['upgrade-insecure-requests'] = '1';
    }

    const headers = { ...defaultHeaders, ...(options.headers || {}) };

    if (isHTTP2) {
      return this.makeHTTP2Request(urlObj, headers, options);
    } else {
      return this.makeHTTPSRequest(urlObj, headers, options);
    }
  }

  async makeHTTP2Request(urlObj, headers, options) {
    return new Promise((resolve, reject) => {
      const client = http2.connect(`${urlObj.protocol}//${urlObj.host}`, {
        settings: {
          headerTableSize: 65536,
          enablePush: false,
          maxConcurrentStreams: 1000,
          initialWindowSize: 6291456,
          maxFrameSize: 16384,
          maxHeaderListSize: 262144
        }
      });

      client.on('error', (err) => {
        log('error', `HTTP/2 client error: ${err.message}`);
        reject(err);
      });

      const req = client.request(headers);

      const chunks = [];
      let responseHeaders = {};
      let status = 200;

      req.on('response', (resHeaders) => {
        responseHeaders = { ...resHeaders };
        status = resHeaders[':status'] || 200;
        log('debug', `Response status: ${status}`);
      });

      req.on('data', (chunk) => {
        chunks.push(chunk);
      });

      req.on('end', () => {
        client.close();
        
        const data = Buffer.concat(chunks);
        
        const cookies = this.parseCookies(responseHeaders['set-cookie']);
        Object.assign(this.sessionCookies, cookies);

        resolve({
          ok: status >= 200 && status < 400,
          status: parseInt(status),
          headers: responseHeaders,
          data: data,
          text: data.toString('utf8'),
          statusText: `${status}`,
          cookies: cookies
        });
      });

      req.on('error', (err) => {
        log('error', `HTTP/2 request error: ${err.message}`);
        client.close();
        reject(err);
      });

      if (options.body) {
        req.write(options.body);
      }

      req.end();
    });
  }

  async makeHTTPSRequest(urlObj, headers, options) {
    return new Promise((resolve, reject) => {
      const requestOptions = {
        hostname: urlObj.hostname,
        port: urlObj.port || 443,
        path: urlObj.pathname + urlObj.search,
        method: options.method || 'GET',
        headers: this.convertHTTP2HeadersToHTTP1(headers),
        timeout: 30000,
        secureProtocol: 'TLSv1_3_method'
      };

      const req = https.request(requestOptions, (res) => {
        const chunks = [];
        
        log('debug', `Response status: ${res.statusCode}`);

        res.on('data', (chunk) => {
          chunks.push(chunk);
        });

        res.on('end', () => {
          const data = Buffer.concat(chunks);
          
          const cookies = this.parseCookies(res.headers['set-cookie']);
          Object.assign(this.sessionCookies, cookies);

          resolve({
            ok: res.statusCode >= 200 && res.statusCode < 400,
            status: res.statusCode,
            headers: res.headers,
            data: data,
            text: data.toString('utf8'),
            statusText: res.statusMessage,
            cookies: cookies
          });
        });
      });

      req.on('error', (err) => {
        log('error', `HTTPS request error: ${err.message}`);
        reject(err);
      });

      req.on('timeout', () => {
        req.destroy();
        reject(new Error('Request timeout'));
      });

      if (options.body) {
        req.write(options.body);
      }

      req.end();
    });
  }

  convertHTTP2HeadersToHTTP1(h2Headers) {
    const h1Headers = {};
    
    for (const [key, value] of Object.entries(h2Headers)) {
      if (!key.startsWith(':')) {
        h1Headers[key] = value;
      }
    }
    
    return h1Headers;
  }

  parseCookies(cookieHeaders) {
    const cookies = {};
    if (!cookieHeaders) return cookies;
    
    const cookieArray = Array.isArray(cookieHeaders) ? cookieHeaders : [cookieHeaders];
    
    for (const cookieHeader of cookieArray) {
      if (cookieHeader) {
        const parts = cookieHeader.split(';')[0].split('=');
        if (parts.length === 2) {
          cookies[parts[0].trim()] = parts[1].trim();
        }
      }
    }
    
    return cookies;
  }

  async downloadWasm(url) {
    log('info', `Downloading WASM from: ${url}`);
    
    const response = await this.makeRequest(url, {
      headers: {
        'accept': 'application/wasm,*/*;q=0.8'
      }
    });

    if (response.status !== 200) {
      throw new Error(`Failed to download WASM: ${response.status}`);
    }

    log('info', `WASM downloaded successfully: ${response.data.length} bytes`);
    return response.data;
  }
}

async function initializeWasm(wasmBytes) {
  log('debug', 'Initializing WASM module');
  const go = new GoWasmRunner();
  go.importObject.gojs["syscall/js.finalizeRef"] = () => null;
  const { instance } = await WebAssembly.instantiate(wasmBytes, go.importObject);
  go.run(instance);
  return { instance, go };
}

async function submitChallenge(client, token, solution, version) {
  log('info', 'Submitting challenge solution');
  const response = await client.makeRequest('https://packdraw.com/.well-known/vercel/security/request-challenge', {
    method: 'POST',
    headers: {
      'x-vercel-challenge-token': token,
      'x-vercel-challenge-solution': solution,
      'x-vercel-challenge-version': version
    }
  });

  if (!response.ok) {
    if (response.headers.get && response.headers.get("Cf-Mitigated") || response.headers["cf-mitigated"]) {
      let rayId = response.headers.get && response.headers.get("Cf-Ray") || response.headers["cf-ray"];
      let message = rayId ? "Ray ID: " + rayId : "Challenge blocked by Cloudflare";
      let error = new Error(message);
      error.__blocked = true;
      throw error;
    } else if (response.status === 401 || response.status === 403) {
      let error = new Error("Challenge blocked");
      error.__blocked = true;
      throw error;
    } else if (response.status === 404) {
      let error = new Error("Challenge not forwarded");
      error.__blocked = true;
      throw error;
    } else if (response.status >= 700) {
      let error = new Error(String(response.status));
      error.__failed = true;
      throw error;
    }
    throw new Error(response.statusText);
  }
  return response;
}

async function solveChallengeRequest(challengeData) {
  const client = new ChromeTLSClient();
  
  try {
    await initializeWasm(await client.downloadWasm('https://packdraw.com/.well-known/vercel/security/static/challenge.v2.wasm'));
    
    let result;
    try {
      let solution = await global.Solve(challengeData.token);
      result = JSON.parse(solution);
      let solutionValue = result.solution;
      await submitChallenge(client, challengeData.token, solutionValue, challengeData.version);
      return { type: "solve-response", success: true, token: '' };
    } catch (error) {
      let isBlocked = error != null && typeof error == "object" && "__blocked" in error;
      let isFailed = error != null && typeof error == "object" && "__failed" in error;
      if (isBlocked) {
        let message = error instanceof Error ? error.message : String(error);
        return { type: "solve-response", success: false, blocked: true, metadata: message ?? undefined, token: '' };
      } else if (result?.["badInfo"]) {
        return { type: "solve-response", success: false, blocked: false, metadata: result?.["badInfo"] ?? undefined, token: '' };
      } else if (isFailed) {
        let message = error instanceof Error ? error.message : String(error);
        return { type: "solve-response", success: false, blocked: false, metadata: message ?? undefined, token: '' };
      } else {
        return { type: "solve-response", success: false, blocked: false, metadata: undefined, token: '' };
      }
    }
  } catch (error) {
    log('error', 'Challenge solving failed:', error.message);
    return { type: "solve-response", success: false, blocked: false, metadata: error.message, token: '' };
  }
}

async function solvePackdrawChallenge() {
  const client = new ChromeTLSClient();
  
  try {
    log('info', '=== Starting Packdraw Challenge Solver ===');
    
    log('info', 'Step 1: Making initial request to packdraw.com');
    const initialResponse = await client.makeRequest('https://packdraw.com');
    
    log('info', `Initial response status: ${initialResponse.status}`);
    
    const challengeToken = initialResponse.headers['x-vercel-challenge-token'] || 
                          initialResponse.headers['X-Vercel-Challenge-Token'];
    if (!challengeToken) {
      log('info', 'No challenge token found - access granted without challenge');
      return { 
        success: true, 
        message: 'No challenge token found, access granted', 
        response: initialResponse.text.substring(0, 200) + '...'
      };
    }

    log('info', `Step 2: Challenge token detected: ${challengeToken.substring(0, 20)}...`);

    log('info', 'Step 3: Solving challenge');
    const challengeResult = await solveChallengeRequest({
      token: challengeToken,
      version: 'v2'
    });

    if (!challengeResult.success) {
      return {
        success: false,
        blocked: challengeResult.blocked || false,
        metadata: challengeResult.metadata || 'Challenge solving failed'
      };
    }

    log('info', 'Step 4: Making final verified request to packdraw.com');
    const finalResponse = await client.makeRequest('https://packdraw.com');
    
    log('info', `Final response status: ${finalResponse.status}`);
    log('info', '=== Challenge solved successfully ===');
    
    return { 
      success: true, 
      message: 'Challenge solved and validated successfully',
      challengeToken: challengeToken.substring(0, 20) + '...',
      finalStatus: finalResponse.status,
      response: finalResponse.text.substring(0, 500) + '...'
    };

  } catch (error) {
    log('error', `Challenge solving failed: ${error.message}`);
    
    if (error.__blocked) {
      return { success: false, blocked: true, metadata: error.message };
    }
    if (error.__failed) {
      return { success: false, blocked: false, metadata: error.message };
    }
    return { success: false, blocked: false, metadata: error.message || 'Unknown error' };
  }
}

if (require.main === module) {
  solvePackdrawChallenge().then(result => {
    log('info', 'Final result:', result);
  }).catch(error => {
    log('error', 'Unhandled error:', error);
  });
}

module.exports = { solvePackdrawChallenge };