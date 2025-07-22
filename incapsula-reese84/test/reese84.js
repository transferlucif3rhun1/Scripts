const express = require('express');
const axios = require('axios');
const { HttpProxyAgent, HttpsProxyAgent } = require('http-proxy-agent');
const cluster = require('cluster');
const numCPUs = require('os').cpus().length;

if (cluster.isMaster) {
    console.log(`Master ${process.pid} is running`);
    for (let i = 0; i < numCPUs; i++) {
        cluster.fork();
    }
    cluster.on('exit', (worker) => {
        console.log(`Worker ${worker.process.pid} died`);
        cluster.fork(); // Replace the dead worker
    });
} else {
    const app = express();
    app.use(express.json());

    app.post('/process', async (req, res) => {
        try {
            const { ua, proxy, url } = req.body;
            const proxyConfig = proxy ? {
                httpAgent: new HttpProxyAgent(proxy),
                httpsAgent: new HttpsProxyAgent(proxy)
            } : {};

            // Custom headers for both GET and POST requests
            const customHeaders = {
                'sec-ch-ua': '"Not A(Brand";v="99", "Brave";v="121", "Chromium";v="121"',
                'accept': 'application/json; charset=utf-8',
                'dnt': '1',
                'sec-ch-ua-mobile': '?0',
                'user-agent': ua,
                'sec-ch-ua-platform': '"macOS"',
                'sec-gpc': '1',
                'accept-language': 'en-GB,en;q=0.7',
                'origin': 'https://www.ticketmaster.com',
                'sec-fetch-site': 'same-site',
                'sec-fetch-mode': 'cors',
                'sec-fetch-dest': 'empty',
                'referer': 'https://www.ticketmaster.com/',
                'accept-encoding': 'gzip, deflate, br'
            };

            // First GET request
            const getResponse = await axios.get(url, {
                ...proxyConfig,
                headers: customHeaders
            });

            // Second request to incapsula without proxies
            const incapsulaResponse = await axios.post('https://incapsula.justhyped.dev/reese84', {
                userAgent: ua,
                script: getResponse.data
            });

            if (!incapsulaResponse.data.payload) {
                return res.status(400).json({ error: "No payload received from processing endpoint." });
            }

            // Final POST request using the payload
            const postData = incapsulaResponse.data.payload;
            const postResponse = await axios.post(url, postData, {
                ...proxyConfig,
                headers: {
                    ...customHeaders,
                    'Content-Type': 'text/plain; charset=utf-8',
                    'Content-Length': Buffer.byteLength(postData).toString()
                }
            });

            res.json(postResponse.data);
        } catch (error) {
            res.status(500).json({ error: error.message });
        }
    });

    const PORT = 3000;
    app.listen(PORT, () => console.log(`Worker ${process.pid}: Listening on port ${PORT}`));
}
