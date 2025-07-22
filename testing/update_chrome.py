import subprocess
import sys
from webdriver_manager.chrome import ChromeDriverManager
from selenium import webdriver
from selenium.webdriver.chrome.service import Service

def update_chrome_browser():
    try:
        # Windows: Update Chrome via Google Update (requires admin privileges)
        subprocess.run(["powershell", 
                        "Start-Process", 
                        "'C:\\Program Files (x86)\\Google\\Update\\GoogleUpdate.exe'", 
                        "/ua /installsource scheduler", 
                        "-Verb RunAs"], 
                       check=True)
        print("Chrome update initiated.")
    except Exception as e:
        print(f"Failed to update Chrome: {e}")

def update_chromedriver():
    try:
        # Automatically download and use the latest ChromeDriver
        driver_service = Service(ChromeDriverManager().install())
        driver = webdriver.Chrome(service=driver_service)
        driver.get("https://www.google.com")
        print("ChromeDriver updated and browser launched successfully.")
        driver.quit()
    except Exception as e:
        print(f"Failed to update ChromeDriver: {e}")

if __name__ == "__main__":
    update_chrome_browser()  # Optional: Requires admin rights
    update_chromedriver()
