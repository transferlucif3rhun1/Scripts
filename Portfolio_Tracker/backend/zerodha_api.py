import time
from typing import Dict, List, Optional
from kiteconnect import KiteConnect
from config import Config
from database import db

class ZerodhaAPI:
    def __init__(self):
        self.api_key = Config.ZERODHA_API_KEY
        self.api_secret = Config.ZERODHA_API_SECRET
        self.access_token = Config.ZERODHA_ACCESS_TOKEN
        self.kite = None
        self.last_request_time = 0
        self.min_request_interval = 0.34
        
        if self.api_key and self.access_token:
            self.initialize_kite()
    
    def initialize_kite(self):
        try:
            self.kite = KiteConnect(api_key=self.api_key)
            self.kite.set_access_token(self.access_token)
            return True
        except Exception as e:
            print(f"Failed to initialize Kite Connect: {e}")
            return False
    
    def rate_limit(self):
        current_time = time.time()
        time_since_last = current_time - self.last_request_time
        if time_since_last < self.min_request_interval:
            time.sleep(self.min_request_interval - time_since_last)
        self.last_request_time = time.time()
    
    def get_profile(self) -> Optional[Dict]:
        if not self.kite:
            return None
        
        try:
            self.rate_limit()
            return self.kite.profile()
        except Exception as e:
            print(f"Error fetching profile: {e}")
            return None
    
    def get_positions(self) -> Optional[Dict]:
        if not self.kite:
            return None
        
        try:
            self.rate_limit()
            return self.kite.positions()
        except Exception as e:
            print(f"Error fetching positions: {e}")
            return None
    
    def get_holdings(self) -> Optional[List[Dict]]:
        if not self.kite:
            return None
        
        try:
            self.rate_limit()
            return self.kite.holdings()
        except Exception as e:
            print(f"Error fetching holdings: {e}")
            return None
    
    def get_ltp(self, instruments: List[str]) -> Optional[Dict]:
        if not self.kite or not instruments:
            return None
        
        try:
            self.rate_limit()
            return self.kite.ltp(instruments)
        except Exception as e:
            print(f"Error fetching LTP: {e}")
            return None
    
    def get_orders(self) -> Optional[List[Dict]]:
        if not self.kite:
            return None
        
        try:
            self.rate_limit()
            return self.kite.orders()
        except Exception as e:
            print(f"Error fetching orders: {e}")
            return None
    
    def get_trades(self) -> Optional[List[Dict]]:
        if not self.kite:
            return None
        
        try:
            self.rate_limit()
            return self.kite.trades()
        except Exception as e:
            print(f"Error fetching trades: {e}")
            return None
    
    def sync_positions_to_db(self) -> bool:
        holdings = self.get_holdings()
        if not holdings:
            return False
        
        try:
            existing_positions = {p['ticker']: p for p in db.get_positions()}
            
            for holding in holdings:
                ticker = holding['tradingsymbol']
                avg_price = float(holding['average_price'])
                units = int(holding['quantity'])
                current_price = float(holding['last_price'])
                
                if ticker in existing_positions:
                    position_id = existing_positions[ticker]['id']
                    db.update_position(position_id, {
                        'avg_price': avg_price,
                        'units': units,
                        'current_price': current_price
                    })
                else:
                    db.add_position({
                        'ticker': ticker,
                        'avg_price': avg_price,
                        'units': units,
                        'current_price': current_price,
                        'portfolio_percentage': 0
                    })
            
            db.set_setting('last_sync', str(int(time.time())))
            return True
            
        except Exception as e:
            print(f"Error syncing positions: {e}")
            return False
    
    def update_prices_in_db(self, symbols: List[str]) -> bool:
        if not symbols:
            return True
        
        cached_prices = db.get_cached_prices(symbols)
        symbols_to_fetch = [s for s in symbols if s not in cached_prices]
        
        if not symbols_to_fetch:
            return True
        
        instruments = [f"NSE:{symbol}" for symbol in symbols_to_fetch]
        ltp_data = self.get_ltp(instruments)
        
        if not ltp_data:
            return False
        
        try:
            price_updates = {}
            for instrument, data in ltp_data.items():
                symbol = instrument.replace('NSE:', '')
                price_updates[symbol] = data['last_price']
            
            db.update_current_prices(price_updates)
            return True
            
        except Exception as e:
            print(f"Error updating prices: {e}")
            return False
    
    def sync_trades_to_db(self) -> bool:
        trades = self.get_trades()
        if not trades:
            return False
        
        try:
            positions = {p['ticker']: p['id'] for p in db.get_positions()}
            
            for trade in trades:
                ticker = trade['tradingsymbol']
                if ticker not in positions:
                    continue
                
                trade_data = {
                    'position_id': positions[ticker],
                    'order_id': trade['order_id'],
                    'trade_type': trade['transaction_type'],
                    'quantity': int(trade['quantity']),
                    'price': float(trade['average_price']),
                    'trade_date': trade['fill_timestamp']
                }
                
                db.add_trade(trade_data)
            
            return True
            
        except Exception as e:
            print(f"Error syncing trades: {e}")
            return False

zerodha_api = ZerodhaAPI()