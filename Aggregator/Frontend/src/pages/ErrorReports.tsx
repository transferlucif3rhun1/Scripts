import React, { useState } from "react";
import { useQuery, useMutation, gql } from "@apollo/client";

const GET_ERROR_REPORTS = gql`
  query {
    errorReports {
      id
      category
      priority
      description
      screenshotURL
      status
      createdAt
    }
  }
`;

const SUBMIT_ERROR_REPORT = gql`
  mutation SubmitErrorReport($description: String!) {
    submitErrorReport(description: $description) {
      id
      status
    }
  }
`;

export default function ErrorReports() {
  const { loading, error, data, refetch } = useQuery(GET_ERROR_REPORTS);
  const [desc, setDesc] = useState("");
  const [submitReport] = useMutation(SUBMIT_ERROR_REPORT);

  if (loading) return <p>Loading error reports...</p>;
  if (error) return <p>Error: {error.message}</p>;

  const handleSubmit = async () => {
    if (!desc.trim()) return;
    try {
      await submitReport({ variables: { description: desc } });
      setDesc("");
      refetch();
      alert("Error report submitted!");
    } catch (e) {
      alert(e.message);
    }
  };

  return (
    <div>
      <h2>Error Reports</h2>
      <div className="mb-3">
        <input
          className="form-control"
          placeholder="Describe the issue"
          value={desc}
          onChange={(e) => setDesc(e.target.value)}
        />
        <button className="btn btn-primary mt-2" onClick={handleSubmit}>
          Submit Error Report
        </button>
      </div>
      <hr />
      {data.errorReports.map((r) => (
        <div key={r.id} className="card mb-2">
          <div className="card-body">
            <p>ID: {r.id}</p>
            <p>Category: {r.category || "N/A"}</p>
            <p>Priority: {r.priority || "Normal"}</p>
            <p>Description: {r.description}</p>
            <p>Status: {r.status}</p>
            <small>Created: {new Date(r.createdAt).toLocaleString()}</small>
          </div>
        </div>
      ))}
    </div>
  );
}
