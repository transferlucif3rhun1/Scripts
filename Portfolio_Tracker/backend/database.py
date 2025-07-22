import sqlite3
import json
from datetime import datetime
from typing import List, Dict, Optional
from config import Config

class Database:
    def __init__(self):
        self.db_path = Config.DATABASE_PATH
        self.init_database()
    
    def get_connection(self):
        conn = sqlite3.connect(self.db_path)
        conn.execute("PRAGMA foreign_keys = ON")
        conn.row_factory = sqlite3.Row
        return conn
    
    def init_database(self):
        conn = self.get_connection()
        try:
            conn.executescript('''
                CREATE TABLE IF NOT EXISTS positions (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    ticker TEXT NOT NULL,
                    exchange TEXT DEFAULT 'NSE',
                    avg_price REAL NOT NULL CHECK(avg_price > 0),
                    units INTEGER NOT NULL CHECK(units > 0),
                    current_price REAL DEFAULT 0,
                    portfolio_percentage REAL DEFAULT 0 CHECK(portfolio_percentage >= 0 AND portfolio_percentage <= 100),
                    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
                    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
                );
                
                CREATE TABLE IF NOT EXISTS strategies (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    position_id INTEGER NOT NULL,
                    financial_year TEXT NOT NULL,
                    investment_strategy TEXT,
                    profit_strategy TEXT,
                    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
                    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
                    FOREIGN KEY (position_id) REFERENCES positions(id) ON DELETE CASCADE
                );
                
                CREATE TABLE IF NOT EXISTS trade_history (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    position_id INTEGER NOT NULL,
                    order_id TEXT,
                    trade_type TEXT NOT NULL,
                    quantity INTEGER NOT NULL,
                    price REAL NOT NULL,
                    trade_date TIMESTAMP,
                    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
                    FOREIGN KEY (position_id) REFERENCES positions(id) ON DELETE CASCADE
                );
                
                CREATE TABLE IF NOT EXISTS price_cache (
                    symbol TEXT PRIMARY KEY,
                    price REAL NOT NULL,
                    timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
                );
                
                CREATE TABLE IF NOT EXISTS app_settings (
                    key TEXT PRIMARY KEY,
                    value TEXT NOT NULL,
                    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
                );
                
                CREATE INDEX IF NOT EXISTS idx_positions_ticker ON positions(ticker);
                CREATE INDEX IF NOT EXISTS idx_strategies_position ON strategies(position_id);
                CREATE INDEX IF NOT EXISTS idx_trade_history_position ON trade_history(position_id);
            ''')
            conn.commit()
        finally:
            conn.close()
    
    def get_positions(self) -> List[Dict]:
        conn = self.get_connection()
        try:
            cursor = conn.execute('''
                SELECT p.*, s.investment_strategy, s.profit_strategy, s.financial_year
                FROM positions p
                LEFT JOIN strategies s ON p.id = s.position_id
                ORDER BY p.ticker
            ''')
            return [dict(row) for row in cursor.fetchall()]
        finally:
            conn.close()
    
    def add_position(self, data: Dict) -> int:
        conn = self.get_connection()
        try:
            cursor = conn.execute('''
                INSERT INTO positions (ticker, exchange, avg_price, units, current_price, portfolio_percentage)
                VALUES (?, ?, ?, ?, ?, ?)
            ''', (
                data['ticker'],
                data.get('exchange', 'NSE'),
                data['avg_price'],
                data['units'],
                data.get('current_price', 0),
                data.get('portfolio_percentage', 0)
            ))
            conn.commit()
            return cursor.lastrowid
        finally:
            conn.close()
    
    def update_position(self, position_id: int, data: Dict) -> bool:
        conn = self.get_connection()
        try:
            conn.execute('''
                UPDATE positions 
                SET avg_price = ?, units = ?, portfolio_percentage = ?, updated_at = CURRENT_TIMESTAMP
                WHERE id = ?
            ''', (
                data.get('avg_price'),
                data.get('units'),
                data.get('portfolio_percentage'),
                position_id
            ))
            conn.commit()
            return conn.total_changes > 0
        finally:
            conn.close()
    
    def delete_position(self, position_id: int) -> bool:
        conn = self.get_connection()
        try:
            conn.execute('DELETE FROM positions WHERE id = ?', (position_id,))
            conn.commit()
            return conn.total_changes > 0
        finally:
            conn.close()
    
    def update_current_prices(self, price_data: Dict) -> None:
        conn = self.get_connection()
        try:
            for symbol, price in price_data.items():
                conn.execute('''
                    INSERT OR REPLACE INTO price_cache (symbol, price, timestamp)
                    VALUES (?, ?, CURRENT_TIMESTAMP)
                ''', (symbol, price))
                
                conn.execute('''
                    UPDATE positions SET current_price = ? WHERE ticker = ?
                ''', (price, symbol))
            conn.commit()
        finally:
            conn.close()
    
    def get_cached_prices(self, symbols: List[str]) -> Dict[str, float]:
        conn = self.get_connection()
        try:
            placeholders = ','.join(['?' for _ in symbols])
            cursor = conn.execute(f'''
                SELECT symbol, price FROM price_cache 
                WHERE symbol IN ({placeholders})
                AND datetime(timestamp, '+{Config.PRICE_CACHE_SECONDS} seconds') > datetime('now')
            ''', symbols)
            return {row['symbol']: row['price'] for row in cursor.fetchall()}
        finally:
            conn.close()
    
    def save_strategy(self, position_id: int, strategy_data: Dict) -> int:
        conn = self.get_connection()
        try:
            conn.execute('''
                INSERT OR REPLACE INTO strategies 
                (position_id, financial_year, investment_strategy, profit_strategy, updated_at)
                VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
            ''', (
                position_id,
                strategy_data['financial_year'],
                json.dumps(strategy_data.get('investment_strategy', {})),
                json.dumps(strategy_data.get('profit_strategy', {}))
            ))
            conn.commit()
            return position_id
        finally:
            conn.close()
    
    def get_strategies(self, position_id: int) -> Dict:
        conn = self.get_connection()
        try:
            cursor = conn.execute('''
                SELECT investment_strategy, profit_strategy, financial_year
                FROM strategies WHERE position_id = ?
            ''', (position_id,))
            row = cursor.fetchone()
            if row:
                return {
                    'investment_strategy': json.loads(row['investment_strategy'] or '{}'),
                    'profit_strategy': json.loads(row['profit_strategy'] or '{}'),
                    'financial_year': row['financial_year']
                }
            return {}
        finally:
            conn.close()
    
    def add_trade(self, trade_data: Dict) -> int:
        conn = self.get_connection()
        try:
            cursor = conn.execute('''
                INSERT INTO trade_history 
                (position_id, order_id, trade_type, quantity, price, trade_date)
                VALUES (?, ?, ?, ?, ?, ?)
            ''', (
                trade_data['position_id'],
                trade_data.get('order_id'),
                trade_data['trade_type'],
                trade_data['quantity'],
                trade_data['price'],
                trade_data.get('trade_date', datetime.now())
            ))
            conn.commit()
            return cursor.lastrowid
        finally:
            conn.close()
    
    def get_setting(self, key: str) -> Optional[str]:
        conn = self.get_connection()
        try:
            cursor = conn.execute('SELECT value FROM app_settings WHERE key = ?', (key,))
            row = cursor.fetchone()
            return row['value'] if row else None
        finally:
            conn.close()
    
    def set_setting(self, key: str, value: str) -> None:
        conn = self.get_connection()
        try:
            conn.execute('''
                INSERT OR REPLACE INTO app_settings (key, value, updated_at)
                VALUES (?, ?, CURRENT_TIMESTAMP)
            ''', (key, value))
            conn.commit()
        finally:
            conn.close()
    
    def backup_database(self) -> str:
        import shutil
        backup_path = f"portfolio_backup_{datetime.now().strftime('%Y%m%d_%H%M%S')}.db"
        shutil.copy2(self.db_path, backup_path)
        return backup_path

db = Database()