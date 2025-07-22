import React, { useState } from "react";
import { useMutation, gql } from "@apollo/client";

const LOGIN_MUTATION = gql`
  mutation Login($email: String!, $password: String!) {
    login(email: $email, password: $password)
  }
`;

export default function Login() {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [loginMutation, { loading }] = useMutation(LOGIN_MUTATION);

  const handleLogin = async () => {
    try {
      const { data } = await loginMutation({ variables: { email, password } });
      if (data.login) {
        localStorage.setItem("token", data.login);
        alert("Login successful!");
        window.location.href = "/";
      }
    } catch (err) {
      alert("Login failed: " + err.message);
    }
  };

  return (
    <div className="card mx-auto" style={{ maxWidth: 400 }}>
      <div className="card-body">
        <h3 className="card-title mb-3">Login</h3>
        <div className="mb-3">
          <label>Email</label>
          <input
            type="text"
            className="form-control"
            disabled={loading}
            value={email}
            onChange={(e) => setEmail(e.target.value)}
          />
        </div>
        <div className="mb-3">
          <label>Password</label>
          <input
            type="password"
            className="form-control"
            disabled={loading}
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        </div>
        <button className="btn btn-primary w-100" onClick={handleLogin} disabled={loading}>
          {loading ? "Logging in..." : "Login"}
        </button>
        <p className="mt-2 text-center">
          Don’t have an account? <a href="/register">Register</a>
        </p>
      </div>
    </div>
  );
}
