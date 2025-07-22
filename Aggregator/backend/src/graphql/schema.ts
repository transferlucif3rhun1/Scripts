import { gql } from 'graphql-tag';

export const typeDefs = gql`
  type User {
    id: ID!
    email: String!
    isAdmin: Boolean
    isVendor: Boolean
    phone: String
    createdAt: String
    updatedAt: String
  }

  type Restaurant {
    id: ID!
    name: String!
    address: String!
    createdAt: String
    updatedAt: String
  }

  type OrderItem {
    productId: String
    quantity: Int
  }

  type Order {
    id: ID!
    userId: String!
    restaurantId: String!
    items: [OrderItem!]!
    paymentMethod: String
    status: String
    createdAt: String
    updatedAt: String
  }

  type Review {
    id: ID!
    userId: String!
    orderId: String!
    restaurantId: String!
    rating: Int!
    comment: String
    createdAt: String
    updatedAt: String
  }

  type ErrorReport {
    id: ID!
    userId: String!
    category: String
    priority: String
    description: String
    screenshotURL: String
    status: String
    createdAt: String
    updatedAt: String
  }

  type AuthPayload {
    token: String!
    user: User!
  }

  type Query {
    me: User
    getRestaurant(id: ID!): Restaurant
    getRestaurants: [Restaurant!]!
    getOrder(id: ID!): Order
    getOrders: [Order!]!
    getReviewsForRestaurant(restaurantId: ID!): [Review!]!
    getErrorReports: [ErrorReport!]!
  }

  type Mutation {
    register(email: String!, password: String!, isAdmin: Boolean, isVendor: Boolean, phone: String): AuthPayload
    login(email: String!, password: String!): AuthPayload
    updateUser(phone: String): User

    createRestaurant(name: String!, address: String!): Restaurant

    createOrder(restaurantId: String!, items: [OrderItemInput!]!, paymentMethod: String!): Order
    updateOrderStatus(orderId: ID!, status: String!): Order

    submitReview(orderId: ID!, rating: Int!, comment: String): Review

    submitErrorReport(category: String, priority: String, description: String!, screenshotURL: String): ErrorReport
    updateErrorReport(id: ID!, status: String): ErrorReport
  }

  input OrderItemInput {
    productId: String
    quantity: Int
  }
`;
