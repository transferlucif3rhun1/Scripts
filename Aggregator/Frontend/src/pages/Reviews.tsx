import React from "react";
import { useQuery, gql } from "@apollo/client";

const GET_REVIEWS = gql`
  query {
    reviews {
      id
      rating
      comment
      createdAt
    }
  }
`;

export default function Reviews() {
  const { loading, error, data } = useQuery(GET_REVIEWS);

  if (loading) return <p>Loading reviews...</p>;
  if (error) return <p>Error: {error.message}</p>;

  return (
    <div>
      <h2>User Reviews</h2>
      {data.reviews.map((r) => (
        <div key={r.id} className="card mb-2">
          <div className="card-body">
            <p>Rating: {r.rating}</p>
            <p>Comment: {r.comment}</p>
            <small>Created: {new Date(r.createdAt).toLocaleString()}</small>
          </div>
        </div>
      ))}
    </div>
  );
}
