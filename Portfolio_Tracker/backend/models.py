from dataclasses import dataclass, asdict
from typing import List, Dict, Optional
from datetime import datetime

@dataclass
class Position:
    ticker: str
    avg_price: float
    units: int
    exchange: str = 'NSE'
    current_price: float = 0.0
    portfolio_percentage: float = 0.0
    id: Optional[int] = None
    created_at: Optional[datetime] = None
    updated_at: Optional[datetime] = None
    
    def __post_init__(self):
        if self.avg_price <= 0:
            raise ValueError("Average price must be positive")
        if self.units <= 0:
            raise ValueError("Units must be positive")
        if not (0 <= self.portfolio_percentage <= 100):
            raise ValueError("Portfolio percentage must be between 0 and 100")
    
    @property
    def current_value(self) -> float:
        return self.current_price * self.units
    
    @property
    def investment_value(self) -> float:
        return self.avg_price * self.units
    
    @property
    def profit_loss(self) -> float:
        return self.current_value - self.investment_value
    
    @property
    def profit_loss_percentage(self) -> float:
        if self.investment_value == 0:
            return 0
        return (self.profit_loss / self.investment_value) * 100
    
    def to_dict(self) -> Dict:
        data = asdict(self)
        data['current_value'] = self.current_value
        data['investment_value'] = self.investment_value
        data['profit_loss'] = self.profit_loss
        data['profit_loss_percentage'] = self.profit_loss_percentage
        return data

@dataclass
class StrategyStep:
    zone_price: float
    allocation_percentage: float
    status: str = 'pending'
    filled_price: Optional[float] = None
    filled_date: Optional[datetime] = None
    
    def __post_init__(self):
        if self.zone_price <= 0:
            raise ValueError("Zone price must be positive")
        if not (0 <= self.allocation_percentage <= 100):
            raise ValueError("Allocation percentage must be between 0 and 100")
        if self.status not in ['pending', 'filled', 'skipped']:
            raise ValueError("Status must be pending, filled, or skipped")

@dataclass
class ProfitTarget:
    target_percentage: float
    exit_percentage: float
    target_type: str = 'percentage'
    status: str = 'pending'
    filled_price: Optional[float] = None
    filled_date: Optional[datetime] = None
    
    def __post_init__(self):
        if self.target_percentage <= 0:
            raise ValueError("Target percentage must be positive")
        if not (0 <= self.exit_percentage <= 100):
            raise ValueError("Exit percentage must be between 0 and 100")
        if self.target_type not in ['percentage', 'price']:
            raise ValueError("Target type must be percentage or price")
        if self.status not in ['pending', 'hit', 'skipped']:
            raise ValueError("Status must be pending, hit, or skipped")

@dataclass
class InvestmentStrategy:
    steps: List[StrategyStep]
    
    def __post_init__(self):
        total_allocation = sum(step.allocation_percentage for step in self.steps)
        if abs(total_allocation - 100) > 0.01:
            raise ValueError(f"Total allocation must be 100%, got {total_allocation}%")
    
    @property
    def completion_percentage(self) -> float:
        if not self.steps:
            return 0
        filled_steps = len([s for s in self.steps if s.status == 'filled'])
        return (filled_steps / len(self.steps)) * 100
    
    def to_dict(self) -> Dict:
        return {
            'steps': [asdict(step) for step in self.steps],
            'completion_percentage': self.completion_percentage
        }

@dataclass
class ProfitStrategy:
    targets: List[ProfitTarget]
    
    def __post_init__(self):
        total_exit = sum(target.exit_percentage for target in self.targets)
        if abs(total_exit - 100) > 0.01:
            raise ValueError(f"Total exit percentage must be 100%, got {total_exit}%")
    
    @property
    def completion_percentage(self) -> float:
        if not self.targets:
            return 0
        hit_targets = len([t for t in self.targets if t.status == 'hit'])
        return (hit_targets / len(self.targets)) * 100
    
    def to_dict(self) -> Dict:
        return {
            'targets': [asdict(target) for target in self.targets],
            'completion_percentage': self.completion_percentage
        }

@dataclass
class Strategy:
    position_id: int
    financial_year: str
    investment_strategy: Optional[InvestmentStrategy] = None
    profit_strategy: Optional[ProfitStrategy] = None
    
    def to_dict(self) -> Dict:
        data = {
            'position_id': self.position_id,
            'financial_year': self.financial_year,
            'investment_strategy': self.investment_strategy.to_dict() if self.investment_strategy else None,
            'profit_strategy': self.profit_strategy.to_dict() if self.profit_strategy else None
        }
        return data

@dataclass
class Trade:
    position_id: int
    trade_type: str
    quantity: int
    price: float
    order_id: Optional[str] = None
    trade_date: Optional[datetime] = None
    id: Optional[int] = None
    
    def __post_init__(self):
        if self.quantity <= 0:
            raise ValueError("Quantity must be positive")
        if self.price <= 0:
            raise ValueError("Price must be positive")
        if self.trade_type not in ['BUY', 'SELL']:
            raise ValueError("Trade type must be BUY or SELL")

def validate_portfolio_allocation(positions: List[Position]) -> bool:
    total_allocation = sum(p.portfolio_percentage for p in positions)
    return total_allocation <= 100