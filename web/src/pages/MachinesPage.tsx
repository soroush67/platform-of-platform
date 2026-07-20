import { useState, type FormEvent } from "react";
import { useParams } from "react-router-dom";

import {
  useArchiveMachine,
  useCheckMachineConnection,
  useCreateMachine,
  useDeleteMachine,
  useMachines,
  useTestMachineConnection,
} from "../api/hooks/useFleet";
import { useSecretMounts, useWriteSecret } from "../api/hooks/useSecrets";
import { CREDENTIAL_TYPES, type Machine } from "../api/types";

function statusBadgeClass(status: string): string {
  if (status === "online" || status === "ok") return "badge-success";
  if (status === "unreachable" || status === "missing" || status === "error") return "badge-danger";
  return "badge-dim";
}

function slugify(name: string): string {
  return name
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
}

export function MachinesPage() {
  const { orgId = "" } = useParams();
  const { data: machines, isLoading } = useMachines(orgId, true);
  const { data: secretMounts } = useSecretMounts(orgId);
  const createMachine = useCreateMachine(orgId);
  const checkConnection = useCheckMachineConnection(orgId);
  const archiveMachine = useArchiveMachine(orgId);
  const deleteMachine = useDeleteMachine(orgId);
  const testConnection = useTestMachineConnection(orgId);
  const writeSecret = useWriteSecret(orgId);

  // confirmingDeleteId - two-step confirm for the destructive Delete
  // action, one row at a time (same pattern as elsewhere in this app).
  const [confirmingDeleteId, setConfirmingDeleteId] = useState<string | null>(null);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  async function onDelete(machineId: string) {
    if (confirmingDeleteId !== machineId) {
      setConfirmingDeleteId(machineId);
      setDeleteError(null);
      return;
    }
    try {
      await deleteMachine.mutateAsync(machineId);
    } catch {
      setDeleteError("Failed to delete - this machine has operation history and must be archived instead.");
    }
    setConfirmingDeleteId(null);
  }

  const [name, setName] = useState("");
  const [host, setHost] = useState("");
  const [sshPort, setSshPort] = useState(22);
  const [sshUser, setSshUser] = useState("");
  const [credentialType, setCredentialType] = useState<(typeof CREDENTIAL_TYPES)[number]>("ssh_key");
  const [credentialMountId, setCredentialMountId] = useState("");
  const [credentialPath, setCredentialPath] = useState("");
  const [secretValue, setSecretValue] = useState("");
  const [deployBasePath, setDeployBasePath] = useState("/opt/fleet");
  const [formError, setFormError] = useState<string | null>(null);
  const [testResult, setTestResult] = useState<string | null>(null);

  function onNameChange(value: string) {
    setName(value);
    // Only auto-fill while the operator hasn't typed a path of their own -
    // once they have, further name edits shouldn't silently overwrite it.
    if (!credentialPath || credentialPath.startsWith("secret/data/fleet/machines/")) {
      setCredentialPath(value ? `secret/data/fleet/machines/${slugify(value)}` : "");
    }
  }

  // writeSecretIfProvided stores secretValue into Vault at credentialPath
  // before Test/Create proceed - KV v2 writes are upserts, so calling
  // this from both places whenever a value is typed is harmless.
  async function writeSecretIfProvided() {
    if (!secretValue) return;
    await writeSecret.mutateAsync({ mountId: credentialMountId, path: credentialPath, value: secretValue });
  }

  async function onTest() {
    setTestResult(null);
    try {
      await writeSecretIfProvided();
      const result = await testConnection.mutateAsync({
        host,
        ssh_port: sshPort,
        ssh_user: sshUser,
        credential_type: credentialType,
        credential_mount_id: credentialMountId,
        credential_path: credentialPath,
      });
      setTestResult(`${result.connection_status} / docker: ${result.docker_status}`);
    } catch {
      setTestResult("test failed");
    }
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setFormError(null);
    try {
      await writeSecretIfProvided();
      await createMachine.mutateAsync({
        name,
        host,
        ssh_port: sshPort,
        ssh_user: sshUser,
        credential_type: credentialType,
        credential_mount_id: credentialMountId,
        credential_path: credentialPath,
        deploy_base_path: deployBasePath,
      });
      setName("");
      setHost("");
      setSshUser("");
      setCredentialPath("");
      setSecretValue("");
      setTestResult(null);
    } catch {
      setFormError(writeSecret.isError ? "Failed to write the secret to Vault." : "Failed to create machine.");
    }
  }

  return (
    <div>
      <div className="page-header">
        <h1>Machines</h1>
      </div>

      {deleteError && <div className="error-banner">{deleteError}</div>}
      {isLoading && <p className="muted">Loading…</p>}
      {machines && (
        <table>
          <thead>
            <tr>
              <th>Name</th>
              <th>Host</th>
              <th>Connection</th>
              <th>Docker</th>
              <th>Last checked</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {machines.data.map((m: Machine) => (
              <tr key={m.id} style={m.archived ? { opacity: 0.5 } : undefined}>
                <td>{m.name}</td>
                <td className="mono">
                  {m.ssh_user}@{m.host}:{m.ssh_port}
                </td>
                <td>
                  <span className={`badge ${statusBadgeClass(m.connection_status)}`}>{m.connection_status}</span>
                </td>
                <td>
                  <span className={`badge ${statusBadgeClass(m.docker_status)}`}>{m.docker_status}</span>
                </td>
                <td className="muted">{m.last_checked_at ? new Date(m.last_checked_at).toLocaleString() : "never"}</td>
                <td>
                  {!m.archived && (
                    <>
                      <button
                        className="secondary"
                        onClick={() => checkConnection.mutate(m.id)}
                        disabled={checkConnection.isPending}
                      >
                        Check connection
                      </button>{" "}
                      <button
                        className="secondary"
                        onClick={() => archiveMachine.mutate(m.id)}
                        disabled={archiveMachine.isPending}
                      >
                        Archive
                      </button>{" "}
                      {confirmingDeleteId === m.id ? (
                        <>
                          <button className="danger" onClick={() => onDelete(m.id)} disabled={deleteMachine.isPending}>
                            {deleteMachine.isPending ? "Deleting…" : "Confirm delete"}
                          </button>{" "}
                          <button className="secondary" onClick={() => setConfirmingDeleteId(null)}>
                            Cancel
                          </button>
                        </>
                      ) : (
                        <button className="danger" onClick={() => onDelete(m.id)}>
                          Delete
                        </button>
                      )}
                    </>
                  )}
                  {m.archived && <span className="badge badge-dim">archived</span>}
                </td>
              </tr>
            ))}
            {machines.data.length === 0 && (
              <tr>
                <td colSpan={6} className="muted">
                  No machines yet.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      )}

      <div className="card" style={{ marginTop: 20, maxWidth: 560 }}>
        <h3>Add machine</h3>
        <p className="muted">
          The secret value below is written directly into the selected Secret Mount's Vault at the given path -
          nothing to do out-of-band first.
        </p>
        {formError && <div className="error-banner">{formError}</div>}
        <form onSubmit={onSubmit}>
          <label>
            Name
            <input value={name} onChange={(e) => onNameChange(e.target.value)} required />
          </label>
          <div className="field-row">
            <label>
              Host
              <input value={host} onChange={(e) => setHost(e.target.value)} placeholder="10.0.0.5" required />
            </label>
            <label>
              SSH port
              <input type="number" value={sshPort} onChange={(e) => setSshPort(Number(e.target.value))} />
            </label>
          </div>
          <label>
            SSH user
            <input value={sshUser} onChange={(e) => setSshUser(e.target.value)} required />
          </label>
          <label>
            Credential type
            <select value={credentialType} onChange={(e) => setCredentialType(e.target.value as (typeof CREDENTIAL_TYPES)[number])}>
              {CREDENTIAL_TYPES.map((t) => (
                <option key={t} value={t}>
                  {t}
                </option>
              ))}
            </select>
          </label>
          <label>
            Secret mount
            <select value={credentialMountId} onChange={(e) => setCredentialMountId(e.target.value)} required>
              <option value="">— select —</option>
              {secretMounts?.data.map((sm) => (
                <option key={sm.id} value={sm.id}>
                  {sm.name}
                </option>
              ))}
            </select>
          </label>
          <label>
            Secret path
            <input
              value={credentialPath}
              onChange={(e) => setCredentialPath(e.target.value)}
              placeholder="secret/data/fleet/machines/this-host"
              required
            />
          </label>
          <label>
            Secret value ({credentialType === "ssh_key" ? "private key" : "password"})
            <input
              type="password"
              value={secretValue}
              onChange={(e) => setSecretValue(e.target.value)}
              placeholder="leave blank if already written to Vault"
            />
          </label>
          <label>
            Deploy base path
            <input value={deployBasePath} onChange={(e) => setDeployBasePath(e.target.value)} required />
          </label>
          <button
            type="button"
            className="secondary"
            onClick={onTest}
            disabled={
              testConnection.isPending || writeSecret.isPending || !host || !sshUser || !credentialMountId || !credentialPath
            }
          >
            Test connection
          </button>{" "}
          {testResult && <span className="mono">{testResult}</span>}
          <div>
            <button type="submit" disabled={createMachine.isPending || writeSecret.isPending}>
              Create
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
