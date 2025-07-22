import { useEffect, useState } from "react";
import { useQuery, gql } from "@apollo/client";

const GET_ME = gql`
  query {
    me {
      id
      email
      phone
      isAdmin
      isVendor
    }
  }
`;

export function useAuth() {
  const [currentUser, setCurrentUser] = useState<any>(null);
  const [isAuthLoading, setIsAuthLoading] = useState(true);

  const { data, loading, refetch } = useQuery(GET_ME, {
    fetchPolicy: "network-only",
  });

  useEffect(() => {
    if (!loading) {
      setCurrentUser(data?.me || null);
      setIsAuthLoading(false);
    }
  }, [data, loading]);

  // Provide a way to refresh the user manually
  const refreshUser = () => refetch();

  return { currentUser, isAuthLoading, refreshUser };
}
