import { gql } from '@apollo/client';

export const GET_ME = gql`
  query Me {
    me {
      id
      email
      isAdmin
      isVendor
      phone
    }
  }
`;

export const GET_RESTAURANTS = gql`
  query {
    getRestaurants {
      id
      name
      address
    }
  }
`;

export const GET_ORDERS = gql`
  query {
    getOrders {
      id
      status
      paymentMethod
      restaurantId
    }
  }
`;

export const GET_REVIEWS_FOR_RESTAURANT = gql`
  query GetReviews($restaurantId: ID!) {
    getReviewsForRestaurant(restaurantId: $restaurantId) {
      id
      rating
      comment
    }
  }
`;
