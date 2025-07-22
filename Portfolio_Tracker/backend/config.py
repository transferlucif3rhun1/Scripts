import os
from dotenv import load_dotenv

load_dotenv()

class Config:
    DATABASE_PATH = 'portfolio.db'
    ZERODHA_API_KEY = os.getenv('ZERODHA_API_KEY', '')
    ZERODHA_API_SECRET = os.getenv('ZERODHA_API_SECRET', '')
    ZERODHA_ACCESS_TOKEN = os.getenv('ZERODHA_ACCESS_TOKEN', '')
    PRICE_CACHE_SECONDS = 60
    FLASK_DEBUG = os.getenv('FLASK_DEBUG', 'False').lower() == 'true'
    FLASK_PORT = int(os.getenv('FLASK_PORT', '5000'))