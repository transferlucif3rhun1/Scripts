import undetected_chromedriver as uc
import time
import json
import random
import tempfile
import os

class FastCookieHunter:
    def __init__(self, headless=True):
        self.driver = None
        self.headless = headless
        self.user_agents = [
            "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
            "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
            "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
            "Mozilla/5.0 (Windows NT 11.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36",
            "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36"
        ]
        self.current_ua = random.choice(self.user_agents)
        self.temp_profile = tempfile.mkdtemp(prefix="tm_")
        self.setup_driver()
    
    def setup_driver(self):
        try:
            from selenium import webdriver
            from selenium.webdriver.chrome.options import Options
            
            options = Options()
            options.add_argument("--no-sandbox")
            options.add_argument("--disable-dev-shm-usage")
            options.add_argument("--disable-blink-features=AutomationControlled")
            options.add_argument(f"--user-agent={self.current_ua}")
            options.add_argument("--incognito")
            options.add_argument(f"--user-data-dir={self.temp_profile}")
            options.add_argument("--disable-extensions")
            options.add_argument("--disable-gpu")
            options.add_argument("--disable-images")
            options.add_argument("--disable-plugins")
            options.add_argument("--no-first-run")
            options.add_argument("--disable-default-apps")
            options.add_argument("--disable-sync")
            options.add_argument("--disable-translate")
            options.add_argument("--disable-web-security")
            options.add_argument("--disable-features=VizDisplayCompositor")
            options.add_argument("--disable-background-timer-throttling")
            options.add_argument("--disable-backgrounding-occluded-windows")
            options.add_argument("--disable-renderer-backgrounding")
            
            if self.headless:
                options.add_argument("--headless")
            
            self.driver = webdriver.Chrome(options=options)
            self.driver.execute_script("Object.defineProperty(navigator, 'webdriver', {get: () => undefined})")
            
        except Exception:
            options = uc.ChromeOptions()
            options.add_argument("--no-sandbox")
            options.add_argument("--disable-dev-shm-usage")
            options.add_argument("--disable-blink-features=AutomationControlled")
            options.add_argument(f"--user-agent={self.current_ua}")
            options.add_argument("--incognito")
            options.add_argument(f"--user-data-dir={self.temp_profile}")
            options.add_argument("--disable-extensions")
            options.add_argument("--disable-gpu")
            options.add_argument("--disable-images")
            options.add_argument("--disable-plugins")
            options.add_argument("--disable-web-security")
            options.add_argument("--disable-features=VizDisplayCompositor")
            options.add_argument("--no-first-run")
            options.add_argument("--disable-default-apps")
            options.add_argument("--disable-sync")
            options.add_argument("--disable-translate")
            options.add_argument("--disable-background-timer-throttling")
            options.add_argument("--disable-backgrounding-occluded-windows")
            options.add_argument("--disable-renderer-backgrounding")
            options.add_argument("--disable-ipc-flooding-protection")
            options.add_argument("--disable-component-update")
            options.add_argument("--disable-domain-reliability")
            options.add_argument("--disable-client-side-phishing-detection")
            
            if self.headless:
                options.add_argument("--headless=new")
            
            self.driver = uc.Chrome(options=options, use_subprocess=False)
            self.driver.execute_script("Object.defineProperty(navigator, 'webdriver', {get: () => undefined})")
    
    def hunt_cookies(self):
        url = "https://identity.ticketmaster.com/sign-in?disableAutoOptIn=false&integratorId=prd1741.iccp&placementId=mytmlogin&redirectUri=https%3A%2F%2Fwww.ticketmaster.com%2F"
        
        self.driver.get(url)
        
        found_cookies = {}
        start_time = time.time()
        
        while time.time() - start_time < 45:
            cookies = self.driver.get_cookies()
            
            for cookie in cookies:
                name = cookie['name'].lower()
                if 'tmpt' in name and 'tmpt' not in found_cookies:
                    found_cookies['tmpt'] = cookie['value']
                if 'eps_sid' in name and 'eps_sid' not in found_cookies:
                    found_cookies['eps_sid'] = cookie['value']
            
            if len(found_cookies) >= 2:
                break
            
            time.sleep(0.3)
        
        return found_cookies
    
    def close(self):
        try:
            if self.driver:
                self.driver.quit()
            if self.temp_profile and os.path.exists(self.temp_profile):
                import shutil
                shutil.rmtree(self.temp_profile, ignore_errors=True)
        except Exception:
            pass

def hunt():
    hunter = FastCookieHunter(headless=True)
    try:
        return hunter.hunt_cookies()
    finally:
        hunter.close()

def hunt_gui():
    hunter = FastCookieHunter(headless=False)
    try:
        return hunter.hunt_cookies()
    finally:
        hunter.close()

def multiple_hunts(count=3, headless=True):
    results = []
    for i in range(count):
        print(f"Hunt {i+1}/{count}...")
        result = hunt() if headless else hunt_gui()
        results.append(result)
        
        if len(result) >= 2:
            print("Success!")
            break
        elif len(result) >= 1:
            print("Partial, retrying...")
        else:
            print("Failed, retrying...")
        
        if i < count - 1:
            time.sleep(1)
    
    return results

if __name__ == "__main__":
    print("Fast Cookie Hunter")
    print("1. Headless")
    print("2. GUI") 
    print("3. Multiple attempts (headless)")
    print("4. Multiple attempts (GUI)")
    
    choice = input("Choice: ").strip()
    
    if choice == "1":
        result = hunt()
    elif choice == "2":
        result = hunt_gui()
    elif choice == "3":
        attempts = int(input("Attempts: "))
        results = multiple_hunts(attempts, headless=True)
        result = results[-1] if results else {}
    elif choice == "4":
        attempts = int(input("Attempts: "))
        results = multiple_hunts(attempts, headless=False)
        result = results[-1] if results else {}
    else:
        result = hunt()
    
    print(json.dumps(result, indent=2))