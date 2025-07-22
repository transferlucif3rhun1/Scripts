const http = require("http");
const cluster = require("cluster");
const os = require("os");

// Function to generate a random integer between min (inclusive) and max (exclusive)
function getRandomInt(min, max) {
  return Math.floor(Math.random() * (max - min)) + min;
}

// Function to generate an array of random integers
function generateRandomArray(length, min, max) {
  return Array.from({ length }, () => getRandomInt(min, max));
}

// Function to generate the client features
function generateClientFeatures() {
  return {
    userAgent:
      "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:" +
      getRandomInt(50, 150) +
      ".0) Gecko/20100101 Firefox/" +
      getRandomInt(50, 150) +
      ".0",
    plugins: [
      { name: "PDF Viewer", filename: "internal-pdf-viewer", length: 2 },
      { name: "Chrome PDF Viewer", filename: "internal-pdf-viewer", length: 2 },
      {
        name: "Chromium PDF Viewer",
        filename: "internal-pdf-viewer",
        length: 2,
      },
      {
        name: "Microsoft Edge PDF Viewer",
        filename: "internal-pdf-viewer",
        length: 2,
      },
      {
        name: "WebKit built-in PDF",
        filename: "internal-pdf-viewer",
        length: 2,
      },
    ],
    localTime: new Date().toLocaleTimeString(),
    timezoneOffset: new Date().getTimezoneOffset(),
    permissionStatus: { state: "prompt" },
    webdriver: Math.random() > 0.5,
    batteryInfo: null,
    features: [
      null,
      `${getRandomInt(1200, 2000)}|${getRandomInt(800, 1200)}|${getRandomInt(
        10,
        40
      )}`,
      false,
      true,
    ],
  };
}

// Function to generate random data
function generateRandomData(clientFeatures) {
  return {
    b0: getRandomInt(8000, 11000),
    b1: generateRandomArray(4, 50, 200),
    b2: getRandomInt(1, 10),
    b3: generateRandomArray(getRandomInt(0, 5), 0, 100),
    b4: getRandomInt(0, 20),
    b5: getRandomInt(0, 5),
    b6: clientFeatures.userAgent,
    b7: clientFeatures.plugins,
    b8: clientFeatures.localTime,
    b9: clientFeatures.timezoneOffset,
    b10: clientFeatures.permissionStatus,
    b11: false,
    b12: clientFeatures.batteryInfo,
    b13: clientFeatures.features,
  };
}

// Worker threads to handle concurrent requests
if (cluster.isMaster) {
  const numCPUs = os.cpus().length;

  console.log(`Master ${process.pid} is running`);

  // Fork workers
  for (let i = 0; i < numCPUs; i++) {
    cluster.fork();
  }

  cluster.on("exit", (worker, code, signal) => {
    console.log(`Worker ${worker.process.pid} died`);
  });
} else {
  // Workers can share any TCP connection. Here, it's an HTTP server.
  const server = http.createServer((req, res) => {
    const clientFeatures = generateClientFeatures();
    const randomData = generateRandomData(clientFeatures);

    if (req.url === "/riskcontent") {
      // Log and respond with the first JSON object in one line
      const logData = JSON.stringify(randomData);
      res.writeHead(200, { "Content-Type": "text/plain" });
      res.end(logData);
    } else {
      res.writeHead(404);
      res.end("Not Found");
    }
  });

  server.listen(3000, () => {
    console.log(`Worker ${process.pid} started server on port 3000`);
  });
}
