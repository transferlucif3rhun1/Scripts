import time
import uuid
import logging
from typing import Optional, Dict
from threading import Lock

import anyio
import undetected_chromedriver as uc
from selenium.common.exceptions import WebDriverException, SessionNotCreatedException

from fastapi import FastAPI, Query, HTTPException
from fastapi.responses import JSONResponse
from contextlib import asynccontextmanager
from urllib.parse import urlparse

LOG = logging.getLogger(__name__)
logging.basicConfig(level=logging.INFO)

# ------------------ CONFIG ------------------
MAX_BROWSERS = 3                # total concurrency across entire app
NO_COOKIE_FAIL_THRESHOLD = 3    # 3 consecutive "no tmpt" => remove & generate new ID
CONCURRENCY_LIMITER = anyio.Semaphore(MAX_BROWSERS)

TICKETMASTER_URL = (
    "https://auth.ticketmaster.com/as/authorization.oauth2?client_id=8bf7204a7e97.web.ticketmaster.us"
    "&response_type=code&scope=openid%20profile%20phone%20email%20tm"
    "&redirect_uri=https://identity.ticketmaster.com/exchange"
    "&visualPresets=tm&lang=en-us&placementId=mytmlogin&hideLeftPanel=false"
    "&integratorId=prd1741.iccp&intSiteToken=tm-us&TMUO=east_eG8EAWun/+rbmgJ00JS5To6kD96420xuYyoN60dDZzc="
    "&deviceId=6E0YCheAYTE3MzU0MTY5MDMwNjAUe21D4MaLAA"
)
TMPT_COOKIE_SUBSTRING = "tmpt"

def create_driver_with_proxy(proxy_str: Optional[str] = None) -> uc.Chrome:
    """
    Attempt to create a headless incognito uc.Chrome(...), with optional proxy.
    If mismatch => if uc.install() available => try once fallback. Else => error but do not crash the entire script.
    """
    chrome_opts = uc.ChromeOptions()
    chrome_opts.add_argument("--headless=new")
    chrome_opts.add_argument("--disable-gpu")
    chrome_opts.add_argument("--no-sandbox")
    chrome_opts.add_argument("--disable-dev-shm-usage")
    chrome_opts.add_argument("--disable-extensions")
    chrome_opts.add_argument("--disable-popup-blocking")
    chrome_opts.add_argument("--disable-notifications")
    chrome_opts.add_argument("--disable-background-networking")
    chrome_opts.add_argument("--disable-background-timer-throttling")
    chrome_opts.add_argument("--disable-breakpad")
    chrome_opts.add_argument("--disable-crash-reporter")
    chrome_opts.add_argument("--disable-sync")
    chrome_opts.add_argument("--metrics-recording-only")
    chrome_opts.add_argument("--no-first-run")
    chrome_opts.add_argument("--password-store=basic")
    chrome_opts.add_argument("--use-mock-keychain")
    chrome_opts.add_argument("--incognito")
    chrome_opts.add_argument("--disable-ipc-flooding-protection")
    chrome_opts.add_argument("--disable-client-side-phishing-detection")
    chrome_opts.add_argument("--disable-hang-monitor")
    chrome_opts.add_argument("--disable-features=TranslateUI")
    chrome_opts.add_argument("--disable-infobars")
    chrome_opts.add_argument("--disable-logging")
    chrome_opts.add_argument("--disable-default-apps")
    chrome_opts.add_argument("--hide-scrollbars")
    chrome_opts.add_argument("--disable-software-rasterizer")
    # If proxy is given, parse & add
    if proxy_str:
        parsed = urlparse(proxy_str)
        # e.g. "http://user:pass@host:port"
        scheme = parsed.scheme or "http"
        netloc = parsed.netloc or parsed.path  # handle weird user:pass@host:port
        if netloc:
            full_proxy = f"{scheme}://{netloc}"
            chrome_opts.add_argument(f"--proxy-server={full_proxy}")
            LOG.info(f"[Driver] Using proxy => {full_proxy}")

    try:
        driver = uc.Chrome(options=chrome_opts)
        driver.set_page_load_timeout(30)
        return driver
    except SessionNotCreatedException as e1:
        if hasattr(uc, "install") and callable(uc.install):
            LOG.warning(f"[Driver] mismatch => trying uc.install() fallback => {e1}")
            try:
                uc.install()
                driver = uc.Chrome(options=chrome_opts)
                driver.set_page_load_timeout(30)
                return driver
            except Exception as e2:
                LOG.error(f"[Driver] fallback failed => {e2}")
                raise RuntimeError(
                    "Chrome version mismatch => fallback also failed => update local Chrome or undetected-chromedriver.\n"
                    + str(e2)
                )
        else:
            raise RuntimeError(
                "Chrome version mismatch => no uc.install() => update local Chrome or undetected-chromedriver.\n"
                + str(e1)
            )
    except Exception as ex:
        raise RuntimeError(f"Driver creation error => {ex}")

class BrowserSlot:
    """
    Each slot => has a 'current_id' that we change if driver is removed.
    driver=None => create on-demand.
    fail_count => 3 => remove driver => also generate new 'current_id'
    lock => ensures multiple requests for same ID are queued
    """
    def __init__(self, slot_name: str):
        self.slot_name = slot_name
        self.current_id = str(uuid.uuid4())  # stable ID until driver is replaced
        self.driver: Optional[uc.Chrome] = None
        self.fail_count = 0
        self.lock = Lock()

    def close_driver_and_new_id(self):
        """Close current driver and also assign a new current_id for next usage."""
        if self.driver:
            try:
                self.driver.quit()
            except Exception as e:
                LOG.warning(f"[{self.slot_name}] close error => {e}")
        self.driver = None
        self.fail_count = 0
        new_id = str(uuid.uuid4())
        LOG.warning(f"[{self.slot_name}] => removed driver, new id={new_id}")
        self.current_id = new_id

class SlotsManager:
    """
    3 stable slots => browser1..browser3 => each has 'current_id'
    - If user calls /cookies?id=some-id => find which slot matches
    - If 3 consecutive misses => close driver + generate new id => return 'retry' with new id
    - If crash => also remove driver => new id => return 'error' with new id
    """
    def __init__(self):
        self.slots: Dict[str, BrowserSlot] = {}

    def initialize_slots(self):
        for i in range(1, MAX_BROWSERS + 1):
            sname = f"browser{i}"
            self.slots[sname] = BrowserSlot(sname)
        LOG.info(f"[Slots] Created {MAX_BROWSERS} => each has driver=None initially.")

    def get_browserlist(self) -> dict:
        return {k: v.current_id for k, v in self.slots.items()}

    def find_slot_by_id(self, requested_id: str) -> Optional[BrowserSlot]:
        """Which slot has current_id==requested_id?"""
        for s_obj in self.slots.values():
            if s_obj.current_id == requested_id:
                return s_obj
        return None

    def do_cookies_nav(self, slot_obj: BrowserSlot, proxy_str: Optional[str]) -> dict:
        with slot_obj.lock:
            # If driver is None => create now
            if slot_obj.driver is None:
                try:
                    LOG.info(f"[{slot_obj.slot_name}] creating driver => id={slot_obj.current_id}")
                    slot_obj.driver = create_driver_with_proxy(proxy_str)
                    slot_obj.fail_count = 0
                except Exception as ex:
                    LOG.error(f"[{slot_obj.slot_name}] create driver failed => {ex}")
                    # driver not created => we still keep the same id or replace?
                    # We'll keep the same ID for now => user can try again.
                    return {
                        "id": slot_obj.current_id,
                        "status": "error",
                        "error": str(ex),
                        "cookies": {},
                        "time_taken": 0.0
                    }

            driver = slot_obj.driver
            sid = slot_obj.current_id
            start = time.time()

            try:
                driver.execute_script("window.open('about:blank','_blank');")
                new_tab = driver.window_handles[-1]
                driver.switch_to.window(new_tab)

                driver.get(TICKETMASTER_URL)
                time.sleep(2)

                cookies_list = driver.get_cookies()
                cookie_dict = {c["name"]: c["value"] for c in cookies_list}

                found_tmpt = any(
                    TMPT_COOKIE_SUBSTRING.lower() in c["name"].lower()
                    or TMPT_COOKIE_SUBSTRING.lower() in c["value"].lower()
                    for c in cookies_list
                )

                duration = round(time.time() - start, 2)

                driver.delete_all_cookies()
                driver.execute_script("window.localStorage.clear(); window.sessionStorage.clear();")
                driver.close()
                if driver.window_handles:
                    driver.switch_to.window(driver.window_handles[0])

                if found_tmpt:
                    slot_obj.fail_count = 0
                    return {
                        "id": sid,
                        "status": "success",
                        "cookies": cookie_dict,
                        "time_taken": duration
                    }
                else:
                    # no 'tmpt'
                    slot_obj.fail_count += 1
                    if slot_obj.fail_count >= NO_COOKIE_FAIL_THRESHOLD:
                        # remove driver => new id => return 'retry' with that new id
                        old_id = slot_obj.current_id
                        slot_obj.close_driver_and_new_id()
                        return {
                            "id": slot_obj.current_id,  # brand new id
                            "status": "retry",
                            "cookies": cookie_dict,
                            "time_taken": duration,
                            "old_id": old_id
                        }
                    else:
                        return {
                            "id": sid,
                            "status": "retry",
                            "cookies": cookie_dict,
                            "time_taken": duration
                        }

            except WebDriverException as we:
                # crash => remove driver => new id => return 'error'
                old_id = slot_obj.current_id
                slot_obj.close_driver_and_new_id()
                return {
                    "id": slot_obj.current_id,  # new ID
                    "status": "error",
                    "error": str(we),
                    "cookies": {},
                    "time_taken": round(time.time() - start, 2),
                    "old_id": old_id
                }

    def close_all(self):
        for s_obj in self.slots.values():
            s_obj.close_driver_and_new_id()
        self.slots.clear()
        LOG.info("[Slots] all cleared.")


slots_manager = SlotsManager()
app = FastAPI()

@asynccontextmanager
async def lifespan(app: FastAPI):
    LOG.info("[Startup] 3 stable slots => each has a 'current_id'")
    slots_manager.initialize_slots()
    yield
    LOG.info("[Shutdown] => close all drivers + clear slots")
    slots_manager.close_all()

app.router.lifespan_context = lifespan

@app.get("/browserlist")
async def browserlist():
    """ Return something like { "browser1": <uuid>, "browser2": <uuid>, "browser3": <uuid> } """
    data = slots_manager.get_browserlist()
    return JSONResponse(data)

@app.get("/cookies")
async def cookies_endpoint(
    id: Optional[str] = None,
    proxy: Optional[str] = Query(None, description="Proxy, e.g. http://user:pass@host:port")
):
    """
    GET /cookies?id=some_id&proxy=...
      1) concurrency => up to 3 in parallel
      2) find slot => 400 if invalid
      3) if 'tmpt' found => success
         else => fail_count++ => if 3 => remove driver => new slot id => return that new ID in JSON
      4) if crash => also remove driver => new ID => return 'error'
      5) always returns 'cookies'
    """
    if not id:
        raise HTTPException(status_code=422, detail="Query param 'id' is required.")

    slot_obj = slots_manager.find_slot_by_id(id)
    if not slot_obj:
        return JSONResponse({"error": f"No slot with session_id={id}"}, status_code=400)

    async with CONCURRENCY_LIMITER:
        result = slots_manager.do_cookies_nav(slot_obj, proxy_str=proxy)
        return JSONResponse(result)


if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8000)
