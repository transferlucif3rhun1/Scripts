import React, { useState } from "react";
import { useMutation, gql } from "@apollo/client";

const CREATE_RESTAURANT = gql`
  mutation CreateRestaurant($name: String!, $address: String!) {
    createRestaurant(name: $name, address: $address) {
      id
      name
      address
      ownerId
    }
  }
`;

export default function CreateRestaurant() {
  const [name, setName] = useState("");
  const [address, setAddress] = useState("");
  const [createRestaurant] = useMutation(CREATE_RESTAURANT);

  const handleCreate = async () => {
    try {
      const { data } = await createRestaurant({ variables: { name, address } });
      alert(`Created restaurant: ${data.createRestaurant.name}`);
      setName("");
      setAddress("");
    } catch (err) {
      alert("Error creating restaurant: " + err.message);
    }
  };

  return (
    <div>
      <h3>Create a Restaurant (Vendor/Admin only)</h3>
      <label>Name:</label>
      <br />
      <input value={name} onChange={(e) => setName(e.target.value)} />
      <br />
      <label>Address:</label>
      <br />
      <input value={address} onChange={(e) => setAddress(e.target.value)} />
      <br />
      <button onClick={handleCreate} style={{ marginTop: "1rem" }}>
        Create Restaurant
      </button>
    </div>
  );
}
