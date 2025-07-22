const fs = require('fs');
const express = require('express');
const cluster = require('cluster');
const os = require('os');
const numCPUs = os.cpus().length;
const port = 3000;
const axios = require('axios');
const httpsProxyAgent = require('https-proxy-agent');
const url_parser = require('url');
const bodyParser = require('body-parser');

const file_paths = {
  "https://www.ticketmaster.de/tmol-dstlxhr.js?d=www.ticketmaster.de":"tm_de.js",
  "https://epsf.ticketmaster.com/eps-d?d=www.ticketmaster.com":"tm_us.js",
  "https://epsf.ticketmaster.co.uk/eps-d?d=www.ticketmaster.co.uk":"tm_uk.js",
  "https://www.ticketmaster.es/tmol-dstlxhr.js?d=www.ticketmaster.es":"tm_es.js",
  "https://epsf.ticketmaster.com.au/eps-d?d=www.ticketmaster.com.au":"tm_au.js",
  "https://www.ticketmaster.be/tmol-dstlxhr.js?d=www.ticketmaster.be":"tm_be.js",
  "https://epsf.ticketmaster.ca/eps-d?d=www.ticketmaster.ca":"tm_ca.js",
  "https://www.ticketmaster.cz/tmol-dstlxhr.js?d=www.ticketmaster.cz":"tm_cz.js",
  "https://www.ticketmaster.dk/tmol-dstlxhr.js?d=www.ticketmaster.dk":"tm_dk.js",
  "https://epsf.ticketmaster.ie/eps-d?d=www.ticketmaster.ie":"tm_ie.js",
  "https://www.ticketmaster.nl/tmol-dstlxhr.js?d=www.ticketmaster.nl":"tm_nl.js",
  "https://epsf.ticketmaster.co.nz/eps-d?d=www.ticketmaster.co.nz":"tm_nz.js",
  "https://www.ticketmaster.no/tmol-dstlxhr.js?d=www.ticketmaster.no":"tm_no.js",
  "https://epsf.ticketmaster.sg/eps-d?d=ticketmaster.sg":"tm_sg.js",
  "https://www.ticketmaster.at/tmol-dstlxhr.js?d=www.ticketmaster.at":"tm_at.js",
  "https://epsf.ticketmaster.ch/eps-d?d=www.ticketmaster.ch":"tm_ch.js",
  "https://epsf.ticketmaster.se/eps-d?d=www.ticketmaster.se":"tm_se.js",
  "https://www.ticketmaster.pl/tmol-dstlxhr.js?d=www.ticketmaster.pl":"tm_pl.js",
  "https://balance.vanillagift.com/tis-them-Ported-I-amis-and-formany-way-thee-not-?d=balance.vanillagift.com":"vanillagiftbalance.js",
  "https://www.vanillagift.com/y-Vpon-his-Guiltiply-did-more-a-Wing-Onell-Vizar?d=www.vanillagift.com":"vanillagiftlogin.js",
  "https://www.pokemoncenter.com/kie-Yes-him-To-the-To-mocking-and-do-mise-I-prom?d=www.pokemoncenter.com":"pokemoncenter.js",
  "https://www.7now.com/g-Like-Secution-What-mock-Tis-non-Here-your-sure?d=www.7now.com":"7now.js",
  "https://premier.hkticketing.com/Indus-Spire-not-Accursedometience-make-numbe-if-?d=premier.hkticketing.com":"premerhkticketing.js",
  "https://www.smythstoys.com/mbit-And-Dirers-him-Face-and-sure-such-Parry-qui?d=www.smythstoys.com":"smythstoys_uk.js",
  "https://api.formula1.com/6657193977244c13?d=account.formula1.com":"f1tv.js",
  "https://www.coursehero.com/Ifainesse-What-mine-Alasterd-the-How-I-haile-Lad?d=www.coursehero.com":"coursehero.js",
}

function findIndexOfOccurrence(str, searchValue,occurence_num) {
  let count = 0;
  let index = -1;

  for (let i = 0; i < str.length; i++) {
      if (str.substring(i, i + searchValue.length) === searchValue) {
          count++;
          if (count === occurence_num) {
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

function reese84_payload(url,get_payload,proxy,data,response_to_send,customUserAgent)
{
  var jsdom = "require('jsdom-global')('<html><head><script src=\"" + url + "\"></script></head><body><pre style=\"word-wrap: break-word; white-space: pre-wrap;\"></pre></body></html>',{url:'" + url + "',referer:'" + url + "',customUserAgent:'" + customUserAgent + "'});const gl = require('gl');global.window.WebGLRenderingContext = gl.WebGLRenderingContext;navigator.driver=false;";
  var script = jsdom.concat(data);

  const patternFirst = /\)\)\);},(.*?)\);}\)];case 0/g;
  const matchesFirst = script.match(patternFirst)[0];
  const patternSecond = /'te']=function\((.*?)\)/;
  const matchesSecond = script.match(patternSecond)[1];

  if(get_payload)
  {
    script = script.replace(matchesFirst,')));},0);})];case 0').replace('function(' + matchesSecond + '){', 'function(' + matchesSecond + '){response_to_send.send(' + matchesSecond +');die();');
  } else {
    script = script.replace(matchesFirst,')));},0);})];case 0').replace('function(' + matchesSecond + '){', 'function(' + matchesSecond + '){getReese84Token(url,proxy,' + matchesSecond + ',response_to_send);die();');
  }

  while(script.includes('||(undefined?'))
  {
    var index = script.indexOf('||(undefined?');
    var sub_str = script.substring(index - 60);
    var sub_str2 = sub_str.substring(sub_str.indexOf("try") + 4,sub_str.indexOf("}catch"));
    var string_to_replace = sub_str2.substring(sub_str2.indexOf("=") + 1);
    script = script.replace(string_to_replace,"\"probably\"");
  }

  var sub_ = script.substring(script.indexOf('if(true)'));
  var sub_2 = sub_.substring(0,sub_.indexOf("var", sub_.indexOf("var") + 1));
  var script_to_remove = sub_2.split(";")[2];
  script = script.replace(script_to_remove + ";","");

  var index2 = script.indexOf('=20;');
  if(index2 > 0)
  {
    var sub__ = script.substring(index2 - 90);
    const patternThird = /var (.*?)=20;/g;
    const matchesThird = sub__.match(patternThird)
    if(matchesThird)
    {
      const extractedString  = sub__.match(patternThird)[0];
      const string_to_replace2 = extractedString .substring(extractedString .indexOf("=") + 1,extractedString .indexOf(";var"))
      script = script.replace(string_to_replace2,"[]");
    }
  }

  var index3 = findIndexOfOccurrence(script,"(-21)",4);
  var sub_3 = script.substring(index3-150,index3+150);
  var indexEqual = findClosestLeftIndex(sub_3,sub_3.indexOf("(-21)"));
  var sub_3_2 = sub_3.substring(indexEqual + 1);
  var string_to_replace = sub_3_2.substring(0,sub_3_2.indexOf(";"));
  script = script.replaceAll(string_to_replace,'\"\\" () { [native code] }\\"\"');

  var index4 = script.indexOf("===null");
  var sub4 = script.substring(index4-100,index4);
  var indexFunction = sub4.indexOf("function");
  var sub_4_1 = sub4.substring(indexFunction);
  string_to_replace = sub_4_1.substring(0,sub_4_1.indexOf("{") + 1);

  const regex_4 = /function\s+[^\(]*\(([^\)]*)\)/;
  const matches_4 = string_to_replace.match(regex_4);

  if (matches_4 && matches_4[1]) {
      const params = matches_4[1].split(',');
      const firstParam = params[0].trim();
      var string_need_replace = string_to_replace + firstParam + "=\"data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAQAAAAECAYAAACp8Z5+AAAAAXNSR0IArs4c6QAAAA9JREFUGFdjZEADjKQLAAAA7gAFLaYDxAAAAABJRU5ErkJggg==\";";
      script = script.replace(string_to_replace,string_need_replace);
  }

  eval(script);
}

function getReese84Token(url,proxy,payload,res,customUserAgent)
{
    const parsedUrl = new url_parser.URL(url);
    const domain = parsedUrl.searchParams.get('d');
    const origin = "https://" + domain;
    console.log(origin);
  const headers = {
    "Content-Type": "text/plain; charset=utf-8",
    Origin: origin,
    Referer: origin + "/",
    "sec-ch-ua": "\"Google Chrome\";v=\"107\", \"Chromium\";v=\"107\", \"Not=A?Brand\";v=\"24\"",
    "sec-ch-ua-mobile": "?0",
    "sec-ch-ua-platform": "\"Windows\"",
    "Sec-Fetch-Dest": "empty",
    "Sec-Fetch-Mode": "cors",
    "Sec-Fetch-Site": "same-site",
    "upgrade-insecure-requests": "1",
    "user-agent": customUserAgent
  }

  const [proxyHost, proxyPort, proxyUsername, proxyPassword] = proxy.split(':');
  const proxyAgent = new httpsProxyAgent({
    host: proxyHost,
    port: proxyPort,
    auth: `${proxyUsername}:${proxyPassword}`
  });

  axios.post(url, payload, {
    headers: headers,
    httpsAgent: proxyAgent
  })
  .then(response => {
      // Handle the response as needed
      res.send(response.data);
  })
  .catch(error => {
      // Handle errors
      console.error('Error:', error);
  });
}

function genReese84script(url,get_payload,proxy,res,customUserAgent)
{
    if(url in file_paths)
    {
        try{
            const filePath = 'reese84-files/' + file_paths[url];
            var data = fs.readFileSync(filePath, 'utf8');
            reese84_payload(url,get_payload,proxy,data,res,customUserAgent);
        } catch (error){
            res.status(400).json({'error':error.message});
        }
    } else {
        res.status(400).json({'error':'No file matched the provided url'});
    }
}

if (cluster.isMaster) {
  console.log(`Master ${process.pid} is running`);

  // Fork workers
  for (let i = 0; i < numCPUs; i++) {
    cluster.fork();
  }

  cluster.on('exit', (worker, code, signal) => {
    console.log(`Worker ${worker.process.pid} died`);
    cluster.fork(); // Replace the dead worker
  });
} else {
  const app = express();

  app.use(bodyParser.json()); // Parse JSON bodies
  app.use(bodyParser.urlencoded({ extended: true })); // Parse URL-encoded bodies
  app.post('/reese84',async (req, res) => {
    const body = req.body;
    const reese84_url = body.url;
    const proxy = body.proxy;
    const get_payload = body.payload ? body.payload : false;
    if(reese84_url && proxy)
    {
      let customUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/114.0.0.0 Safari/537.36";
      if(body.userAgent)
      {
        customUserAgent = body.userAgent;
      }
      genReese84script(reese84_url,get_payload,proxy,res,customUserAgent);
    } else {
        res.status(400).json({'error':'Missing reese84 URL or proxy'});
    }
  });

  app.listen(port, () => {
    console.log(`Worker ${process.pid} is listening on port ${port}`);
  });
}
