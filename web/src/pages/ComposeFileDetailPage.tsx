import { useEffect, useState, type FormEvent } from "react";
import { Link, useParams } from "react-router-dom";

import {
  useAttachNetwork,
  useAttachVolume,
  useComposeFile,
  useComposeFileNetworks,
  useComposeFileVolumes,
  useCreateFleetVariable,
  useDeleteFleetVariable,
  useDetachNetwork,
  useDetachVolume,
  useFleetNetworks,
  useFleetVariables,
  useFleetVolumes,
  useMachines,
  useOperations,
  useTriggerOperation,
  useUpdateComposeFileContent,
} from "../api/hooks/useFleet";
import { OPERATION_TYPES, VAR_TYPES, type VarType } from "../api/types";

function operationStatusBadgeClass(status: string): string {
  if (status === "success") return "badge-success";
  if (status === "failed") return "badge-danger";
  if (status === "running") return "badge-warning";
  return "badge-dim";
}

export function ComposeFileDetailPage() {
  const { orgId = "", composeFileId = "" } = useParams();
  const { data: composeFile } = useComposeFile(orgId, composeFileId);
  const updateContent = useUpdateComposeFileContent(orgId);

  const { data: networks } = useComposeFileNetworks(orgId, composeFileId);
  const { data: allNetworks } = useFleetNetworks(orgId);
  const attachNetwork = useAttachNetwork(orgId, composeFileId);
  const detachNetwork = useDetachNetwork(orgId, composeFileId);

  const { data: volumes } = useComposeFileVolumes(orgId, composeFileId);
  const { data: allVolumes } = useFleetVolumes(orgId);
  const attachVolume = useAttachVolume(orgId, composeFileId);
  const detachVolume = useDetachVolume(orgId, composeFileId);

  const { data: variables } = useFleetVariables(orgId, composeFileId);
  const createVariable = useCreateFleetVariable(orgId, composeFileId);
  const deleteVariable = useDeleteFleetVariable(orgId, composeFileId);

  const { data: machines } = useMachines(orgId);
  const { data: operations } = useOperations(orgId, composeFileId);
  const triggerOperation = useTriggerOperation(orgId);

  const [content, setContent] = useState("");
  const [contentSaved, setContentSaved] = useState(false);
  useEffect(() => {
    if (composeFile) setContent(composeFile.compose_content);
  }, [composeFile]);

  const [selectedNetworkId, setSelectedNetworkId] = useState("");
  const [selectedVolumeId, setSelectedVolumeId] = useState("");
  const [containerPath, setContainerPath] = useState("");

  const [varKey, setVarKey] = useState("");
  const [varType, setVarType] = useState<VarType>("kv");
  const [varValue, setVarValue] = useState("");
  const [varSecretMountId, setVarSecretMountId] = useState("");
  const [varSecretPath, setVarSecretPath] = useState("");
  const [varFileTargetPath, setVarFileTargetPath] = useState("");
  const [varError, setVarError] = useState<string | null>(null);

  const [deployMachineId, setDeployMachineId] = useState("");
  const [operationType, setOperationType] = useState<(typeof OPERATION_TYPES)[number]>("deploy");
  const [triggerError, setTriggerError] = useState<string | null>(null);

  async function onSaveContent(e: FormEvent) {
    e.preventDefault();
    setContentSaved(false);
    await updateContent.mutateAsync({ composeFileId, composeContent: content });
    setContentSaved(true);
  }

  async function onCreateVariable(e: FormEvent) {
    e.preventDefault();
    setVarError(null);
    try {
      await createVariable.mutateAsync({
        key: varKey,
        var_type: varType,
        value: varType === "secret" ? undefined : varValue,
        secret_mount_id: varType === "secret" ? varSecretMountId : undefined,
        secret_path: varType === "secret" ? varSecretPath : undefined,
        file_target_path: varType === "file_template" || varType === "config_file" ? varFileTargetPath : undefined,
      });
      setVarKey("");
      setVarValue("");
      setVarSecretMountId("");
      setVarSecretPath("");
      setVarFileTargetPath("");
    } catch {
      setVarError("Failed to create variable.");
    }
  }

  async function onTrigger(e: FormEvent) {
    e.preventDefault();
    setTriggerError(null);
    try {
      await triggerOperation.mutateAsync({
        compose_file_id: composeFileId,
        machine_id: deployMachineId,
        operation_type: operationType,
      });
    } catch {
      setTriggerError("Failed to trigger operation - the machine may be archived.");
    }
  }

  const attachedNetworkIds = new Set(networks?.data.map((n) => n.id));
  const attachedVolumeIds = new Set(volumes?.data.map((va) => va.volume.id));

  return (
    <div>
      <div className="page-header">
        <h1>{composeFile?.name ?? "Compose file"}</h1>
        {composeFile?.is_global && <span className="badge">global</span>}
      </div>

      <div className="card">
        <h3>docker-compose.yml</h3>
        <form onSubmit={onSaveContent}>
          <textarea
            className="mono"
            value={content}
            onChange={(e) => {
              setContent(e.target.value);
              setContentSaved(false);
            }}
            rows={14}
          />
          <button type="submit" disabled={updateContent.isPending}>
            Save
          </button>{" "}
          {contentSaved && <span className="badge badge-success">saved</span>}
        </form>
      </div>

      <div className="section-title">Networks</div>
      <div className="card">
        {networks?.data.map((n) => (
          <div key={n.id} className="field-row" style={{ alignItems: "center", marginBottom: 6 }}>
            <span className="mono">{n.name}</span>
            <button className="danger" onClick={() => detachNetwork.mutate(n.id)} disabled={detachNetwork.isPending}>
              Detach
            </button>
          </div>
        ))}
        {networks?.data.length === 0 && <p className="muted">No networks attached.</p>}
        <div className="field-row" style={{ marginTop: 10 }}>
          <select value={selectedNetworkId} onChange={(e) => setSelectedNetworkId(e.target.value)}>
            <option value="">— select network —</option>
            {allNetworks?.data
              .filter((n) => !attachedNetworkIds.has(n.id))
              .map((n) => (
                <option key={n.id} value={n.id}>
                  {n.name}
                </option>
              ))}
          </select>
          <button
            className="secondary"
            disabled={!selectedNetworkId}
            onClick={() => {
              attachNetwork.mutate(selectedNetworkId);
              setSelectedNetworkId("");
            }}
          >
            Attach
          </button>
        </div>
      </div>

      <div className="section-title">Volumes</div>
      <div className="card">
        {volumes?.data.map((va) => (
          <div key={va.volume.id} className="field-row" style={{ alignItems: "center", marginBottom: 6 }}>
            <span className="mono">
              {va.volume.name} → {va.container_path}
            </span>
            <button className="danger" onClick={() => detachVolume.mutate(va.volume.id)} disabled={detachVolume.isPending}>
              Detach
            </button>
          </div>
        ))}
        {volumes?.data.length === 0 && <p className="muted">No volumes attached.</p>}
        <div className="field-row" style={{ marginTop: 10 }}>
          <select value={selectedVolumeId} onChange={(e) => setSelectedVolumeId(e.target.value)}>
            <option value="">— select volume —</option>
            {allVolumes?.data
              .filter((v) => !attachedVolumeIds.has(v.id))
              .map((v) => (
                <option key={v.id} value={v.id}>
                  {v.name}
                </option>
              ))}
          </select>
          <input
            value={containerPath}
            onChange={(e) => setContainerPath(e.target.value)}
            placeholder="/data"
            style={{ flex: 1 }}
          />
          <button
            className="secondary"
            disabled={!selectedVolumeId || !containerPath}
            onClick={() => {
              attachVolume.mutate({ volumeId: selectedVolumeId, containerPath });
              setSelectedVolumeId("");
              setContainerPath("");
            }}
          >
            Attach
          </button>
        </div>
      </div>

      <div className="section-title">Variables</div>
      <table>
        <thead>
          <tr>
            <th>Key</th>
            <th>Type</th>
            <th>Value</th>
            <th>File target path</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {variables?.data.map((v) => (
            <tr key={v.id}>
              <td className="mono">{v.key}</td>
              <td>{v.var_type}</td>
              <td className="mono">
                {v.secret_ref ? `vault: ${v.secret_ref.mount_id}/${v.secret_ref.path}` : v.value}
              </td>
              <td className="mono">{v.file_target_path || "—"}</td>
              <td>
                <button className="danger" onClick={() => deleteVariable.mutate(v.id)}>
                  Delete
                </button>
              </td>
            </tr>
          ))}
          {variables?.data.length === 0 && (
            <tr>
              <td colSpan={5} className="muted">
                No variables yet.
              </td>
            </tr>
          )}
        </tbody>
      </table>
      <div className="card" style={{ marginTop: 12, maxWidth: 520 }}>
        <h3>Add variable</h3>
        {varError && <div className="error-banner">{varError}</div>}
        <form onSubmit={onCreateVariable}>
          <div className="field-row">
            <label>
              Key
              <input value={varKey} onChange={(e) => setVarKey(e.target.value)} required />
            </label>
            <label>
              Type
              <select value={varType} onChange={(e) => setVarType(e.target.value as VarType)}>
                {VAR_TYPES.map((t) => (
                  <option key={t} value={t}>
                    {t}
                  </option>
                ))}
              </select>
            </label>
          </div>
          {varType === "secret" ? (
            <div className="field-row">
              <label>
                Secret mount ID
                <input value={varSecretMountId} onChange={(e) => setVarSecretMountId(e.target.value)} required />
              </label>
              <label>
                Path
                <input value={varSecretPath} onChange={(e) => setVarSecretPath(e.target.value)} required />
              </label>
            </div>
          ) : (
            <label>
              Value
              <input value={varValue} onChange={(e) => setVarValue(e.target.value)} required />
            </label>
          )}
          {(varType === "file_template" || varType === "config_file") && (
            <label>
              File target path (relative to the deploy directory)
              <input value={varFileTargetPath} onChange={(e) => setVarFileTargetPath(e.target.value)} required />
            </label>
          )}
          <button type="submit" disabled={createVariable.isPending}>
            Add
          </button>
        </form>
      </div>

      <div className="section-title">Deploy</div>
      <div className="card" style={{ maxWidth: 480 }}>
        {triggerError && <div className="error-banner">{triggerError}</div>}
        <form onSubmit={onTrigger}>
          <label>
            Machine
            <select value={deployMachineId} onChange={(e) => setDeployMachineId(e.target.value)} required>
              <option value="">— select —</option>
              {machines?.data
                .filter((m) => !m.archived)
                .map((m) => (
                  <option key={m.id} value={m.id}>
                    {m.name} ({m.host})
                  </option>
                ))}
            </select>
          </label>
          <label>
            Operation
            <select value={operationType} onChange={(e) => setOperationType(e.target.value as (typeof OPERATION_TYPES)[number])}>
              {OPERATION_TYPES.map((t) => (
                <option key={t} value={t}>
                  {t}
                </option>
              ))}
            </select>
          </label>
          <button type="submit" disabled={triggerOperation.isPending || !deployMachineId}>
            {triggerOperation.isPending ? "Triggering…" : "Trigger"}
          </button>
        </form>
      </div>

      <div className="section-title">Operation history</div>
      <table>
        <thead>
          <tr>
            <th>Status</th>
            <th>Operation</th>
            <th>Machine</th>
            <th>Created</th>
          </tr>
        </thead>
        <tbody>
          {operations?.data.map((op) => (
            <tr key={op.id}>
              <td>
                <Link to={`/orgs/${orgId}/operations/${op.id}`}>
                  <span className={`badge ${operationStatusBadgeClass(op.status)}`}>{op.status}</span>
                </Link>
              </td>
              <td className="mono">{op.operation_type}</td>
              <td className="mono">{machines?.data.find((m) => m.id === op.machine_id)?.name ?? op.machine_id}</td>
              <td className="muted">{new Date(op.created_at).toLocaleString()}</td>
            </tr>
          ))}
          {operations?.data.length === 0 && (
            <tr>
              <td colSpan={4} className="muted">
                No operations yet.
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}
