const axios = require('axios')
const path = require('path')
const fs = require('fs')
const { HttpsProxyAgent } = require('https-proxy-agent');

async function tmLogin ()
{
    let agent = new HttpsProxyAgent(`http://viridian007:FctXDTqOR43hyn7y@proxy.packetstream.io:31112`)
    let repeatedString = '-'.repeat(4096);
    try {
        let response = await axios.get(`https://identity.ticketmaster.com/sign-in?integratorId=prd1741.iccp&placementId=mytmlogin&redirectUri=https%3A%2F%2Fwww.ticketmaster.com%2F`, {
            httpsAgent:agent,
            httpAgent:agent, 
            headers: {
                'Accept': 'text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7',
                'Accept-Encoding': 'gzip, deflate, br',
                'Accept-Language': 'en-US,en;q=0.9',
                'Cache-Control': 'no-cache',
                'Connection': 'keep-alive',
                'Host': 'identity.ticketmaster.com',
                'Pragma': 'no-cache',
                'Sec-Ch-Ua': '"Not_A Brand";v="8", "Chromium";v="120", "Google Chrome";v="120"',
                'Sec-Ch-Ua-Mobile': '?0',
                'Sec-Ch-Ua-Platform': '"macOS"',
                'Sec-Fetch-Dest': 'document',
                'Sec-Fetch-Mode': 'navigate',
                'Sec-Fetch-Site': 'none',
                'Sec-Fetch-User': '?1',
                'Upgrade-Insecure-Requests': '1',
                'User-Agent': 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36',
                'cookie':`reese84=${repeatedString}`, 
                    },
            
          });
          console.log(response.status) 
    } catch (error) {
        console.log('error')
    }
 
}

async function runSequentially() {
    for (let i = 0; i < 100; i++) {
         tmLogin();
    }
}

runSequentially();

function processProxyFile(fileName) {
    const filePath = path.join(process.cwd(), 'proxies', fileName);
    const content = fs.readFileSync(filePath, 'utf8');
    const lines = content.split('\n').filter(line => line.trim() !== '');
    lines.shift(); // Remove the header row
  
    return lines.map(line => line.replace(/\r$/, '')); // Remove carriage return and return the line
  }
  function parseProxyString(proxyString) {
    const [host, port, username, password] = proxyString.split(':');
    if (!host || !port || isNaN(port) || Number(port) <= 0 || Number(port) >= 65536) {
      throw new Error(`Invalid proxy string: ${proxyString}`);
    }
    return { host, port, username, password: password.replace(/\r/g, "") };
  }