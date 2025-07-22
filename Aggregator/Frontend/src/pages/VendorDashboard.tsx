import React from "react";
import { useQuery, gql } from "@apollo/client";

const GET_RESTAURANTS = gql`
  query {
    restaurants {
      id
      name
      address
      ownerId
    }
  }
`;

export default function VendorDashboard({ socket }) {
  const { loading, error, data } = useQuery(GET_RESTAURANTS);

  if (loading) return <p>Loading restaurants...</p>;
  if (error) return <p>Error: {error.message}</p>;

  // You might filter only restaurants owned by the vendor's userId if needed

  return (
    <div>
      <h2>Vendor Dashboard</h2>
      <p>Manage your restaurants, orders, etc.</p>
      <hr />
      <h4>All Restaurants</h4>
      {data.restaurants.map((r) => (
        <div key={r.id} className="card mb-2">
          <div className="card-body">
            <p>Name: {r.name}</p>
            <p>Address: {r.address}</p>
            <small>OwnerID: {r.ownerId}</small>
          </div>
        </div>
      ))}
      {/* In a real app, you'd have forms to create/edit restaurants, see vendor orders, etc. */}
    </div>
  );
}
