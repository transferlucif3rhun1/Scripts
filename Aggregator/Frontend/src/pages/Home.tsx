import React from 'react';
import { useQuery } from '@apollo/client';
import { GET_RESTAURANTS } from '../graphql/queries';

const Home: React.FC = () => {
  const { data, loading, error } = useQuery(GET_RESTAURANTS);

  if (loading) return <p>Loading restaurants...</p>;
  if (error) return <p>Error: {error.message}</p>;

  return (
    <div>
      <h2>Restaurant List</h2>
      {data?.getRestaurants?.map((rest: any) => (
        <div key={rest.id} style={{ marginBottom: '1rem' }}>
          <strong>{rest.name}</strong>
          <p>{rest.address}</p>
        </div>
      ))}
    </div>
  );
};

export default Home;
