from flask import Flask, request, jsonify
from flask_cors import CORS
import time
from typing import Dict, List
from config import Config
from database import db
from models import Position, validate_portfolio_allocation
from zerodha_api import zerodha_api
from strategy_engine import strategy_engine

app = Flask(__name__)
CORS(app)

@app.route('/api/health', methods=['GET'])
def health_check():
    return jsonify({
        'status': 'healthy',
        'timestamp': int(time.time()),
        'database': 'connected'
    })

@app.route('/api/portfolio', methods=['GET'])
def get_portfolio():
    try:
        positions = db.get_positions()
        portfolio_data = []
        
        for pos_data in positions:
            position = Position(
                id=pos_data['id'],
                ticker=pos_data['ticker'],
                avg_price=pos_data['avg_price'],
                units=pos_data['units'],
                current_price=pos_data.get('current_price', 0),
                portfolio_percentage=pos_data.get('portfolio_percentage', 0),
                exchange=pos_data.get('exchange', 'NSE')
            )
            
            pos_dict = position.to_dict()
            
            strategy_data = db.get_strategies(position.id)
            if strategy_data:
                pos_dict['has_strategy'] = True
                pos_dict['investment_strategy'] = strategy_data.get('investment_strategy')
                pos_dict['profit_strategy'] = strategy_data.get('profit_strategy')
                pos_dict['financial_year'] = strategy_data.get('financial_year')
                
                inv_completion = 0
                profit_completion = 0
                
                if strategy_data.get('investment_strategy'):
                    inv_strategy = strategy_engine.parse_investment_strategy(strategy_data['investment_strategy'])
                    inv_completion = inv_strategy.completion_percentage
                
                if strategy_data.get('profit_strategy'):
                    profit_strategy = strategy_engine.parse_profit_strategy(strategy_data['profit_strategy'])
                    profit_completion = profit_strategy.completion_percentage
                
                pos_dict['investment_completion'] = inv_completion
                pos_dict['profit_completion'] = profit_completion
            else:
                pos_dict['has_strategy'] = False
                pos_dict['investment_completion'] = 0
                pos_dict['profit_completion'] = 0
            
            portfolio_data.append(pos_dict)
        
        summary = strategy_engine.calculate_portfolio_summary([
            Position(
                ticker=p['ticker'],
                avg_price=p['avg_price'],
                units=p['units'],
                current_price=p.get('current_price', 0),
                portfolio_percentage=p.get('portfolio_percentage', 0)
            ) for p in positions
        ])
        
        return jsonify({
            'success': True,
            'positions': portfolio_data,
            'summary': summary
        })
        
    except Exception as e:
        return jsonify({
            'success': False,
            'error': str(e)
        }), 500

@app.route('/api/portfolio/<int:position_id>', methods=['PUT'])
def update_position(position_id):
    try:
        data = request.get_json()
        
        if 'avg_price' in data and data['avg_price'] <= 0:
            return jsonify({'success': False, 'error': 'Average price must be positive'}), 400
        
        if 'units' in data and data['units'] <= 0:
            return jsonify({'success': False, 'error': 'Units must be positive'}), 400
        
        if 'portfolio_percentage' in data and not (0 <= data['portfolio_percentage'] <= 100):
            return jsonify({'success': False, 'error': 'Portfolio percentage must be between 0 and 100'}), 400
        
        success = db.update_position(position_id, data)
        
        if success:
            return jsonify({'success': True})
        else:
            return jsonify({'success': False, 'error': 'Position not found'}), 404
            
    except Exception as e:
        return jsonify({'success': False, 'error': str(e)}), 500

@app.route('/api/portfolio', methods=['POST'])
def add_position():
    try:
        data = request.get_json()
        
        position = Position(
            ticker=data['ticker'],
            avg_price=data['avg_price'],
            units=data['units'],
            exchange=data.get('exchange', 'NSE'),
            portfolio_percentage=data.get('portfolio_percentage', 0)
        )
        
        position_id = db.add_position(position.to_dict())
        
        return jsonify({
            'success': True,
            'position_id': position_id
        })
        
    except ValueError as e:
        return jsonify({'success': False, 'error': str(e)}), 400
    except Exception as e:
        return jsonify({'success': False, 'error': str(e)}), 500

@app.route('/api/portfolio/<int:position_id>', methods=['DELETE'])
def delete_position(position_id):
    try:
        success = db.delete_position(position_id)
        
        if success:
            return jsonify({'success': True})
        else:
            return jsonify({'success': False, 'error': 'Position not found'}), 404
            
    except Exception as e:
        return jsonify({'success': False, 'error': str(e)}), 500

@app.route('/api/portfolio/sync', methods=['POST'])
def sync_portfolio():
    try:
        if not zerodha_api.kite:
            return jsonify({
                'success': False,
                'error': 'Zerodha API not configured'
            }), 400
        
        success = zerodha_api.sync_positions_to_db()
        
        if success:
            return jsonify({
                'success': True,
                'message': 'Portfolio synced successfully'
            })
        else:
            return jsonify({
                'success': False,
                'error': 'Failed to sync portfolio'
            }), 500
            
    except Exception as e:
        return jsonify({'success': False, 'error': str(e)}), 500

@app.route('/api/prices/<symbols>', methods=['GET'])
def get_prices(symbols):
    try:
        symbol_list = symbols.split(',')
        
        cached_prices = db.get_cached_prices(symbol_list)
        
        if len(cached_prices) == len(symbol_list):
            return jsonify({
                'success': True,
                'prices': cached_prices,
                'source': 'cache'
            })
        
        if zerodha_api.kite:
            success = zerodha_api.update_prices_in_db(symbol_list)
            if success:
                updated_prices = db.get_cached_prices(symbol_list)
                return jsonify({
                    'success': True,
                    'prices': updated_prices,
                    'source': 'live'
                })
        
        return jsonify({
            'success': True,
            'prices': cached_prices,
            'source': 'cache_partial'
        })
        
    except Exception as e:
        return jsonify({'success': False, 'error': str(e)}), 500

@app.route('/api/strategies/<int:position_id>', methods=['GET'])
def get_strategies(position_id):
    try:
        strategy_data = db.get_strategies(position_id)
        
        if strategy_data:
            positions = db.get_positions()
            position_data = next((p for p in positions if p['id'] == position_id), None)
            
            if position_data:
                position = Position(
                    id=position_data['id'],
                    ticker=position_data['ticker'],
                    avg_price=position_data['avg_price'],
                    units=position_data['units'],
                    current_price=position_data.get('current_price', 0),
                    portfolio_percentage=position_data.get('portfolio_percentage', 0)
                )
                
                result = {
                    'success': True,
                    'strategy': strategy_data,
                    'position': position.to_dict()
                }
                
                if strategy_data.get('investment_strategy'):
                    portfolio_value = 100000
                    result['investment_calculations'] = strategy_engine.calculate_investment_amounts(
                        position, 
                        strategy_engine.parse_investment_strategy(strategy_data['investment_strategy']),
                        portfolio_value
                    )
                
                if strategy_data.get('profit_strategy'):
                    result['profit_calculations'] = strategy_engine.calculate_profit_targets(
                        position,
                        strategy_engine.parse_profit_strategy(strategy_data['profit_strategy'])
                    )
                
                return jsonify(result)
        
        return jsonify({
            'success': True,
            'strategy': {},
            'position': None
        })
        
    except Exception as e:
        return jsonify({'success': False, 'error': str(e)}), 500

@app.route('/api/strategies', methods=['POST'])
def save_strategy():
    try:
        data = request.get_json()
        
        validation = strategy_engine.validate_strategy_data(data)
        if not validation['valid']:
            return jsonify({
                'success': False,
                'error': 'Invalid strategy configuration',
                'details': validation['errors']
            }), 400
        
        position_id = data['position_id']
        strategy_id = db.save_strategy(position_id, data)
        
        return jsonify({
            'success': True,
            'strategy_id': strategy_id,
            'warnings': validation.get('warnings', [])
        })
        
    except Exception as e:
        return jsonify({'success': False, 'error': str(e)}), 500

@app.route('/api/strategies/<int:position_id>/triggers', methods=['GET'])
def get_strategy_triggers(position_id):
    try:
        positions = db.get_positions()
        position_data = next((p for p in positions if p['id'] == position_id), None)
        
        if not position_data:
            return jsonify({'success': False, 'error': 'Position not found'}), 404
        
        position = Position(
            id=position_data['id'],
            ticker=position_data['ticker'],
            avg_price=position_data['avg_price'],
            units=position_data['units'],
            current_price=position_data.get('current_price', 0),
            portfolio_percentage=position_data.get('portfolio_percentage', 0)
        )
        
        triggers = strategy_engine.check_strategy_triggers(position, position.current_price)
        
        return jsonify({
            'success': True,
            'triggers': triggers
        })
        
    except Exception as e:
        return jsonify({'success': False, 'error': str(e)}), 500

@app.route('/api/strategies/<int:position_id>/update-status', methods=['PUT'])
def update_strategy_status(position_id):
    try:
        data = request.get_json()
        
        success = strategy_engine.update_strategy_status(
            position_id,
            data['step_type'],
            data['step_number'],
            data['status'],
            data.get('filled_price')
        )
        
        if success:
            return jsonify({'success': True})
        else:
            return jsonify({'success': False, 'error': 'Failed to update strategy status'}), 500
            
    except Exception as e:
        return jsonify({'success': False, 'error': str(e)}), 500

@app.route('/api/trades/sync', methods=['POST'])
def sync_trades():
    try:
        if not zerodha_api.kite:
            return jsonify({
                'success': False,
                'error': 'Zerodha API not configured'
            }), 400
        
        success = zerodha_api.sync_trades_to_db()
        
        if success:
            return jsonify({
                'success': True,
                'message': 'Trades synced successfully'
            })
        else:
            return jsonify({
                'success': False,
                'error': 'Failed to sync trades'
            }), 500
            
    except Exception as e:
        return jsonify({'success': False, 'error': str(e)}), 500

@app.route('/api/backup', methods=['POST'])
def backup_database():
    try:
        backup_path = db.backup_database()
        return jsonify({
            'success': True,
            'backup_file': backup_path
        })
        
    except Exception as e:
        return jsonify({'success': False, 'error': str(e)}), 500

@app.route('/api/zerodha/auth-url', methods=['GET'])
def get_auth_url():
    try:
        if not Config.ZERODHA_API_KEY:
            return jsonify({
                'success': False,
                'error': 'Zerodha API key not configured'
            }), 400
        
        auth_url = f"https://kite.trade/connect/login?api_key={Config.ZERODHA_API_KEY}"
        
        return jsonify({
            'success': True,
            'auth_url': auth_url
        })
        
    except Exception as e:
        return jsonify({'success': False, 'error': str(e)}), 500

@app.route('/api/zerodha/set-token', methods=['POST'])
def set_access_token():
    try:
        data = request.get_json()
        request_token = data.get('request_token')
        
        if not request_token:
            return jsonify({'success': False, 'error': 'Request token required'}), 400
        
        kite = KiteConnect(api_key=Config.ZERODHA_API_KEY)
        token_data = kite.generate_session(request_token, api_secret=Config.ZERODHA_API_SECRET)
        
        access_token = token_data['access_token']
        db.set_setting('zerodha_access_token', access_token)
        
        zerodha_api.access_token = access_token
        zerodha_api.initialize_kite()
        
        return jsonify({
            'success': True,
            'message': 'Access token set successfully'
        })
        
    except Exception as e:
        return jsonify({'success': False, 'error': str(e)}), 500

if __name__ == '__main__':
    app.run(
        debug=Config.FLASK_DEBUG,
        port=Config.FLASK_PORT,
        host='0.0.0.0'
    )