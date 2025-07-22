import React from 'react';
import { Link } from 'react-router-dom';

const Navbar: React.FC = () => {
  return (
    <nav style={{ padding: '1rem', borderBottom: '1px solid #ccc' }}>
      <Link to="/">Home</Link> |{' '}
      <Link to="/orders">My Orders</Link> |{' '}
      <Link to="/admin">Admin Dashboard</Link>
    </nav>
  );
};

export default Navbar;
