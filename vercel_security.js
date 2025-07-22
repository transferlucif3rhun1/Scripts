const fetch = require('node-fetch');
const crypto = require('crypto');
const { TextEncoder, TextDecoder } = require('util');

// Polyfill for browser-like environment
global.window = global;
global.crypto = {
    getRandomValues: (array) => crypto.randomFillSync(array)
};
global.performance = {
    now: () => Date.now()
};
global.TextEncoder = TextEncoder;
global.TextDecoder = TextDecoder;
global.self = global;

// Custom Go WASM runtime implementation
class Go {
    constructor() {
        this.argv = [];
        this.env = {};
        this.exit = (code) => {
            if (code !== 0) console.error(`WASI exit code: ${code}`);
        };
        this.mem = new DataView(new ArrayBuffer(0));
        this._values = [];
        this._ids = new Map();
        this._idPool = 0;
    }

    importObject() {
        return {
            gojs: {
                'runtime.ticks': () => Date.now(),
                'runtime.timeOrigin': () => 0,
                'runtime.walltime': () => Math.floor(Date.now() / 1000),
                'runtime.nanotime': () => BigInt(Date.now()) * 1000000n,
                'syscall/js.valueNew': () => {
                    const id = this._idPool++;
                    this._values[id] = {};
                    return id;
                },
                'syscall/js.valueGet': (ref, propPtr) => {
                    const prop = this.readString(propPtr);
                    const value = this._values[ref][prop];
                    return value || 0;
                },
                'syscall/js.valueSet': (ref, propPtr, valueRef) => {
                    const prop = this.readString(propPtr);
                    this._values[ref][prop] = valueRef;
                },
                'syscall/js.valueCall': (ref, methodPtr, argsPtr, argsLen) => {
                    const method = this.readString(methodPtr);
                    if (method === 'toString') {
                        return this.allocateString(String(this._values[ref]));
                    }
                    return 0;
                },
                'syscall/js.stringVal': (strPtr) => {
                    const str = this.readString(strPtr);
                    const id = this._idPool++;
                    this._values[id] = str;
                    return id;
                },
                'syscall/js.valuePrepareString': (strPtr) => {
                    return this.allocateString(this.readString(strPtr));
                },
                'syscall/js.valueLoadString': (ref, buf, max) => {
                    const str = String(this._values[ref] || '');
                    const encoder = new TextEncoder();
                    const encoded = encoder.encode(str);
                    const memView = new Uint8Array(this.mem.buffer);
                    const length = Math.min(encoded.length, max);
                    for (let i = 0; i < length; i++) {
                        memView[buf + i] = encoded[i];
                    }
                    return length;
                },
                'syscall/js.finalizeRef': (ref) => {
                    delete this._values[ref];
                },
                'syscall/js.valueIsNull': (ref) => ref === 0 ? 1 : 0,
                'syscall/js.valueIsUndefined': (ref) => ref === 0 ? 1 : 0,
            },
            wasi_snapshot_preview1: {
                fd_write: () => 0,
                fd_close: () => 0,
                fd_seek: () => 0,
                fd_fdstat_get: () => 0,
                proc_exit: this.exit,
                random_get: (buf, bufLen) => {
                    const randomBytes = crypto.randomBytes(bufLen);
                    const memView = new Uint8Array(this.mem.buffer);
                    for (let i = 0; i < bufLen; i++) {
                        memView[buf + i] = randomBytes[i];
                    }
                    return 0;
                },
                clock_time_get: (id, precision, time) => {
                    const memView = new BigUint64Array(this.mem.buffer);
                    memView[time / 8] = BigInt(Date.now()) * 1000000n;
                    return 0;
                },
                environ_sizes_get: () => 0,
                environ_get: () => 0,
                args_sizes_get: () => 0,
                args_get: () => 0,
            },
            env: {
                memory: new WebAssembly.Memory({ initial: 256 }),
                table: new WebAssembly.Table({ initial: 0, element: 'anyfunc' }),
            }
        };
    }

    readString(ptr) {
        const memView = new Uint8Array(this.mem.buffer);
        let str = '';
        let i = 0;
        while (memView[ptr + i] !== 0) {
            str += String.fromCharCode(memView[ptr + i]);
            i++;
        }
        return str;
    }

    allocateString(str) {
        const encoder = new TextEncoder();
        const encoded = encoder.encode(str);
        const ptr = this.malloc(encoded.length + 1);
        const memView = new Uint8Array(this.mem.buffer);
        for (let i = 0; i < encoded.length; i++) {
            memView[ptr + i] = encoded[i];
        }
        memView[ptr + encoded.length] = 0;
        return ptr;
    }

    malloc(size) {
        // Simple memory allocator
        if (!this.heapBase) this.heapBase = 1024;
        const ptr = this.heapBase;
        this.heapBase += size;
        
        // Grow memory if needed
        const neededPages = Math.ceil((ptr + size) / (64 * 1024));
        const currentPages = this.mem.buffer.byteLength / (64 * 1024);
        if (neededPages > currentPages) {
            const growBy = neededPages - currentPages;
            this.importObject().env.memory.grow(growBy);
        }
        
        return ptr;
    }

    async run(instance) {
        this.mem = new DataView(instance.exports.memory.buffer);
        try {
            if (instance.exports._start) {
                await instance.exports._start();
            } else if (instance.exports.__wasm_call_ctors) {
                await instance.exports.__wasm_call_ctors();
            }
        } catch (err) {
            if (!err.message.includes('exit code')) {
                throw err;
            }
        }
    }
}

class VercelChallengeSolver {
    constructor() {
        this.wasmUrl = 'https://packdraw.com/.well-known/vercel/security/static/challenge.v2.wasm';
        this.verifyUrl = 'https://packdraw.com/.well-known/vercel/security/request-challenge';
        this.token = null;
        this.solution = null;
        this.cookie = null;
    }

    async loadWasm() {
        const response = await fetch(this.wasmUrl);
        if (!response.ok) throw new Error('Failed to load WASM challenge');
        return response.arrayBuffer();
    }

    async solveChallenge(token) {
        this.token = token;
        const wasmBuffer = await this.loadWasm();
        const go = new Go();
        const importObject = go.importObject();
        
        // Instantiate WASM module
        const { instance } = await WebAssembly.instantiate(wasmBuffer, importObject);
        
        // Run Go runtime
        await go.run(instance);
        
        // Access the Solve function from global scope
        if (!global.Solve || typeof global.Solve !== 'function') {
            // Try to find the solve function in exports as a fallback
            const solve = instance.exports._solve || 
                          instance.exports.solve || 
                          instance.exports.__solve;
            if (!solve) {
                throw new Error('Solve function not found in WASM exports');
            }
            
            // If we have a pointer-based solve function
            const tokenPtr = go.allocateString(this.token);
            const solutionPtr = solve(tokenPtr);
            this.solution = go.readString(solutionPtr);
        } else {
            // Use the global Solve function
            this.solution = global.Solve(this.token);
        }
        
        return this.solution;
    }

    async verifySolution() {
        if (!this.solution) throw new Error('No solution to verify');
        
        const response = await fetch(this.verifyUrl, {
            method: 'POST',
            headers: {
                'x-vercel-challenge-token': this.token,
                'x-vercel-challenge-solution': this.solution,
                'x-vercel-challenge-version': '2',
                'Content-Type': 'application/json'
            }
        });
        
        if (!response.ok) {
            const error = await response.text();
            throw new Error(`Verification failed: ${response.status} - ${error}`);
        }
        
        // Extract cookie from response headers
        const cookies = response.headers.raw()['set-cookie'];
        if (!cookies) throw new Error('No cookies in response');
        
        const cookieMatch = cookies.join(';').match(/_vcrcs=([^;]+)/);
        if (!cookieMatch) throw new Error('_vcrcs cookie not found in response');
        
        this.cookie = cookieMatch[1];
        return this.cookie;
    }

    async execute(token) {
        try {
            await this.solveChallenge(token);
            await this.verifySolution();
            return {
                success: true,
                token: this.token,
                solution: this.solution,
                cookie: this.cookie
            };
        } catch (error) {
            return {
                success: false,
                error: error.message,
                token: this.token
            };
        }
    }
}

// Usage
(async () => {
    // Token format: 2.<timestamp>.<version>.<payload>
    const token = "2.1749295434.60.NWU0MjY1MGU3Mjc4YTI2ZTA1MmQ1YjMzNTg1YzNmYWE7NDY4ZDA5OWE7ZTVhNTQ2ZDk2MDMyNDU4YzZkM2NjMTVkODIzNDRmODI3MTA1OWYzODszO3fHDwMBvfOwN/2jkV+Y2XhF30DMXdHst/3sNYMWg77KIbIqvs2kKd8tOKZV.1091614af24bfc1ea2008ec6deb18aea";
    
    const solver = new VercelChallengeSolver();
    const result = await solver.execute(token);
    
    if (result.success) {
        console.log('Challenge solved successfully!');
        console.log('Solution:', result.solution);
        console.log('Cookie:', result.cookie);
        
        // In a browser environment, you would set the cookie:
        // document.cookie = `_vcrcs=${result.cookie}; path=/; secure; samesite=lax`;
    } else {
        console.error('Challenge failed:', result.error);
    }
})();