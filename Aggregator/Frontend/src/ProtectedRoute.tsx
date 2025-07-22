import React from "react";
import { Navigate } from "react-router-dom";
import { useAuth } from "../utils/useAuthContext";

export default function ProtectedRoute({ children }: any) {
  const { currentUser, isAuthLoading } = useAuth();
  if (isAuthLoading) return <p>Loading...</p>;
  if (!currentUser) return <Navigate to="/login" />;
  return children;
}
