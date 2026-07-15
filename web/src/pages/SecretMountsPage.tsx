import { useState, type FormEvent } from "react";
import { useParams } from "react-router-dom";

import { useCreateSecretMount, useSecretMounts, useTestSecretMountConnection } from "../api/hooks/useSecrets";

export function SecretMountsPage() {
  const { orgId = "" } = useParams();
  const { data, isLoading } = useSecretMounts(orgId);
  const createMount = useCreateSecretMount(orgId);
  const testConnection = useTestSecretMountConnection(orgId);

  const [name, setName] = useState("");
  const [address, setAddress] = useState("");
  const [roleId, setRoleId] = useState("");
  const [secretId, setSecretId] = useState("");
  const [formError, setFormError] = useState<string | null>(null);
  const [testResult, setTestResult] = useState<Record<string, "ok" | "fail">>({});

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setFormError(null);
    try {
      await createMount.mutateAsync({ name, backend_type: "vault", address, role_id: roleId, secret_id: secretId });
      setName("");
      setAddress("");
      setRoleId("");
      setSecretId("");
    } catch {
      setFormError("Failed to create secret mount.");
    }
  }

  async function onTest(mountId: string) {
    try {
      await testConnection.mutateAsync(mountId);
      setTestResult((prev) => ({ ...prev, [mountId]: "ok" }));
    } catch {
      setTestResult((prev) => ({ ...prev, [mountId]: "fail" }));
    }
  }

  return (
    <div>
      <div className="page-header">
        <h1>Secret mounts</h1>
      </div>

      {isLoading && <p className="muted">Loading…</p>}
      {data && (
        <table>
          <thead>
            <tr>
              <th>Name</th>
              <th>Backend</th>
              <th>Address</th>
              <th>Role ID</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {data.data.map((m) => (
              <tr key={m.id}>
                <td>{m.name}</td>
                <td className="mono">{m.backend_type}</td>
                <td className="mono">{m.address}</td>
                <td className="mono">{m.role_id}</td>
                <td>
                  <button className="secondary" onClick={() => onTest(m.id)}>
                    Test connection
                  </button>{" "}
                  {testResult[m.id] === "ok" && <span className="badge badge-success">connected</span>}
                  {testResult[m.id] === "fail" && <span className="badge badge-danger">failed</span>}
                </td>
              </tr>
            ))}
            {data.data.length === 0 && (
              <tr>
                <td colSpan={5} className="muted">
                  No secret mounts yet.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      )}

      <div className="card" style={{ marginTop: 20, maxWidth: 480 }}>
        <h3>Create secret mount</h3>
        <p className="muted">Only backend_type "vault" is implemented today.</p>
        {formError && <div className="error-banner">{formError}</div>}
        <form onSubmit={onSubmit}>
          <label>
            Name
            <input value={name} onChange={(e) => setName(e.target.value)} required />
          </label>
          <label>
            Vault address
            <input
              value={address}
              onChange={(e) => setAddress(e.target.value)}
              placeholder="http://vault:8200"
              required
            />
          </label>
          <label>
            AppRole role_id
            <input value={roleId} onChange={(e) => setRoleId(e.target.value)} required />
          </label>
          <label>
            AppRole secret_id
            <input type="password" value={secretId} onChange={(e) => setSecretId(e.target.value)} required />
          </label>
          <button type="submit" disabled={createMount.isPending}>
            Create
          </button>
        </form>
      </div>
    </div>
  );
}
