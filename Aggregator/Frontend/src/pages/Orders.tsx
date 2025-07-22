import React from 'react';
import { useQuery } from '@apollo/client';
import { GET_ORDERS } from '../graphql/queries';

const Orders: React.FC = () => {
  const { data, loading, error } = useQuery(GET_ORDERS);

  if (loading) return <p>Loading your orders...</p>;
  if (error) return <p>Error: {error.message}</p>;

  return (
    <div>
      <h2>My Orders</h2>
      {data?.getOrders?.length === 0 && <p>No orders found.</p>}
      {data?.getOrders?.map((order: any) => (
        <div key={order.id} style={{ marginBottom: '1rem' }}>
          <p>Order ID: {order.id}</p>
          <p>Status: {order.status}</p>
          <p>Restaurant: {order.restaurantId}</p>
        </div>
      ))}
    </div>
  );
};

export default Orders;
