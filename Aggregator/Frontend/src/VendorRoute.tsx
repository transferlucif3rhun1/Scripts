import React from "react";
import { Navigate } from "react-router-dom";
import { useAuth } from "../utils/useAuthContext";

export default function VendorRoute({ children }: any) {
  const { currentUser, isAuthLoading } = useAuth();
  if (isAuthLoading) return <p>Loading...</p>;
  if (!currentUser || !currentUser.isVendor) return <Navigate to="/" />;
  return children;
}
