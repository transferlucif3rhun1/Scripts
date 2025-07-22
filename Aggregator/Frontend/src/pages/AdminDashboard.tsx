import React, { useEffect } from 'react';
import socket from '../socket';

const AdminDashboard: React.FC = () => {
  useEffect(() => {
    // Listen for real-time events
    socket.on('NEW_ORDER', (data) => {
      console.log('New Order Received:', data);
      alert(`New order arrived! ID: ${data.orderId}`);
    });
    socket.on('ERROR_REPORT_UPDATED', (data) => {
      console.log('Error Report Updated:', data);
    });
    // Cleanup on unmount
    return () => {
      socket.off('NEW_ORDER');
      socket.off('ERROR_REPORT_UPDATED');
    };
  }, []);

  return (
    <div>
      <h2>Admin Dashboard</h2>
      <p>Real-time notifications will appear here.</p>
      {/* Could add more admin-specific features, like listing error reports, etc. */}
    </div>
  );
};

export default AdminDashboard;
