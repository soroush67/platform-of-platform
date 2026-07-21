import { useState, type FormEvent } from "react";
import { useParams } from "react-router-dom";

import {
  useArchiveMachine,
  useCheckMachineConnection,
  useCreateMachine,
  useDeleteMachine,
  useDuplicateMachine,
  useMachines,
  useTestMachineConnection,
  useUnarchiveMachine,
  useUpdateMachine,
} from "../api/hooks/useFleet";
import { useSecretMounts, useWriteSecret } from "../api/hooks/useSecrets";
import { CREDENTIAL_STORAGES, CREDENTIAL_TYPES, type CredentialStorage, type Machine } from "../api/types";

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
  const unarchiveMachine = useUnarchiveMachine(orgId);
  const deleteMachine = useDeleteMachine(orgId);
  const testConnection = useTestMachineConnection(orgId);
  const writeSecret = useWriteSecret(orgId);
  const updateMachine = useUpdateMachine(orgId);
  const duplicateMachine = useDuplicateMachine(orgId);
  const [duplicateError, setDuplicateError] = useState<string | null>(null);

  // duplicatingId - shows an editable name field (pre-filled "{name}
  // (copy)") before the clone is actually created, one row at a time -
  // operator's own explicit ask: see and confirm/edit the name first,
  // not a silent instant clone.
  const [duplicatingId, setDuplicatingId] = useState<string | null>(null);
  const [duplicateNameDraft, setDuplicateNameDraft] = useState("");

  function onStartDuplicate(m: Machine) {
    setDuplicatingId(m.id);
    setDuplicateNameDraft(`${m.name} (copy)`);
    setDuplicateError(null);
  }

  async function onConfirmDuplicate(machineId: string) {
    setDuplicateError(null);
    try {
      await duplicateMachine.mutateAsync({ machineId, name: duplicateNameDraft });
      setDuplicatingId(null);
    } catch {
      setDuplicateError("Failed to duplicate - a machine with that name may already exist.");
    }
  }

  // editingDeployPathId - inline edit, one row's Deploy base path at a
  // time (same one-row-at-a-time posture as confirmingDeleteId below).
  const [editingDeployPathId, setEditingDeployPathId] = useState<string | null>(null);
  const [deployPathDraft, setDeployPathDraft] = useState("");
  const [deployPathError, setDeployPathError] = useState<string | null>(null);

  function onStartEditDeployPath(m: Machine) {
    setEditingDeployPathId(m.id);
    setDeployPathDraft(m.deploy_base_path);
    setDeployPathError(null);
  }

  async function onSaveDeployPath(machineId: string) {
    try {
      await updateMachine.mutateAsync({ machineId, deploy_base_path: deployPathDraft });
      setEditingDeployPathId(null);
    } catch {
      setDeployPathError("Failed to update deploy base path.");
    }
  }

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
  // credentialStorage defaults to "local" - no live Vault dependency to
  // create/test a Machine unless the operator explicitly opts into Vault.
  const [credentialStorage, setCredentialStorage] = useState<CredentialStorage>("local");
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
  // before Test/Create proceed - only relevant for "vault" storage; KV v2
  // writes are upserts, so calling this from both places is harmless.
  async function writeSecretIfProvided() {
    if (credentialStorage !== "vault" || !secretValue) return;
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
        credential_storage: credentialStorage,
        credential_mount_id: credentialStorage === "vault" ? credentialMountId : undefined,
        credential_path: credentialStorage === "vault" ? credentialPath : undefined,
        credential_secret: credentialStorage === "local" ? secretValue : undefined,
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
        credential_storage: credentialStorage,
        credential_mount_id: credentialStorage === "vault" ? credentialMountId : undefined,
        credential_path: credentialStorage === "vault" ? credentialPath : undefined,
        credential_secret: credentialStorage === "local" ? secretValue : undefined,
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
      {deployPathError && <div className="error-banner">{deployPathError}</div>}
      {duplicateError && <div className="error-banner">{duplicateError}</div>}
      {isLoading && <p className="muted">Loading…</p>}
      {machines && (
        <table>
          <thead>
            <tr>
              <th>Name</th>
              <th>Host</th>
              <th>Deploy base path</th>
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
                <td className="mono">
                  {editingDeployPathId === m.id ? (
                    <span className="field-row" style={{ alignItems: "center" }}>
                      <input
                        value={deployPathDraft}
                        onChange={(e) => setDeployPathDraft(e.target.value)}
                        style={{ minWidth: 160 }}
                      />
                      <button
                        className="secondary"
                        onClick={() => onSaveDeployPath(m.id)}
                        disabled={updateMachine.isPending || !deployPathDraft}
                      >
                        Save
                      </button>
                      <button className="secondary" onClick={() => setEditingDeployPathId(null)}>
                        Cancel
                      </button>
                    </span>
                  ) : (
                    <>
                      {m.deploy_base_path}{" "}
                      {!m.archived && (
                        <button className="secondary" onClick={() => onStartEditDeployPath(m)}>
                          Edit
                        </button>
                      )}
                    </>
                  )}
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
                      {duplicatingId === m.id ? (
                        <span className="field-row" style={{ alignItems: "center" }}>
                          <input
                            value={duplicateNameDraft}
                            onChange={(e) => setDuplicateNameDraft(e.target.value)}
                            style={{ minWidth: 160 }}
                          />
                          <button
                            className="secondary"
                            onClick={() => onConfirmDuplicate(m.id)}
                            disabled={duplicateMachine.isPending || !duplicateNameDraft}
                          >
                            {duplicateMachine.isPending ? "Creating…" : "Confirm"}
                          </button>
                          <button className="secondary" onClick={() => setDuplicatingId(null)}>
                            Cancel
                          </button>
                        </span>
                      ) : (
                        <button className="secondary" onClick={() => onStartDuplicate(m)}>
                          Duplicate
                        </button>
                      )}{" "}
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
                  {m.archived && (
                    <>
                      <span className="badge badge-dim">archived</span>{" "}
                      <button
                        className="secondary"
                        onClick={() => unarchiveMachine.mutate(m.id)}
                        disabled={unarchiveMachine.isPending}
                      >
                        Unarchive
                      </button>
                    </>
                  )}
                </td>
              </tr>
            ))}
            {machines.data.length === 0 && (
              <tr>
                <td colSpan={7} className="muted">
                  No machines yet.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      )}

      <div className="card" style={{ marginTop: 20, maxWidth: 560 }}>
        <h3>Add machine</h3>
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

          <label>Credential storage</label>
          <div className="field-row" style={{ marginBottom: 10 }}>
            {CREDENTIAL_STORAGES.map((s) => (
              <label key={s} style={{ flexDirection: "row", alignItems: "center", gap: 6 }}>
                <input
                  type="radio"
                  name="credential_storage"
                  value={s}
                  checked={credentialStorage === s}
                  onChange={() => setCredentialStorage(s)}
                />
                {s === "local" ? "Local (encrypted, no Vault needed)" : "Vault"}
              </label>
            ))}
          </div>

          {credentialStorage === "vault" ? (
            <>
              <p className="muted">
                The secret value below is written directly into the selected Secret Mount's Vault at the given path -
                nothing to do out-of-band first.
              </p>
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
            </>
          ) : (
            <label>
              {credentialType === "ssh_key" ? "Private key" : "Password"} (encrypted at rest, no Vault involved)
              <input
                type="password"
                value={secretValue}
                onChange={(e) => setSecretValue(e.target.value)}
                required
              />
            </label>
          )}

          <label>
            Deploy base path
            <input value={deployBasePath} onChange={(e) => setDeployBasePath(e.target.value)} required />
          </label>
          <button
            type="button"
            className="secondary"
            onClick={onTest}
            disabled={
              testConnection.isPending ||
              writeSecret.isPending ||
              !host ||
              !sshUser ||
              (credentialStorage === "vault" ? !credentialMountId || !credentialPath : !secretValue)
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
