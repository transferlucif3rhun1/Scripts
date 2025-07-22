const axios = require('axios');
const tough = require('tough-cookie');
const Cookie = tough.Cookie;

// API endpoints and constants
const CONSTANTS = {
    RECAPTCHA: {
        BASE_URL: 'https://www.google.com/recaptcha/',
        ANCHOR_PATTERN: /(api2|enterprise)\/anchor\?(.*)/,
        POST_DATA_TEMPLATE: 'v={}&reason=q&c={}&k={}&co={}',
        ANCHOR_URL: 'https://www.google.com/recaptcha/enterprise/anchor?ar=1&k=6LdWxZEkAAAAAIHtgtxW_lIfRHlcLWzZMMiwx9E1&co=aHR0cHM6Ly9hdXRoLnRpY2tldG1hc3Rlci5jb206NDQz&hl=fr&v=lqsTZ5beIbCkK4uGEGv9JmUR&size=invisible&cb=c8csckoko34z'
    },
    TICKETMASTER: {
        AUTH_URL: 'https://auth.ticketmaster.com/epsf/gec/v2/auth.ticketmaster.com',
        SITE_KEY: '6LdWxZEkAAAAAIHtgtxW_lIfRHlcLWzZMMiwx9E1',
        MAX_ATTEMPTS: 3
    },
    HTTP: {
        HEADERS: {
            'accept': 'image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8',
            'accept-language': 'fr-FR,fr;q=0.9',
            'priority': 'i',
            'referer': 'https://auth.ticketmaster.com/as/authorization.oauth2?client_id=35a8d3d0b1f1.web.ticketmaster.uk&response_type=code&scope=openid%20profile%20phone%20email%20tm&redirect_uri=https://identity.ticketmaster.co.uk/exchange&visualPresets=tmeu&lang=en-gb&placementId=myAccount&showHeader=true&hideLeftPanel=false&integratorId=tmuk.myAccount&intSiteToken=tm-uk&TMUO=eucentral_NLVJ%2F0V2lU0EJB4130sQhDMag4h4eOPwYmauJsqDtII%3D&deviceId=25tHGw9%2Bgc3HysjMxMjFyMTKxsVKGH26SA825w',
            'sec-ch-ua': '"Chromium";v="130", "Brave";v="130", "Not?A_Brand";v="99"',
            'sec-ch-ua-mobile': '?0',
            'sec-ch-ua-platform': '"Windows"',
            'sec-fetch-dest': 'image',
            'sec-fetch-mode': 'no-cors',
            'sec-fetch-site': 'same-origin',
            'sec-gpc': '1',
            'user-agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36'
        },
        CONTENT_TYPE: {
            FORM: 'application/x-www-form-urlencoded'
        }
    }
};

const DEFAULT_PARAMS = {
    'client_id': '35a8d3d0b1f1.web.ticketmaster.uk',
    'response_type': 'code',
    'scope': 'openid profile phone email tm',
    'redirect_uri': 'https://identity.ticketmaster.co.uk/exchange',
    'visualPresets': 'tmeu',
    'lang': 'en-gb',
    'placementId': 'myAccount',
    'showHeader': 'true',
    'hideLeftPanel': 'false',
    'integratorId': 'tmuk.myAccount',
    'intSiteToken': 'tm-uk',
    'TMUO': 'eucentral_NLVJ/0V2lU0EJB4130sQhDMag4h4eOPwYmauJsqDtII=',
    'deviceId': '25tHGw9+gc3HysjMxMjFyMTKxsVKGH26SA825w'
};

const extractRecaptchaParams = (anchorUrl) => {
    const matches = anchorUrl.match(CONSTANTS.RECAPTCHA.ANCHOR_PATTERN);
    if (!matches) {
        throw new Error("Invalid anchor URL format");
    }
    return {
        apiVersion: matches[1],
        paramsStr: matches[2]
    };
};

const parseQueryParams = (paramsStr) => {
    return paramsStr.split('&').reduce((acc, pair) => {
        const [key, value] = pair.split('=');
        acc[key] = value;
        return acc;
    }, {});
};

const formatPostData = (template, params, token) => {
    return template
        .replace('{}', params.v || '')
        .replace('{}', token)
        .replace('{}', params.k || '')
        .replace('{}', params.co || '');
};

const createAxiosConfig = (headers, proxy = null) => {
    const config = {
        headers,
        maxRedirects: 0,
        validateStatus: status => status >= 200 && status < 400
    };

    if (proxy) {
        const [host, port] = proxy.split(':');
        config.proxy = { host, port };
    }

    return config;
};

const solveRecaptcha = async (anchorUrl) => {
    const { apiVersion, paramsStr } = extractRecaptchaParams(anchorUrl);
    const fullUrlBase = `${CONSTANTS.RECAPTCHA.BASE_URL}${apiVersion}/`;
    
    const response = await axios.get(`${fullUrlBase}anchor?${paramsStr}`);
    const tokenMatch = response.data.match(/"recaptcha-token" value="(.*?)"/);
    if (!tokenMatch) {
        throw new Error("Could not find recaptcha token");
    }

    const queryParams = parseQueryParams(paramsStr);
    const formattedPostData = formatPostData(
        CONSTANTS.RECAPTCHA.POST_DATA_TEMPLATE,
        queryParams,
        tokenMatch[1]
    );

    const reloadResponse = await axios.post(
        `${fullUrlBase}reload?k=${queryParams.k || ''}`,
        formattedPostData,
        { headers: { 'Content-Type': CONSTANTS.HTTP.CONTENT_TYPE.FORM } }
    );

    const answerMatch = reloadResponse.data.match(/"rresp","(.*?)"/);
    if (!answerMatch) {
        throw new Error("Could not find reCAPTCHA answer in response");
    }

    return answerMatch[1];
};

const solveCaptcha = async (proxy = null) => {
    try {
        const recaptchaToken = await solveRecaptcha(CONSTANTS.RECAPTCHA.ANCHOR_URL);
        const params = new URLSearchParams(DEFAULT_PARAMS);
        const url = `${CONSTANTS.TICKETMASTER.AUTH_URL}/${CONSTANTS.TICKETMASTER.SITE_KEY}/Login_Login/${recaptchaToken}?${params.toString()}`;
        
        const response = await axios.get(
            url,
            createAxiosConfig(CONSTANTS.HTTP.HEADERS, proxy)
        );
        
        return { response, headers: response.headers };
    } catch (error) {
        console.error(`Captcha solving failed: ${error.message}`);
        return { error: error.message };
    }
};

const extractTmptCookie = async () => {
    for (let attempt = 0; attempt < CONSTANTS.TICKETMASTER.MAX_ATTEMPTS; attempt++) {
        try {
            const { response, headers, error } = await solveCaptcha();
            
            if (error) {
                throw new Error(error);
            }

            if (response?.status === 200 && headers['set-cookie']) {
                const tmptCookie = headers['set-cookie']
                    .map(cookieStr => Cookie.parse(cookieStr))
                    .find(cookie => cookie?.key === 'tmpt');

                if (tmptCookie) {
                    return {
                        status: 'success',
                        cookie: tmptCookie.value
                    };
                }
            }
        } catch (error) {
            console.error(`Attempt ${attempt + 1}/${CONSTANTS.TICKETMASTER.MAX_ATTEMPTS} failed:`, error.message);
        }
    }
    
    return {
        status: 'error',
        cookie: "unable to get cookie"
    };
};

module.exports = {
    extractTmptCookie,
    solveCaptcha
};