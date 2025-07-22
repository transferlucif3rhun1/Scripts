const express = require('express');
const cluster = require('cluster');
const os = require('os');
const axios = require('axios');
const HttpsProxyAgent = require('https-proxy-agent');
const url_parser = require('url');
const bodyParser = require('body-parser');
const winston = require('winston');
const NodeCache = require('node-cache');
require('dotenv').config();

const numCPUs = os.cpus().length;
const port = process.env.PORT || 3003;
const appCache = new NodeCache({ stdTTL: 60, checkperiod: 120 });

const logger = winston.createLogger({
  level: 'info',
  format: winston.format.combine(
    winston.format.timestamp(),
    winston.format.json()
  ),
  transports: [
    new winston.transports.Console(),
    new winston.transports.File({ filename: 'combined.log' })
  ]
});

function findIndexOfOccurrence(str, searchValue, occurrence_num) {
  let count = 0;
  let index = -1;

  for (let i = 0; i < str.length; i++) {
    if (str.substring(i, i + searchValue.length) === searchValue) {
      count++;
      if (count === occurrence_num) {
        index = i;
        break;
      }
    }
  }

  return index;
}

function findClosestLeftIndex(str, index) {
  for (let i = index - 1; i >= 0; i--) {
    if (str[i] === '=') {
      return i;
    }
  }
  return -1; // Return -1 if no '=' is found
}

function getFunctionName29(script, index) {
  const locationIndex = findIndexOfOccurrence(script, "215464049", index);
  if (locationIndex === -1) {
    return '';
  }
  const sub = script.substring(locationIndex, locationIndex + 300);
  const sub2 = sub.substring(sub.indexOf("window"), sub.indexOf(";});") + 3);
  return sub2;
}

function reese84_payload(url, get_payload, proxy, data, response_to_send, customUserAgent) {
  try {
    let jsdom = `require('jsdom-global')('<html><head><script src="${url}"></script></head><body><pre style="word-wrap: break-word; white-space: pre-wrap;"></pre></body></html>',{url:'${url}',referer:'${url}',customUserAgent:'${customUserAgent}'});const gl = require('gl');global.window.WebGLRenderingContext = gl.WebGLRenderingContext;navigator.driver=false;`;
    let script = jsdom.concat(data);

    const patternFirst = /\)\)\);},(.*?)\);}\)];case 0/g;
    const matchesFirst = script.match(patternFirst);
    if (!matchesFirst || matchesFirst.length === 0) {
      throw new Error("Pattern first match not found");
    }

    const patternSecond = /'te']=function\((.*?)\)/;
    const matchesSecond = script.match(patternSecond);
    if (!matchesSecond || matchesSecond.length < 2) {
      throw new Error("Pattern second match not found");
    }

    if (get_payload) {
      script = script.replace(matchesFirst[0], ')));},0);})];case 0').replace(`function(${matchesSecond[1]}){`, `function(${matchesSecond[1]}){response_to_send.send(${matchesSecond[1]});die();`);
    } else {
      script = script.replace(matchesFirst[0], ')));},0);})];case 0').replace(`function(${matchesSecond[1]}){`, `function(${matchesSecond[1]}){getReese84Token(url,proxy,${matchesSecond[1]},response_to_send,customUserAgent);die();`);
    }

    while (script.includes('||(undefined?')) {
      const index = script.indexOf('||(undefined?');
      const sub_str = script.substring(index - 60);
      const sub_str2 = sub_str.substring(sub_str.indexOf("try") + 4, sub_str.indexOf("}catch"));
      const string_to_replace = sub_str2.substring(sub_str2.indexOf("=") + 1);
      script = script.replace(string_to_replace, "\"probably\"");
    }

    let sub_ = script.substring(script.indexOf('if(true)'));
    const sub_2 = sub_.substring(0, sub_.indexOf("var", sub_.indexOf("var") + 1));
    const script_to_remove = sub_2.split(";")[2];
    script = script.replace(script_to_remove + ";", "");

    const index2 = script.indexOf('=20;');
    if (index2 > 0) {
      const sub__ = script.substring(index2 - 90);
      const patternThird = /var (.*?)=20;/g;
      const matchesThird = sub__.match(patternThird);
      if (matchesThird && matchesThird.length > 0) {
        const extractedString = matchesThird[0];
        const string_to_replace2 = extractedString.substring(extractedString.indexOf("=") + 1, extractedString.indexOf(";var"));
        script = script.replace(string_to_replace2, "[]");
      }
    }

    const index3 = findIndexOfOccurrence(script, "(-21)", 4);
    if (index3 !== -1) {
      const sub_3 = script.substring(index3 - 150, index3 + 150);
      const indexEqual = findClosestLeftIndex(sub_3, sub_3.indexOf("(-21)"));
      const sub_3_2 = sub_3.substring(indexEqual + 1);
      const string_to_replace = sub_3_2.substring(0, sub_3_2.indexOf(";"));
      script = script.replaceAll(string_to_replace, '\"\\" () { [native code] }\\"\"');
    }

    const index4 = script.indexOf("===null");
    if (index4 !== -1) {
      const sub4 = script.substring(index4 - 100, index4);
      const indexFunction = sub4.indexOf("function");
      const sub_4_1 = sub4.substring(indexFunction);
      const string_to_replace = sub_4_1.substring(0, sub_4_1.indexOf("{") + 1);

      const regex_4 = /function\s+[^\(]*\(([^\)]*)\)/;
      const matches_4 = string_to_replace.match(regex_4);

      if (matches_4 && matches_4[1]) {
        const params = matches_4[1].split(',');
        const firstParam = params[0].trim();
        const string_need_replace = string_to_replace + firstParam + `="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAQAAAAECAYAAACp8Z5+AAAAAXNSR0IArs4c6QAAAA9JREFUGFdjZEADjKQLAAAA7gAFLaYDxAAAAABJRU5ErkJggg==";`;
        script = script.replace(string_to_replace, string_need_replace);
      }
    }

    script = script.replace(getFunctionName29(script, 2), "29");
    script = script.replace(getFunctionName29(script, 3), "29");
    script = script.replace(getFunctionName29(script, 4), "29");
    script = script.replace(getFunctionName29(script, 5), "29");

    eval(script);
  } catch (error) {
    logger.error(`Error in reese84_payload: ${error.message}`);
    response_to_send.status(500).send(`Error processing script: ${error.message}`);
  }
}

async function fetchScript(url) {
  const cachedData = appCache.get(url);
  if (cachedData) {
    logger.info(`Cache hit for URL: ${url}`);
    return cachedData;
  } else {
    logger.info(`Cache miss for URL: ${url}, fetching from network`);
    try {
      const response = await axios.get(url, { timeout: 10000 });
      appCache.set(url, response.data);
      return response.data;
    } catch (error) {
      logger.error(`Error fetching script from URL: ${url} - ${error.message}`);
      throw error;
    }
  }
}

async function getReese84Token(url, proxy, payload, res, customUserAgent) {
  const parsedUrl = new url_parser.URL(url);
  const domain = parsedUrl.searchParams.get('d');
  const origin = `https://${domain}`;
  const headers = {
    "sec-ch-ua": `"Not/A)Brand";v="8", "Chromium";v="126", "Brave";v="126"`,
    "accept": "application/json; charset=utf-8",
    "content-type": "text/plain; charset=utf-8",
    "dnt": "1",
    "sec-ch-ua-mobile": "?0",
    "user-agent": customUserAgent,
    "sec-ch-ua-platform": `"macOS"`,
    "sec-gpc": "1",
    "accept-language": "en-GB,en;q=0.5",
    "origin": origin,
    "sec-fetch-site": "same-site",
    "sec-fetch-mode": "cors",
    "sec-fetch-dest": "empty",
    "referer": origin,
    "accept-encoding": "gzip, deflate, br, zstd",
    "priority": "u=1, i",
  };

  const [proxyHost, proxyPort, proxyUsername, proxyPassword] = proxy.split(':');
  const proxyAgent = new HttpsProxyAgent({
    host: proxyHost,
    port: proxyPort,
    auth: `${proxyUsername}:${proxyPassword}`
  });

  try {
    const response = await axios.post(url, payload, {
      headers: headers,
      httpsAgent: proxyAgent,
      timeout: 10000,
      retry: 3
    });
    res.send(response.data);
  } catch (error) {
    logger.error(`Error in getReese84Token: ${error.message}`);
    if (error.response) {
      logger.error('Response Error:', error.response.data);
      res.status(error.response.status).send(error.response.data);
    } else if (error.request) {
      logger.error('Request Error:', error.request);
      res.status(500).send('No response received from server');
    } else {
      logger.error('Error:', error.message);
      res.status(500).send(error.message);
    }
  }
}

async function genReese84script(url, get_payload, proxy, res, customUserAgent) {
  try {
    const data = await fetchScript(url);
    reese84_payload(url, get_payload, proxy, data, res, customUserAgent);
  } catch (error) {
    logger.error(`Error in genReese84script: ${error.message}`);
    res.status(404).send("Proxy error or timeout");
  }
}

if (cluster.isMaster) {
  logger.info(`Master ${process.pid} is running`);

  // Fork workers
  for (let i = 0; i < numCPUs; i++) {
    cluster.fork();
  }

  cluster.on('exit', (worker, code, signal) => {
    logger.warn(`Worker ${worker.process.pid} died, starting a new worker`);
    cluster.fork(); // Replace the dead worker
  });
} else {
  const app = express();

  app.use(bodyParser.json()); // Parse JSON bodies
  app.use(bodyParser.urlencoded({ extended: true })); // Parse URL-encoded bodies

  app.post('/reese84', async (req, res) => {
    const { url: reese84_url, proxy, payload, userAgent } = req.body;
    if (reese84_url && proxy) {
      const customUserAgent = userAgent || process.env.USER_AGENT || "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36";
      genReese84script(reese84_url, payload || false, proxy, res, customUserAgent);
    } else {
      res.status(400).json({ 'error': 'Missing reese84 URL or proxy' });
    }
  });

  const server = app.listen(port, () => {
    logger.info(`Worker ${process.pid} is listening on port ${port}`);
  });

  // Graceful shutdown
  process.on('SIGTERM', () => {
    logger.info('SIGTERM signal received: closing HTTP server');
    server.close(() => {
      logger.info('HTTP server closed');
    });
  });

  process.on('SIGINT', () => {
    logger.info('SIGINT signal received: closing HTTP server');
    server.close(() => {
      logger.info('HTTP server closed');
    });
  });
}
