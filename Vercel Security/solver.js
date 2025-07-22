const https = require('https');
const http = require('http');
const { URL } = require('url');
const fs = require('fs').promises;
const crypto = require('crypto');
const { performance } = require('perf_hooks');

let g = [];
const y = new TextEncoder();
const d = new TextDecoder();
const i = new DataView(new ArrayBuffer(8));

class GoRuntime {
  constructor() {
    this._callbackTimeouts = new Map();
    this._nextCallbackTimeoutID = 1;

    const loadValue = (addr) => {
      i.setBigInt64(0, addr, true);
      const f = i.getFloat64(0, true);
      if (f === 0) return undefined;
      if (!isNaN(f)) return f;
      const id = addr & 0xffffffffn;
      return this._values[id];
    };

    const storeValue = (addr, v) => {
      const nanHead = 0x7FF80000;
      if (typeof v === "number") {
        if (isNaN(v)) {
          new DataView(this._inst.exports.memory.buffer).setBigUint64(addr, 0x7FF8000000000000n, true);
          return;
        }
        if (v === 0) {
          new DataView(this._inst.exports.memory.buffer).setBigUint64(addr, 0x7FF8000000000001n, true);
          return;
        }
        i.setFloat64(0, v, true);
        new DataView(this._inst.exports.memory.buffer).setBigUint64(addr, i.getBigInt64(0, true), true);
        return;
      }

      switch (v) {
        case undefined:
          new DataView(this._inst.exports.memory.buffer).setBigUint64(addr, 0n, true);
          return;
        case null:
          new DataView(this._inst.exports.memory.buffer).setBigUint64(addr, 0x7FF8000000000002n, true);
          return;
        case true:
          new DataView(this._inst.exports.memory.buffer).setBigUint64(addr, 0x7FF8000000000003n, true);
          return;
        case false:
          new DataView(this._inst.exports.memory.buffer).setBigUint64(addr, 0x7FF8000000000004n, true);
          return;
      }

      let id = this._ids.get(v);
      if (id === undefined) {
        id = this._idPool.pop();
        if (id === undefined) {
          id = BigInt(this._values.length);
        }
        this._values[id] = v;
        this._goRefCounts[id] = 0;
        this._ids.set(v, id);
      }
      this._goRefCounts[id]++;

      let typeFlag = 1n;
      switch (typeof v) {
        case "string":
          typeFlag = 2n;
          break;
        case "symbol":
          typeFlag = 3n;
          break;
        case "function":
          typeFlag = 4n;
          break;
      }
      new DataView(this._inst.exports.memory.buffer).setBigUint64(addr, id | ((0x7FF80000n | typeFlag) << 32n), true);
    };

    const loadSliceOfValues = (addr, len) => {
      const a = new Array(len);
      for (let i = 0; i < len; i++) {
        a[i] = loadValue(new DataView(this._inst.exports.memory.buffer).getBigUint64(addr + i * 8, true));
      }
      return a;
    };

    const timeOrigin = Date.now() - performance.now();

    this.importObject = {
      wasi_snapshot_preview1: {
        fd_write: (fd, iovs, iovsLen, nwritten) => {
          let written = 0;
          if (fd === 1) {
            for (let i = 0; i < iovsLen; i++) {
              const iov = iovs + i * 8;
              const ptr = new DataView(this._inst.exports.memory.buffer).getUint32(iov, true);
              const len = new DataView(this._inst.exports.memory.buffer).getUint32(iov + 4, true);
              written += len;
              for (let j = 0; j < len; j++) {
                const byte = new DataView(this._inst.exports.memory.buffer).getUint8(ptr + j);
                if (byte !== 13) {
                  if (byte === 10) {
                    const line = d.decode(new Uint8Array(g));
                    g = [];
                    console.log(line);
                  } else {
                    g.push(byte);
                  }
                }
              }
            }
          }
          new DataView(this._inst.exports.memory.buffer).setUint32(nwritten, written, true);
          return 0;
        },
        fd_close: () => 0,
        fd_fdstat_get: () => 0,
        fd_seek: () => 0,
        proc_exit: (code) => {
          this.exited = true;
        },
        random_get: (ptr, len) => {
          crypto.getRandomValues(new Uint8Array(this._inst.exports.memory.buffer, ptr, len));
          return 0;
        }
      },
      gojs: {
        "runtime.ticks": () => timeOrigin + performance.now(),
        "runtime.sleepTicks": (timeout) => {
          setTimeout(this._inst.exports.go_scheduler, timeout);
        },
        "syscall/js.finalizeRef": (addr) => {
          const id = new DataView(this._inst.exports.memory.buffer).getBigUint64(addr, true) & 0xffffffffn;
          this._goRefCounts[id]--;
          if (this._goRefCounts[id] === 0) {
            const v = this._values[id];
            this._values[id] = null;
            this._ids.delete(v);
            this._idPool.push(id);
          }
        },
        "syscall/js.stringVal": (ret, ptr, len) => {
          const str = d.decode(new Uint8Array(this._inst.exports.memory.buffer, ptr, len));
          storeValue(ret, str);
        },
        "syscall/js.valueGet": (ret, v_addr, p_ptr, p_len) => {
          const v = loadValue(v_addr);
          const p = d.decode(new Uint8Array(this._inst.exports.memory.buffer, p_ptr, p_len));
          const result = Reflect.get(v, p);
          storeValue(ret, result);
        },
        "syscall/js.valueSet": (v_addr, p_ptr, p_len, x_addr) => {
          const v = loadValue(v_addr);
          const p = d.decode(new Uint8Array(this._inst.exports.memory.buffer, p_ptr, p_len));
          const x = loadValue(x_addr);
          Reflect.set(v, p, x);
        },
        "syscall/js.valueDelete": (v_addr, p_ptr, p_len) => {
            const v = loadValue(v_addr);
            const p = d.decode(new Uint8Array(this._inst.exports.memory.buffer, p_ptr, p_len));
            Reflect.deleteProperty(v, p);
        },
        "syscall/js.valueIndex": (ret, v_addr, i) => {
          storeValue(ret, Reflect.get(loadValue(v_addr), i));
        },
        "syscall/js.valueSetIndex": (v_addr, i, x_addr) => {
          Reflect.set(loadValue(v_addr), i, loadValue(x_addr));
        },
        "syscall/js.valueCall": (ret, v_addr, m_ptr, m_len, args_addr, args_len) => {
            const v = loadValue(v_addr);
            const m = d.decode(new Uint8Array(this._inst.exports.memory.buffer, m_ptr, m_len));
            const args = loadSliceOfValues(args_addr, args_len);
            try {
                const result = Reflect.apply(Reflect.get(v, m), v, args);
                storeValue(ret, result);
                new DataView(this._inst.exports.memory.buffer).setUint8(ret + 8, 1);
            } catch (err) {
                storeValue(ret, err);
                new DataView(this._inst.exports.memory.buffer).setUint8(ret + 8, 0);
            }
        },
        "syscall/js.valueInvoke": (ret, v_addr, args_addr, args_len) => {
            try {
                const v = loadValue(v_addr);
                const args = loadSliceOfValues(args_addr, args_len);
                const result = Reflect.apply(v, undefined, args);
                storeValue(ret, result);
                new DataView(this._inst.exports.memory.buffer).setUint8(ret + 8, 1);
            } catch (err) {
                storeValue(ret, err);
                new DataView(this._inst.exports.memory.buffer).setUint8(ret + 8, 0);
            }
        },
        "syscall/js.valueNew": (ret, v_addr, args_addr, args_len) => {
            const ctor = loadValue(v_addr);
            const args = loadSliceOfValues(args_addr, args_len);
            try {
                const result = Reflect.construct(ctor, args);
                storeValue(ret, result);
                new DataView(this._inst.exports.memory.buffer).setUint8(ret + 8, 1);
            } catch (err) {
                storeValue(ret, err);
                new DataView(this._inst.exports.memory.buffer).setUint8(ret + 8, 0);
            }
        },
        "syscall/js.valueLength": (v_addr) => loadValue(v_addr).length,
        "syscall/js.valuePrepareString": (ret, v_addr) => {
            const str = String(loadValue(v_addr));
            const-encoded = y.encode(str);
            storeValue(ret, encoded);
            new DataView(this._inst.exports.memory.buffer).setInt32(ret + 8, encoded.length, true);
        },
        "syscall/js.valueLoadString": (v_addr, b_ptr, b_len) => {
            const str = loadValue(v_addr);
            new Uint8Array(this._inst.exports.memory.buffer, b_ptr, b_len).set(str);
        },
        "syscall/js.valueInstanceOf": (v_addr, t_addr) => loadValue(v_addr) instanceof loadValue(t_addr),
        "syscall/js.copyBytesToGo": (ret, dst_addr, src_addr) => {
          const dst = new Uint8Array(this._inst.exports.memory.buffer, dst_addr, new DataView(this._inst.exports.memory.buffer).getBigUint64(dst_addr + 8, true));
          const src = loadValue(src_addr);
          if (!(src instanceof Uint8Array || src instanceof Uint8ClampedArray)) {
            new DataView(this._inst.exports.memory.buffer).setUint8(ret + 8, 0);
            return;
          }
          const toCopy = src.subarray(0, dst.length);
          dst.set(toCopy);
          new DataView(this._inst.exports.memory.buffer).setUint32(ret, toCopy.length, true);
          new DataView(this._inst.exports.memory.buffer).setUint8(ret + 8, 1);
        },
        "syscall/js.copyBytesToJS": (ret, dst_addr, src_addr, src_len) => {
            const dst = loadValue(dst_addr);
            const src = new Uint8Array(this._inst.exports.memory.buffer, src_addr, src_len);
            if (!(dst instanceof Uint8Array || dst instanceof Uint8ClampedArray)) {
              new DataView(this._inst.exports.memory.buffer).setUint8(ret + 8, 0);
              return;
            }
            const toCopy = src.subarray(0, dst.length);
            dst.set(toCopy);
            new DataView(this._inst.exports.memory.buffer).setUint32(ret, toCopy.length, true);
            new DataView(this._inst.exports.memory.buffer).setUint8(ret + 8, 1);
        }
      }
    };
    this.importObject.env = this.importObject.gojs;
  }

  async run(instance) {
    this._inst = instance;
    this._values = [NaN, 0, null, true, false, global, this];
    this._goRefCounts = [];
    this._ids = new Map();
    this._idPool = [];
    this.exited = false;
    
    const mem = new DataView(this._inst.exports.memory.buffer);
    
    while (!this.exited) {
      const sp = mem.getBigUint64(8, true);
      const callbackPromise = new Promise((resolve) => {
        this._resolveCallbackPromise = resolve;
      });
      this._inst.exports._start();
      if (this.exited) break;
      await callbackPromise;
    }
  }

  _resume() {
    if (this.exited) throw new Error("Go program has already exited");
    this._inst.exports.resume();
  }

  _makeFuncWrapper(id) {
    const go = this;
    return function() {
      const event = { id, this: this, args: arguments };
      go._pendingEvent = event;
      go._resume();
      return event.result;
    };
  }
}

function request(url, options = {}) {
  return new Promise((resolve, reject) => {
    const parsedUrl = new URL(url);
    const protocol = parsedUrl.protocol === 'https:' ? https : http;
    const req = protocol.request(parsedUrl, options, (res) => {
      const chunks = [];
      res.on('data', chunk => chunks.push(chunk));
      res.on('error', reject);
      res.on('end', () => {
        const body = Buffer.concat(chunks);
        resolve({
          ok: res.statusCode >= 200 && res.statusCode < 300,
          status: res.statusCode,
          statusText: res.statusMessage,
          headers: res.headers,
          text: () => Promise.resolve(body.toString()),
          buffer: () => Promise.resolve(body),
        });
      });
    });
    req.on('error', reject);
    if (options.body) {
      req.write(options.body);
    }
    req.end();
  });
}

async function loadWasm(wasmUrl) {
  const response = await request(wasmUrl);
  if (!response.ok) {
    throw new Error(`Failed to fetch WASM: ${response.status} ${response.statusText}`);
  }
  const wasmBuffer = await response.buffer();
  const go = new GoRuntime();
  const { instance } = await WebAssembly.instantiate(wasmBuffer, go.importObject);
  go.run(instance);
  return { instance, go };
}

async function submitChallenge(baseUrl, token, solution, version) {
  const url = new URL('/.well-known/vercel/security/request-challenge', baseUrl);
  const response = await request(url.href, {
    method: 'POST',
    headers: {
      'x-vercel-challenge-token': token,
      'x-vercel-challenge-solution': solution,
      'x-vercel-challenge-version': version || 'v2'
    }
  });

  if (!response.ok) {
    const cfMitigated = response.headers['cf-mitigated'];
    const cfRay = response.headers['cf-ray'];
    if (cfMitigated) {
      const message = cfRay ? `Ray ID: ${cfRay}` : 'Challenge blocked by Cloudflare';
      const error = new Error(message);
      error.__blocked = true;
      throw error;
    }
    if (response.status === 401 || response.status === 403) {
      const error = new Error('Challenge blocked');
      error.__blocked = true;
      throw error;
    }
    if (response.status === 404) {
      const error = new Error('Challenge not forwarded');
      error.__blocked = true;
      throw error;
    }
    if (response.status >= 700) {
      const error = new Error(String(response.status));
      error.__failed = true;
      throw error;
    }
    throw new Error(response.statusText);
  }
  return response;
}

async function solveVercelChallenge(hostUrl) {
  try {
    const initialResponse = await request(hostUrl);
    const challengeToken = initialResponse.headers['x-vercel-challenge-token'];
    if (!challengeToken) {
      throw new Error('No x-vercel-challenge-token found in response headers');
    }
    
    const wasmUrl = new URL('/.well-known/vercel/security/static/challenge.v2.wasm', hostUrl);
    
    // The global.Solve function is exposed by the WASM module after it runs.
    await loadWasm(wasmUrl.href); 
    
    if (typeof global.Solve !== 'function') {
      throw new Error('Solve function not found. Make sure the WASM module exposes it properly.');
    }

    const solutionJson = await global.Solve(challengeToken);
    const { solution } = JSON.parse(solutionJson);
    
    await submitChallenge(hostUrl, challengeToken, solution, 'v2');
    
    return { success: true, token: challengeToken, solution };
  } catch (error) {
    console.error('Error solving challenge:', error.message);
    if (error.__blocked) {
      console.error('Challenge was blocked');
    } else if (error.__failed) {
      console.error('Challenge failed with status:', error.message);
    }
    throw error;
  }
}

if (require.main === module) {
  const hostUrl = process.argv[2];
  if (!hostUrl) {
    console.error('Usage: node solver.js <host-url>');
    console.error('Example: node solver.js https://example.vercel.app');
    process.exit(1);
  }
  
  solveVercelChallenge(hostUrl)
    .then(result => {
      console.log('Success:', result);
      process.exit(0);
    })
    .catch(error => {
      console.error('Failed:', error.message);
      process.exit(1);
    });
}

module.exports = { solveVercelChallenge, GoRuntime };