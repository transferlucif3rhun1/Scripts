import React from "react";
import { Navigate } from "react-router-dom";
import { useAuth } from "../utils/useAuthContext";

export default function AdminRoute({ children }: any) {
  const { currentUser, isAuthLoading } = useAuth();
  if (isAuthLoading) return <p>Loading...</p>;
  if (!currentUser || !currentUser.isAdmin) return <Navigate to="/" />;
  return children;
}
