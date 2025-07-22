from typing import Dict, List, Optional
from models import Position, Strategy, InvestmentStrategy, ProfitStrategy, StrategyStep, ProfitTarget
from database import db

class StrategyEngine:
    def __init__(self):
        pass
    
    def calculate_investment_amounts(self, position: Position, strategy: InvestmentStrategy, 
                                   portfolio_value: float) -> Dict:
        position_allocation = (position.portfolio_percentage / 100) * portfolio_value
        amounts = []
        
        for step in strategy.steps:
            step_amount = (step.allocation_percentage / 100) * position_allocation
            amounts.append({
                'zone_price': step.zone_price,
                'allocation_percentage': step.allocation_percentage,
                'amount': step_amount,
                'units': int(step_amount / step.zone_price) if step.zone_price > 0 else 0,
                'status': step.status
            })
        
        return {
            'total_allocation': position_allocation,
            'steps': amounts
        }
    
    def calculate_profit_targets(self, position: Position, strategy: ProfitStrategy) -> Dict:
        targets = []
        
        for target in strategy.targets:
            if target.target_type == 'percentage':
                target_price = position.avg_price * (1 + target.target_percentage / 100)
            else:
                target_price = target.target_percentage
            
            exit_units = int((target.exit_percentage / 100) * position.units)
            exit_value = exit_units * target_price
            
            targets.append({
                'target_percentage': target.target_percentage,
                'target_price': target_price,
                'exit_percentage': target.exit_percentage,
                'exit_units': exit_units,
                'exit_value': exit_value,
                'status': target.status
            })
        
        return {
            'targets': targets,
            'total_units': position.units
        }
    
    def check_strategy_triggers(self, position: Position, current_price: float) -> Dict:
        triggers = {
            'investment_triggers': [],
            'profit_triggers': []
        }
        
        strategy_data = db.get_strategies(position.id)
        if not strategy_data:
            return triggers
        
        if strategy_data.get('investment_strategy'):
            inv_strategy = self.parse_investment_strategy(strategy_data['investment_strategy'])
            for i, step in enumerate(inv_strategy.steps):
                if step.status == 'pending' and current_price <= step.zone_price:
                    triggers['investment_triggers'].append({
                        'step_number': i + 1,
                        'zone_price': step.zone_price,
                        'current_price': current_price,
                        'allocation_percentage': step.allocation_percentage
                    })
        
        if strategy_data.get('profit_strategy'):
            profit_strategy = self.parse_profit_strategy(strategy_data['profit_strategy'])
            for i, target in enumerate(profit_strategy.targets):
                if target.status == 'pending':
                    if target.target_type == 'percentage':
                        target_price = position.avg_price * (1 + target.target_percentage / 100)
                    else:
                        target_price = target.target_percentage
                    
                    if current_price >= target_price:
                        triggers['profit_triggers'].append({
                            'target_number': i + 1,
                            'target_price': target_price,
                            'current_price': current_price,
                            'exit_percentage': target.exit_percentage
                        })
        
        return triggers
    
    def parse_investment_strategy(self, strategy_dict: Dict) -> InvestmentStrategy:
        steps = []
        for step_data in strategy_dict.get('steps', []):
            step = StrategyStep(
                zone_price=step_data['zone_price'],
                allocation_percentage=step_data['allocation_percentage'],
                status=step_data.get('status', 'pending'),
                filled_price=step_data.get('filled_price'),
                filled_date=step_data.get('filled_date')
            )
            steps.append(step)
        return InvestmentStrategy(steps=steps)
    
    def parse_profit_strategy(self, strategy_dict: Dict) -> ProfitStrategy:
        targets = []
        for target_data in strategy_dict.get('targets', []):
            target = ProfitTarget(
                target_percentage=target_data['target_percentage'],
                exit_percentage=target_data['exit_percentage'],
                target_type=target_data.get('target_type', 'percentage'),
                status=target_data.get('status', 'pending'),
                filled_price=target_data.get('filled_price'),
                filled_date=target_data.get('filled_date')
            )
            targets.append(target)
        return ProfitStrategy(targets=targets)
    
    def update_strategy_status(self, position_id: int, step_type: str, step_number: int, 
                             status: str, filled_price: Optional[float] = None) -> bool:
        strategy_data = db.get_strategies(position_id)
        if not strategy_data:
            return False
        
        try:
            if step_type == 'investment':
                inv_strategy = strategy_data.get('investment_strategy', {})
                if 'steps' in inv_strategy and step_number < len(inv_strategy['steps']):
                    inv_strategy['steps'][step_number]['status'] = status
                    if filled_price:
                        inv_strategy['steps'][step_number]['filled_price'] = filled_price
                    
                    strategy_data['investment_strategy'] = inv_strategy
            
            elif step_type == 'profit':
                profit_strategy = strategy_data.get('profit_strategy', {})
                if 'targets' in profit_strategy and step_number < len(profit_strategy['targets']):
                    profit_strategy['targets'][step_number]['status'] = status
                    if filled_price:
                        profit_strategy['targets'][step_number]['filled_price'] = filled_price
                    
                    strategy_data['profit_strategy'] = profit_strategy
            
            db.save_strategy(position_id, strategy_data)
            return True
            
        except Exception as e:
            print(f"Error updating strategy status: {e}")
            return False
    
    def calculate_portfolio_summary(self, positions: List[Position]) -> Dict:
        total_investment = sum(p.investment_value for p in positions)
        total_current = sum(p.current_value for p in positions)
        total_pnl = total_current - total_investment
        total_pnl_percentage = (total_pnl / total_investment * 100) if total_investment > 0 else 0
        
        allocation_used = sum(p.portfolio_percentage for p in positions)
        
        return {
            'total_investment': total_investment,
            'total_current_value': total_current,
            'total_pnl': total_pnl,
            'total_pnl_percentage': total_pnl_percentage,
            'allocation_used': allocation_used,
            'allocation_remaining': 100 - allocation_used,
            'position_count': len(positions)
        }
    
    def validate_strategy_data(self, strategy_data: Dict) -> Dict:
        errors = []
        warnings = []
        
        if 'investment_strategy' in strategy_data:
            inv_strategy = strategy_data['investment_strategy']
            if 'steps' in inv_strategy:
                total_allocation = sum(step.get('allocation_percentage', 0) for step in inv_strategy['steps'])
                if abs(total_allocation - 100) > 0.01:
                    errors.append(f"Investment strategy total allocation is {total_allocation}%, must be 100%")
                
                prev_price = float('inf')
                for i, step in enumerate(inv_strategy['steps']):
                    if step.get('zone_price', 0) >= prev_price:
                        warnings.append(f"Investment step {i+1} price should be lower than step {i}")
                    prev_price = step.get('zone_price', 0)
        
        if 'profit_strategy' in strategy_data:
            profit_strategy = strategy_data['profit_strategy']
            if 'targets' in profit_strategy:
                total_exit = sum(target.get('exit_percentage', 0) for target in profit_strategy['targets'])
                if abs(total_exit - 100) > 0.01:
                    errors.append(f"Profit strategy total exit is {total_exit}%, must be 100%")
        
        return {
            'valid': len(errors) == 0,
            'errors': errors,
            'warnings': warnings
        }

strategy_engine = StrategyEngine()