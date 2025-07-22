import { gql } from '@apollo/client';

export const REGISTER = gql`
  mutation Register($email: String!, $password: String!, $isAdmin: Boolean, $isVendor: Boolean, $phone: String) {
    register(email: $email, password: $password, isAdmin: $isAdmin, isVendor: $isVendor, phone: $phone) {
      token
      user {
        id
        email
        isAdmin
        isVendor
      }
    }
  }
`;

export const LOGIN = gql`
  mutation Login($email: String!, $password: String!) {
    login(email: $email, password: $password) {
      token
      user {
        id
        email
        isAdmin
        isVendor
      }
    }
  }
`;

export const CREATE_ORDER = gql`
  mutation CreateOrder($restaurantId: String!, $items: [OrderItemInput!]!, $paymentMethod: String!) {
    createOrder(restaurantId: $restaurantId, items: $items, paymentMethod: $paymentMethod) {
      id
      status
    }
  }
`;

export const UPDATE_ORDER_STATUS = gql`
  mutation UpdateOrderStatus($orderId: ID!, $status: String!) {
    updateOrderStatus(orderId: $orderId, status: $status) {
      id
      status
    }
  }
`;
