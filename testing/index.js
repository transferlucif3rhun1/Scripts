// =========================
// server.js
// =========================

/*
    Comprehensive Browser Management Server
    - Manages a pool of Puppeteer browser instances.
    - Handles concurrent requests without spawning multiple browsers per request.
    - Monitors browser health and automatically recovers from crashes.
    - Cleans up idle browsers to free resources.
    - Provides API endpoints for health checks, listing browsers, and retrieving cookies.
*/

// =========================
// Import Dependencies
// =========================

import express from "express";
import puppeteerExtra from "puppeteer-extra";
import StealthPlugin from "puppeteer-extra-plugin-stealth";
import UserAgent from "user-agents";
import { query, validationResult } from 'express-validator';
import winston from 'winston';
import os from 'os';
import process from 'process';
import { spawn } from 'child_process';

// =========================
// Initialize Express App
// =========================

const app = express();

// =========================
// Initialize Puppeteer Extra with Stealth Plugin
// =========================

puppeteerExtra.use(StealthPlugin());

// =========================
// Logging Configuration
// =========================

const logger = winston.createLogger({
    level: 'debug', // Set to 'info' or 'error' in production
    format: winston.format.combine(
        winston.format.timestamp({ format: 'YYYY-MM-DD HH:mm:ss.SSS' }),
        winston.format.printf(({ timestamp, level, message }) => `${timestamp} [${level.toUpperCase()}]: ${message}`)
    ),
    transports: [
        new winston.transports.Console(),
        new winston.transports.File({ filename: 'server.log' })
    ],
});

// =========================
// Xvfb Setup for Non-Windows Systems
// =========================

let xvfbProcess = null;
let displayNumber = null;

// Function to start Xvfb
const startXvfb = () => {
    return new Promise((resolve, reject) => {
        const display = 99; // Choose an available display number
        const args = [`:${display}`, "-screen", "0", "1280x1024x24", "-nolisten", "tcp"];
        xvfbProcess = spawn("Xvfb", args, { stdio: 'ignore' });

        xvfbProcess.on('error', (err) => {
            reject(err);
        });

        xvfbProcess.on('exit', (code) => {
            if (code !== 0) {
                reject(new Error(`Xvfb exited with code ${code}`));
            }
        });

        // Wait a short time to ensure Xvfb starts
        setTimeout(() => {
            resolve(display);
        }, 500);
    });
};

// Function to stop Xvfb
const stopXvfb = () => {
    return new Promise((resolve, reject) => {
        if (xvfbProcess) {
            xvfbProcess.on('exit', () => {
                resolve();
            });
            xvfbProcess.on('error', (err) => { // Utilize 'reject'
                reject(err);
            });
            xvfbProcess.kill();
            xvfbProcess = null;
        } else {
            resolve();
        }
    });
};

// Initialize Xvfb if not on Windows
if (os.platform() !== 'win32') {
    startXvfb()
        .then((display) => {
            displayNumber = display;
            logger.info(`Xvfb started on display :${displayNumber}.`);
        })
        .catch((err) => { // 'err' is used here
            logger.error(`Failed to start Xvfb: ${err.message}`);
            process.exit(1);
        });
} else {
    logger.warn("Running on Windows. Xvfb is not required and will not be initialized.");
}

// =========================
// Configuration Constants
// =========================

const MAX_GLOBAL_CONCURRENCY = 5; // Number of concurrent browser slots
const NAVIGATION_TIMEOUT = 60000; // 60 seconds
const NO_COOKIE_FAIL_THRESHOLD = 3; // Max consecutive failures before resetting a slot
const IDLE_TIMEOUT = 10 * 60 * 1000; // 10 minutes

const TICKETMASTER_URL = "https://auth.ticketmaster.com/as/authorization.oauth2?client_id=8bf7204a7e97.web.ticketmaster.us&response_type=code&scope=openid%20profile%20phone%20email%20tm&redirect_uri=https://identity.ticketmaster.com/exchange&visualPresets=tm&lang=en-us&placementId=mytmlogin&hideLeftPanel=false&integratorId=prd1741.iccp&intSiteToken=tm-us";
const TMPT_COOKIE_SUBSTRING = "tmpt";

const BLOCKED_RESOURCE_TYPES = []; // e.g. ["image", "stylesheet", "font"]
const BLOCKED_DOMAINS = [
    // e.g. 'ads.example.com', 'tracker.example.org'
];

// =========================
// Utility Functions
// =========================

// Mutex Class for Synchronization
class Mutex {
    constructor() {
        this._queue = [];
        this._locked = false;
    }

    lock() {
        return new Promise(resolve => {
            if (this._locked) {
                this._queue.push(resolve);
            } else {
                this._locked = true;
                resolve();
            }
        });
    }

    unlock() {
        if (this._queue.length > 0) {
            const nextResolve = this._queue.shift();
            nextResolve();
        } else {
            this._locked = false;
        }
    }
}

// Randomization Helpers

function getRandomUserAgent() {
    const ua = new UserAgent();
    return ua.toString();
}

function getRandomViewport() {
    const commonViewports = [
        { width: 1366, height: 768 },
        { width: 1920, height: 1080 },
        { width: 1536, height: 864 },
        { width: 1280, height: 720 },
        { width: 1440, height: 900 },
        { width: 1600, height: 900 },
    ];
    if (Math.random() > 0.5) {
        return commonViewports[Math.floor(Math.random() * commonViewports.length)];
    } else {
        const width = Math.floor(Math.random() * (1920 - 800 + 1)) + 800;
        const height = Math.floor(Math.random() * (1080 - 600 + 1)) + 600;
        return { width, height };
    }
}

const timezones = [
    "America/New_York",
    "America/Los_Angeles",
    "Europe/London",
    "Europe/Berlin",
    "Asia/Tokyo",
    "Asia/Singapore",
    "Australia/Sydney",
];

function getRandomTimezone() {
    return timezones[Math.floor(Math.random() * timezones.length)];
}

const languages = [
    "en-US",
    "en-GB",
    "de-DE",
    "fr-FR",
    "es-ES",
    "zh-CN",
    "ja-JP",
];

function getRandomLanguage() {
    return languages[Math.floor(Math.random() * languages.length)];
}

function getRandomHardwareConcurrency() {
    const possible = [2, 4, 8, 16];
    return possible[Math.floor(Math.random() * possible.length)];
}

function getRandomDeviceMemory() {
    const possible = [1, 2, 4, 8];
    return possible[Math.floor(Math.random() * possible.length)];
}

function getRandomDoNotTrack() {
    return Math.random() > 0.5 ? '1' : '0';
}

async function moveMouseHumanLike(page, startX, startY, endX, endY) {
    try {
        const steps = 15 + Math.floor(Math.random() * 15); // 15-30 steps
        let curX = startX;
        let curY = startY;
        const xStep = (endX - startX) / steps;
        const yStep = (endY - startY) / steps;

        await page.mouse.move(curX, curY);

        for (let i = 0; i < steps; i++) {
            const jitterX = (Math.random() - 0.5) * 2;
            const jitterY = (Math.random() - 0.5) * 2;

            curX += xStep + jitterX;
            curY += yStep + jitterY;

            await page.mouse.move(curX, curY);
            await new Promise(r => setTimeout(r, 20 + Math.floor(Math.random() * 30)));
        }
    } catch (err) {
        logger.warn(`Error moving mouse human-like: ${err.message}`);
    }
}

// =========================
// Browser Slot Class
// =========================

class Slot {
    constructor(name) {
        this.name = name;
        this.browser = null;
        this.failCount = 0;
        this.queue = [];
        this.lastUsed = Date.now();
        this.mutex = new Mutex();
        this.isShuttingDown = false;
        this.launching = false;
    }

    async launchBrowser(proxy) {
        if (this.launching) {
            logger.debug(`[${this.name}] Launch already in progress.`);
            return; // Prevent multiple launches
        }
        this.launching = true;

        try {
            this.browser = await createBrowser(proxy, this.name);
            this.browser.on('disconnected', () => {
                if (!this.isShuttingDown) {
                    logger.warn(`[${this.name}] Browser disconnected unexpectedly.`);
                    this.handleBrowserCrash(proxy);
                }
            });
            logger.info(`[${this.name}] Browser launched and ready.`);
        } catch (err) {
            logger.error(`[${this.name}] Browser launch failed: ${err.message}\nStack: ${err.stack}`);
            this.browser = null;
            throw err;
        } finally {
            this.launching = false;
        }
    }

    async handleBrowserCrash(proxy) {
        this.failCount += 1;
        if (this.failCount >= NO_COOKIE_FAIL_THRESHOLD) {
            logger.error(`[${this.name}] Max fail count reached. Not attempting to relaunch browser.`);
            return;
        }

        const backoffTime = Math.pow(2, this.failCount) * 1000; // Exponential backoff
        logger.info(`[${this.name}] Attempting to relaunch browser (Attempt ${this.failCount}/${NO_COOKIE_FAIL_THRESHOLD}) in ${backoffTime}ms.`);

        await new Promise(resolve => setTimeout(resolve, backoffTime));

        try {
            await this.launchBrowser(proxy);
            this.failCount = 0;
            this.processQueue();
        } catch (err) {
            logger.error(`[${this.name}] Failed to relaunch browser: ${err.message}\nStack: ${err.stack}`);
        }
    }

    async enqueueRequest(proxy) {
        return new Promise((resolve, reject) => {
            this.queue.push({ proxy, resolve, reject });
            logger.debug(`[${this.name}] Enqueued request. Queue length: ${this.queue.length}`);
            this.processQueue();
        });
    }

    async processQueue() {
        if (this.queue.length === 0) {
            logger.debug(`[${this.name}] No requests to process.`);
            return;
        }
        if (this.launching) {
            logger.debug(`[${this.name}] Currently launching. Waiting for launch to complete.`);
            return; // Wait until current launch completes
        }
        if (!this.browser) {
            const { proxy } = this.queue[0];
            try {
                logger.debug(`[${this.name}] No active browser. Launching new browser.`);
                await this.launchBrowser(proxy);
            } catch (err) {
                logger.error(`[${this.name}] Failed to launch browser for queued requests: ${err.message}\nStack: ${err.stack}`);
                // Fail all queued requests
                while (this.queue.length > 0) {
                    const { reject } = this.queue.shift();
                    reject(new Error('Browser launch failed.'));
                }
                return;
            }
        }

        const { proxy, resolve, reject } = this.queue.shift();
        this.lastUsed = Date.now();
        logger.debug(`[${this.name}] Dequeued request. Remaining queue length: ${this.queue.length}`);

        await this.mutex.lock();

        try {
            const result = await this.handleRequest(proxy);
            resolve(result);
        } catch (err) {
            reject(err);
        } finally {
            this.mutex.unlock();
            this.processQueue();
        }
    }

    async handleRequest(proxy) {
        if (!this.browser) throw new Error('Browser not initialized.');

        const page = await this.browser.newPage();

        // Capture browser console errors
        page.on('console', msg => {
            if (msg.type() === 'error') {
                logger.error(`[${this.name}] Browser Console Error: ${msg.text()}`);
            } else if (msg.type() === 'warn') {
                logger.warn(`[${this.name}] Browser Console warn: ${msg.text()}`);
            } else {
                logger.debug(`[${this.name}] Browser Console ${msg.type()}: ${msg.text()}`);
            }
        });

        // Handle proxy authentication if needed
        const parsedProxy = parseProxyString(proxy);
        if (parsedProxy && parsedProxy.username && parsedProxy.password) {
            try {
                await page.authenticate({
                    username: parsedProxy.username,
                    password: parsedProxy.password,
                });
                logger.debug(`[${this.name}] Proxy authentication successful.`);
            } catch (err) {
                logger.warn(`[${this.name}] Proxy authentication failed: ${err.message}`);
            }
        }

        try {
            await setupPage(page, this.name, logger);
            await moveMouseHumanLike(page, 10, 10, 300, 200);

            let tmptFound = false;

            // Listen for responses to detect 'tmpt' cookie
            const onResponse = async (response) => {
                const headers = response.headers();
                const setCookie = headers['set-cookie'];
                if (setCookie && Array.isArray(setCookie)) {
                    for (const cookie of setCookie) {
                        if (cookie.toLowerCase().includes(TMPT_COOKIE_SUBSTRING)) {
                            tmptFound = true;
                            logger.info(`[${this.name}] Detected '${TMPT_COOKIE_SUBSTRING}' cookie via response.`);
                            await page.close().catch(e => logger.warn(`[${this.name}] Error closing page: ${e.message}`));
                            break;
                        }
                    }
                }
            };
            page.on('response', onResponse);

            // Navigate to the URL
            await page.goto(TICKETMASTER_URL, {
                waitUntil: "networkidle2",
                timeout: NAVIGATION_TIMEOUT,
            });

            // Wait for a specific selector
            await page.waitForSelector('.sc-oTNly', { timeout: NAVIGATION_TIMEOUT })
                .catch(err => {
                    logger.warn(`[${this.name}] Timeout waiting for selector '.sc-oTNly': ${err.message}`);
                });

            // Remove response listener
            page.off('response', onResponse);

            // Retrieve cookies
            const cookiesArr = await page.cookies();
            const cookieObj = {};
            cookiesArr.forEach((c) => {
                cookieObj[c.name] = c.value;
            });
            logger.debug(`[${this.name}] Retrieved cookies: ${JSON.stringify(cookieObj)}`);

            // Check if 'tmpt' cookie is found
            if (cookiesArr.some(c => c.name.toLowerCase() === TMPT_COOKIE_SUBSTRING)) {
                tmptFound = true;
                logger.info(`[${this.name}] 'tmpt' cookie found in cookies.`);
            }

            // Clear cookies & storage
            await page.deleteCookie(...cookiesArr).catch(e => logger.warn(`[${this.name}] Error deleting cookies: ${e.message}`));
            await page.evaluate(() => {
                window.localStorage.clear();
                window.sessionStorage.clear();
            }).catch(e => logger.warn(`[${this.name}] Error clearing storage: ${e.message}`));

            // Close page if still open
            if (!page.isClosed()) {
                await page.close().catch(e => logger.warn(`[${this.name}] Error closing page: ${e.message}`));
                logger.debug(`[${this.name}] Page closed.`);
            }

            const timeSpent = ((Date.now() - this.lastUsed) / 1000).toFixed(2);
            logger.info(`[${this.name}] Completed navigation in ${timeSpent} seconds.`);

            if (tmptFound) {
                this.failCount = 0;
                return {
                    status: "success",
                    cookies: cookieObj,
                    time_taken: parseFloat(timeSpent),
                };
            } else {
                this.failCount += 1;
                logger.warn(`[${this.name}] '${TMPT_COOKIE_SUBSTRING}' cookie not found. Fail count: ${this.failCount}`);

                if (this.failCount >= NO_COOKIE_FAIL_THRESHOLD) {
                    logger.warn(`[${this.name}] Fail threshold reached. Resetting browser.`);
                    await this.resetBrowser();
                }

                return {
                    status: "retry",
                    cookies: cookieObj,
                    time_taken: parseFloat(timeSpent),
                };
            }
        } catch (err) {
            logger.error(`[${this.name}] Error during navigation: ${err.message}\nStack: ${err.stack}`);
            await page.close().catch(e => logger.warn(`[${this.name}] Error closing page after failure: ${e.message}`));
            throw err;
        }
    }

    async resetBrowser() {
        if (this.browser) {
            try {
                await this.browser.close();
                logger.info(`[${this.name}] Browser closed for reset.`);
            } catch (err) {
                logger.warn(`[${this.name}] Error closing browser during reset: ${err.message}`);
            } finally {
                this.browser = null;
                this.failCount = 0;
            }
        }
    }

    async shutdown() {
        this.isShuttingDown = true;
        if (this.browser) {
            try {
                await this.browser.close();
                logger.info(`[${this.name}] Browser closed during shutdown.`);
            } catch (err) {
                logger.warn(`[${this.name}] Error closing browser during shutdown: ${err.message}`);
            } finally {
                this.browser = null;
            }
        }
    }
}

// =========================
// Browser Pool Management
// =========================

class BrowserPool {
    constructor(maxConcurrency) {
        this.slots = {};
        this.maxConcurrency = maxConcurrency;

        for (let i = 1; i <= this.maxConcurrency; i++) {
            const slotName = `browser${i}`;
            this.slots[slotName] = new Slot(slotName);
            logger.debug(`Initialized slot: ${slotName}`);
        }
    }

    async enqueueRequest(proxy) {
        // Select the slot with the least number of queued requests
        let selectedSlot = null;
        let minQueue = Infinity;

        for (const slot of Object.values(this.slots)) {
            if (slot.queue.length < minQueue) {
                minQueue = slot.queue.length;
                selectedSlot = slot;
            }
        }

        if (selectedSlot) {
            logger.debug(`Assigning request to slot: ${selectedSlot.name}`);
            return selectedSlot.enqueueRequest(proxy);
        } else {
            throw new Error('No available browser slots.');
        }
    }

    getBrowserNames() {
        return Object.keys(this.slots);
    }

    async shutdownAll() {
        logger.info("Shutting down all browser slots.");
        const shutdownPromises = Object.values(this.slots).map(slot => slot.shutdown());
        await Promise.all(shutdownPromises);
    }
}

const browserPool = new BrowserPool(MAX_GLOBAL_CONCURRENCY);

// =========================
// Helper Functions
// =========================

// Browser Creation Function
async function createBrowser(proxy, slotName) {
    // Clone base launchOptions
    const baseLaunchOptions = {
        headless: false, // Set to 'false' for headful mode (visible browser)
        args: [
            "--no-sandbox",
            "--disable-setuid-sandbox",
            "--disable-gpu",
            "--disable-dev-shm-usage",
            "--disable-extensions",
            "--disable-popup-blocking",
            "--disable-notifications",
            "--disable-background-networking",
            "--disable-sync",
            "--no-first-run",
            "--incognito",
            "--disable-features=TranslateUI",
            "--hide-scrollbars",
            "--disable-background-timer-throttling",
            "--disable-renderer-backgrounding",
            "--disable-site-isolation-trials",
            "--disable-breakpad",
            "--disable-client-side-phishing-detection",
            "--disable-hang-monitor",
            "--disable-infobars",
            "--disable-logging",
            "--disable-default-apps",
            "--disable-software-rasterizer",
            "--mute-audio",
        ],
        env: {
            ...process.env,
        },
        defaultViewport: getRandomViewport(),
        ignoreHTTPSErrors: true, // To handle any SSL issues gracefully
    };

    if (os.platform() !== 'win32' && displayNumber !== null) {
        baseLaunchOptions.env.DISPLAY = `:${displayNumber}`; // Set DISPLAY for Puppeteer on Unix-based systems
    }

    // Random user-agent from user-agents library
    const randomUA = getRandomUserAgent();

    // Add user-agent to launch arguments
    baseLaunchOptions.args.push(`--user-agent=${randomUA}`);

    // Parse proxy
    const parsedProxy = parseProxyString(proxy);
    if (parsedProxy && parsedProxy.host && parsedProxy.port) {
        const { protocol, host, port } = parsedProxy;
        const proxyArg = `--proxy-server=${protocol}://${host}:${port}`;
        baseLaunchOptions.args.push(proxyArg);
        logger.debug(`[${slotName}] Using proxy: ${proxyArg}`);
    } else if (proxy) {
        logger.warn(`[${slotName}] Proxy string provided but could not parse: ${proxy}`);
    }

    try {
        logger.debug(`[${slotName}] Launching browser with UA="${randomUA}".`);
        const browser = await puppeteerExtra.launch(baseLaunchOptions);
        logger.info(`[${slotName}] Browser launched successfully.`);
        return browser;
    } catch (err) {
        logger.error(`[${slotName}] Failed to launch browser: ${err.message}\nStack: ${err.stack}`);
        throw err;
    }
}

// Page Setup Function
async function setupPage(page, slotName, logger) {
    try {
        // 1. Random time zone
        const randomTz = getRandomTimezone();
        await page.emulateTimezone(randomTz).catch(err => {
            logger.warn(`[${slotName}] Could not emulate timezone (${randomTz}): ${err.message}`);
        });

        // 2. Random Accept-Language
        const randomLang = getRandomLanguage();
        await page.setExtraHTTPHeaders({
            "Accept-Language": randomLang,
        });

        // 3. Additional stealth-like overrides
        await page.evaluateOnNewDocument(({
            randomHC,
            randomMem,
            randomDNT
        }) => {
            Object.defineProperty(navigator, 'webdriver', {
                get: () => false,
            });
            Object.defineProperty(navigator, 'hardwareConcurrency', {
                get: () => randomHC,
            });
            Object.defineProperty(navigator, 'deviceMemory', {
                get: () => randomMem,
            });
            Object.defineProperty(navigator, 'doNotTrack', {
                get: () => randomDNT,
            });
        }, {
            randomHC: getRandomHardwareConcurrency(),
            randomMem: getRandomDeviceMemory(),
            randomDNT: getRandomDoNotTrack(),
        });

        // 4. Network conditions (HTTP/2 if supported automatically)
        const client = await page.target().createCDPSession();
        await client.send('Network.enable');
        await client.send('Network.emulateNetworkConditions', {
            offline: false,
            latency: Math.floor(Math.random() * 200) + 50, // 50-250ms
            downloadThroughput: (Math.floor(Math.random() * 2000) + 500) * 1024, 
            uploadThroughput: (Math.floor(Math.random() * 500) + 100) * 1024,
        });

        // 5. Request Interception
        await page.setRequestInterception(true);
        page.on("request", (req) => {
            const resourceType = req.resourceType();
            const url = req.url();

            // Block resources if needed
            if (BLOCKED_RESOURCE_TYPES.includes(resourceType)) {
                const isBlockedDomain = BLOCKED_DOMAINS.some(domain => url.includes(domain));
                if (isBlockedDomain || BLOCKED_DOMAINS.length === 0) {
                    logger.debug(`[${slotName}] Blocking ${resourceType} request to ${url}`);
                    return req.abort();
                }
            }

            // Add random header
            const newHeaders = Object.assign({}, req.headers(), {
                'X-Random-Header': Math.random().toString(36).substring(2),
            });
            req.continue({ headers: newHeaders });
        });

        logger.debug(`[${slotName}] Setup page with randomization: TZ=${randomTz}, Lang=${randomLang}`);
    } catch (err) {
        logger.error(`[${slotName}] Error setting up page: ${err.message}\nStack: ${err.stack}`);
        throw err;
    }
}

// Proxy Parsing Helper
function parseProxyString(proxyString = "") {
    if (!proxyString.trim()) return null;

    let protocol = "http";
    let cleanedString = proxyString.trim();

    if (cleanedString.startsWith("http://")) {
        protocol = "http";
        cleanedString = cleanedString.replace("http://", "");
    } else if (cleanedString.startsWith("https://")) {
        protocol = "https";
        cleanedString = cleanedString.replace("https://", "");
    } else if (cleanedString.startsWith("socks4://")) {
        protocol = "socks4";
        cleanedString = cleanedString.replace("socks4://", "");
    } else if (cleanedString.startsWith("socks5://")) {
        protocol = "socks5";
        cleanedString = cleanedString.replace("socks5://", "");
    }

    let username = null;
    let password = null;
    let hostPort = cleanedString;

    if (cleanedString.includes("@")) {
        const [userPass, hostPortPart] = cleanedString.split("@");
        hostPort = hostPortPart || "";
        const [user, pass] = userPass.split(":");
        username = user || null;
        password = pass || null;
    }

    const [host, port] = hostPort.split(":");
    return { protocol, host, port, username, password };
}

// =========================
// API Endpoints
// =========================

// Health check
app.get("/health", (req, res) => {
    res.json({ status: "ok" });
});

// List browsers
app.get("/browserlist", (req, res) => {
    const browserNames = browserPool.getBrowserNames();
    res.json({ browsers: browserNames });
});

// Get cookies
app.get("/cookies", [
    query('browser').optional().trim(),
    query('proxy').optional().trim(),
], async (req, res) => {
    // Validate 'browser' parameter if provided
    if (req.query.browser) {
        const browserName = req.query.browser;
        if (!browserPool.getBrowserNames().includes(browserName)) {
            logger.warn(`Invalid browser name received: ${browserName}`);
            return res.status(422).json({ errors: [{ msg: 'Invalid browser name' }] });
        }
    }

    const errors = validationResult(req);
    if (!errors.isEmpty()) {
        logger.warn(`Validation errors in /cookies request: ${JSON.stringify(errors.array())}`);
        return res.status(422).json({ errors: errors.array() });
    }

    const { browser, proxy } = req.query;

    try {
        let result;
        if (browser) {
            // Enqueue request to the specified slot
            const slot = browserPool.slots[browser];
            logger.debug(`Received /cookies request for ${browser} with proxy=${proxy || 'None'}.`);
            result = await slot.enqueueRequest(proxy);
        } else {
            // Enqueue request to any available slot
            logger.debug(`Received /cookies request without specific browser. Assigning to any available slot.`);
            result = await browserPool.enqueueRequest(proxy);
        }
        logger.debug(`Sending response for /cookies request: ${JSON.stringify(result)}`);
        res.json(result);
    } catch (err) { // 'err' is used here
        logger.error(`Error handling /cookies request: ${err.message}\nStack: ${err.stack}`);
        res.status(500).json({ status: "error", cookies: {}, time_taken: 0 });
    }
});

// =========================
// Graceful Shutdown Handling
// =========================

const shutdown = async () => {
    logger.info("Shutting down server...");

    // Shutdown all browser slots
    await browserPool.shutdownAll();

    // Stop Xvfb if it's running
    if (os.platform() !== 'win32') {
        try {
            await stopXvfb();
            logger.info("Xvfb stopped.");
        } catch (err) { // 'err' is used here
            logger.error(`Error stopping Xvfb: ${err.message}`);
        }
    }

    process.exit(0);
};

process.on("SIGINT", shutdown);
process.on("SIGTERM", shutdown);

// =========================
// Idle Browser Cleanup
// =========================

setInterval(() => {
    const now = Date.now();
    for (const slot of Object.values(browserPool.slots)) {
        if (slot.browser && (now - slot.lastUsed) > IDLE_TIMEOUT) {
            logger.info(`[${slot.name}] Browser idle for over ${IDLE_TIMEOUT / 60000} minutes. Closing browser.`);
            slot.resetBrowser().catch(e => logger.warn(`[${slot.name}] Error closing browser due to inactivity: ${e.message}`));
            slot.lastUsed = now;
            logger.info(`[${slot.name}] Browser closed due to inactivity.`);
        }
    }
}, 60 * 1000); // Every minute

// =========================
// Start the Server
// =========================

const PORT = 8000;
app.listen(PORT, () => {
    logger.info(`Node server running on http://0.0.0.0:${PORT}`);
});
