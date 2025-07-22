import React, { useState } from "react";
import { useMutation, gql } from "@apollo/client";

const REGISTER_MUTATION = gql`
  mutation Register($email: String!, $password: String!, $phone: String) {
    register(email: $email, password: $password, phone: $phone)
  }
`;

export default function Register() {
  const [form, setForm] = useState({ email: "", password: "", phone: "" });
  const [registerMutation, { loading }] = useMutation(REGISTER_MUTATION);

  const handleSubmit = async () => {
    try {
      await registerMutation({ variables: form });
      alert("Registration successful. You can now login!");
      window.location.href = "/login";
    } catch (err) {
      alert("Error: " + err.message);
    }
  };

  return (
    <div className="card mx-auto" style={{ maxWidth: 400 }}>
      <div className="card-body">
        <h3 className="card-title mb-3">Register</h3>
        <div className="mb-3">
          <label>Email</label>
          <input
            type="text"
            className="form-control"
            value={form.email}
            onChange={(e) => setForm({ ...form, email: e.target.value })}
          />
        </div>
        <div className="mb-3">
          <label>Password</label>
          <input
            type="password"
            className="form-control"
            value={form.password}
            onChange={(e) => setForm({ ...form, password: e.target.value })}
          />
        </div>
        <div className="mb-3">
          <label>Phone (optional)</label>
          <input
            type="text"
            className="form-control"
            value={form.phone}
            onChange={(e) => setForm({ ...form, phone: e.target.value })}
          />
        </div>
        <button className="btn btn-primary w-100" onClick={handleSubmit} disabled={loading}>
          {loading ? "Registering..." : "Register"}
        </button>
      </div>
    </div>
  );
}
