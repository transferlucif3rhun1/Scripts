import requests
import re
from typing import Dict, Optional, Tuple, Union
from urllib.parse import parse_qs, urlparse
from http.cookies import SimpleCookie
from dataclasses import dataclass
from functools import wraps
import logging

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)

# Constants and Configuration
CONSTANTS = {
    "RECAPTCHA": {
        "BASE_URL": "https://www.google.com/recaptcha/",
        "ANCHOR_URL": "https://www.google.com/recaptcha/enterprise/anchor?ar=1&k=6LdWxZEkAAAAAIHtgtxW_lIfRHlcLWzZMMiwx9E1&co=aHR0cHM6Ly9hdXRoLnRpY2tldG1hc3Rlci5jb206NDQz&hl=fr&v=lqsTZ5beIbCkK4uGEGv9JmUR&size=invisible&cb=c8csckoko34z",
        "POST_DATA_TEMPLATE": "v={}&reason=q&c={}&k={}&co={}",
        "TOKEN_PATTERN": r'"recaptcha-token" value="(.*?)"',
        "RESPONSE_PATTERN": r'"rresp","(.*?)"',
        "URL_PATTERN": r"(api2|enterprise)/anchor\?(.*)"
    },
    "TICKETMASTER": {
        "AUTH_URL": "https://auth.ticketmaster.com/epsf/gec/v2/auth.ticketmaster.com",
        "SITE_KEY": "6LdWxZEkAAAAAIHtgtxW_lIfRHlcLWzZMMiwx9E1",
        "MAX_ATTEMPTS": 3
    },
    "HTTP": {
        "HEADERS": {
            "accept": "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8",
            "accept-language": "fr-FR,fr;q=0.9",
            "priority": "i",
            "referer": "https://auth.ticketmaster.com/as/authorization.oauth2?client_id=35a8d3d0b1f1.web.ticketmaster.uk&response_type=code&scope=openid%20profile%20phone%20email%20tm&redirect_uri=https://identity.ticketmaster.co.uk/exchange&visualPresets=tmeu&lang=en-gb&placementId=myAccount&showHeader=true&hideLeftPanel=false&integratorId=tmuk.myAccount&intSiteToken=tm-uk&TMUO=eucentral_NLVJ%2F0V2lU0EJB4130sQhDMag4h4eOPwYmauJsqDtII%3D&deviceId=25tHGw9%2Bgc3HysjMxMjFyMTKxsVKGH26SA825w",
            "sec-ch-ua": '"Chromium";v="130", "Brave";v="130", "Not?A_Brand";v="99"',
            "sec-ch-ua-mobile": "?0",
            "sec-ch-ua-platform": '"Windows"',
            "sec-fetch-dest": "image",
            "sec-fetch-mode": "no-cors",
            "sec-fetch-site": "same-origin",
            "sec-gpc": "1",
            "user-agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36",
            "Content-Type": "application/x-www-form-urlencoded"
        }
    },
    "DEFAULT_PARAMS": {
        "client_id": "35a8d3d0b1f1.web.ticketmaster.uk",
        "response_type": "code",
        "scope": "openid profile phone email tm",
        "redirect_uri": "https://identity.ticketmaster.co.uk/exchange",
        "visualPresets": "tmeu",
        "lang": "en-gb",
        "placementId": "myAccount",
        "showHeader": "true",
        "hideLeftPanel": "false",
        "integratorId": "tmuk.myAccount",
        "intSiteToken": "tm-uk",
        "TMUO": "eucentral_NLVJ/0V2lU0EJB4130sQhDMag4h4eOPwYmauJsqDtII=",
        "deviceId": "25tHGw9+gc3HysjMxMjFyMTKxsVKGH26SA825w"
    }
}

@dataclass
class RecaptchaParams:
    """Data class for storing parsed recaptcha parameters"""
    api_version: str
    params_str: str

def error_handler(func):
    """Decorator for handling request errors"""
    @wraps(func)
    def wrapper(*args, **kwargs):
        try:
            return func(*args, **kwargs)
        except requests.RequestException as e:
            logger.error(f"Request error in {func.__name__}: {str(e)}")
            return {"error": str(e)}
        except Exception as e:
            logger.error(f"Unexpected error in {func.__name__}: {str(e)}")
            return {"error": "An unexpected error occurred"}
    return wrapper

def parse_recaptcha_url(anchor_url: str) -> RecaptchaParams:
    """Parse recaptcha URL and extract necessary parameters"""
    matches = re.search(CONSTANTS["RECAPTCHA"]["URL_PATTERN"], anchor_url)
    if not matches:
        raise ValueError("Invalid anchor URL format")
    
    return RecaptchaParams(
        api_version=matches.group(1),
        params_str=matches.group(2)
    )

def parse_query_params(params_str: str) -> Dict[str, str]:
    """Parse query string into dictionary of parameters"""
    return {
        key: value[0] for key, value in 
        parse_qs(params_str).items()
    }

def create_session(proxy: Optional[str] = None) -> requests.Session:
    """Create and configure requests session"""
    session = requests.Session()
    session.headers.update(CONSTANTS["HTTP"]["HEADERS"])
    
    if proxy:
        session.proxies = {
            'http': proxy,
            'https': proxy
        }
    
    return session

@error_handler
def solve_recaptcha(anchor_url: str, session: requests.Session) -> str:
    """Solve recaptcha challenge and return token"""
    recaptcha_params = parse_recaptcha_url(anchor_url)
    url_base = f"{CONSTANTS['RECAPTCHA']['BASE_URL']}{recaptcha_params.api_version}/"
    
    # Get initial token
    response = session.get(f"{url_base}anchor?{recaptcha_params.params_str}")
    token_match = re.search(CONSTANTS["RECAPTCHA"]["TOKEN_PATTERN"], response.text)
    if not token_match:
        raise ValueError("Could not find recaptcha token")
    
    # Parse parameters
    query_params = parse_query_params(recaptcha_params.params_str)
    
    # Format post data
    post_data = CONSTANTS["RECAPTCHA"]["POST_DATA_TEMPLATE"].format(
        query_params.get('v', ''),
        token_match.group(1),
        query_params.get('k', ''),
        query_params.get('co', '')
    )
    
    # Get final token
    reload_response = session.post(
        f"{url_base}reload?k={query_params.get('k', '')}",
        data=post_data
    )
    
    answer_match = re.search(CONSTANTS["RECAPTCHA"]["RESPONSE_PATTERN"], reload_response.text)
    if not answer_match:
        raise ValueError("Could not find reCAPTCHA answer")
        
    return answer_match.group(1)

@error_handler
def solve_ticketmaster_captcha(session: requests.Session) -> Dict[str, Union[requests.Response, Dict]]:
    """Solve Ticketmaster's captcha challenge"""
    recaptcha_token = solve_recaptcha(CONSTANTS["RECAPTCHA"]["ANCHOR_URL"], session)
    
    url = (
        f"{CONSTANTS['TICKETMASTER']['AUTH_URL']}/"
        f"{CONSTANTS['TICKETMASTER']['SITE_KEY']}/Login_Login/{recaptcha_token}"
    )
    
    response = session.get(url, params=CONSTANTS["DEFAULT_PARAMS"])
    return {"response": response, "cookies": response.cookies}

def extract_tmpt_cookie() -> Dict[str, str]:
    """Extract TMPT cookie from Ticketmaster response"""
    for attempt in range(CONSTANTS["TICKETMASTER"]["MAX_ATTEMPTS"]):
        try:
            session = create_session()
            result = solve_ticketmaster_captcha(session)
            
            if "error" in result:
                raise ValueError(result["error"])
                
            if result["response"].status_code == 200:
                tmpt_cookie = result["cookies"].get("tmpt")
                if tmpt_cookie:
                    return {
                        "status": "success",
                        "cookie": tmpt_cookie
                    }
                    
            logger.warning(f"Attempt {attempt + 1}: No TMPT cookie found")
            
        except Exception as e:
            logger.error(f"Attempt {attempt + 1} failed: {str(e)}")
            
    return {
        "status": "error",
        "cookie": "unable to get cookie"
    }

if __name__ == "__main__":
    result = extract_tmpt_cookie()
    print(result)