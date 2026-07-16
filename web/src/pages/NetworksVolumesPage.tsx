import { useState, type FormEvent } from "react";
import { useParams } from "react-router-dom";

import {
  useCreateFleetNetwork,
  useCreateFleetVolume,
  useDeleteFleetNetwork,
  useDeleteFleetVolume,
  useFleetNetworks,
  useFleetVolumes,
} from "../api/hooks/useFleet";

export function NetworksVolumesPage() {
  const { orgId = "" } = useParams();
  const { data: networks } = useFleetNetworks(orgId);
  const { data: volumes } = useFleetVolumes(orgId);
  const createNetwork = useCreateFleetNetwork(orgId);
  const deleteNetwork = useDeleteFleetNetwork(orgId);
  const createVolume = useCreateFleetVolume(orgId);
  const deleteVolume = useDeleteFleetVolume(orgId);

  const [networkName, setNetworkName] = useState("");
  const [networkExternal, setNetworkExternal] = useState(false);
  const [volumeName, setVolumeName] = useState("");
  const [volumeHostPath, setVolumeHostPath] = useState("");

  async function onCreateNetwork(e: FormEvent) {
    e.preventDefault();
    await createNetwork.mutateAsync({ name: networkName, external: networkExternal });
    setNetworkName("");
    setNetworkExternal(false);
  }

  async function onCreateVolume(e: FormEvent) {
    e.preventDefault();
    await createVolume.mutateAsync({ name: volumeName, host_path: volumeHostPath });
    setVolumeName("");
    setVolumeHostPath("");
  }

  return (
    <div>
      <div className="page-header">
        <h1>Networks &amp; volumes</h1>
      </div>

      <div className="section-title">Networks</div>
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>External</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {networks?.data.map((n) => (
            <tr key={n.id}>
              <td>{n.name}</td>
              <td>{n.external ? "Yes" : "No"}</td>
              <td>
                <button className="danger" onClick={() => deleteNetwork.mutate(n.id)} disabled={deleteNetwork.isPending}>
                  Delete
                </button>
              </td>
            </tr>
          ))}
          {networks?.data.length === 0 && (
            <tr>
              <td colSpan={3} className="muted">
                No networks yet.
              </td>
            </tr>
          )}
        </tbody>
      </table>
      <div className="card" style={{ marginTop: 12, maxWidth: 480 }}>
        <h3>Create network</h3>
        <form onSubmit={onCreateNetwork}>
          <label>
            Name
            <input value={networkName} onChange={(e) => setNetworkName(e.target.value)} required />
          </label>
          <label style={{ flexDirection: "row", alignItems: "center", gap: 6 }}>
            <input type="checkbox" checked={networkExternal} onChange={(e) => setNetworkExternal(e.target.checked)} />
            External (already exists on target hosts)
          </label>
          <button type="submit" disabled={createNetwork.isPending}>
            Create
          </button>
        </form>
      </div>

      <div className="section-title">Volumes</div>
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Host path</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {volumes?.data.map((v) => (
            <tr key={v.id}>
              <td>{v.name}</td>
              <td className="mono">{v.host_path}</td>
              <td>
                <button className="danger" onClick={() => deleteVolume.mutate(v.id)} disabled={deleteVolume.isPending}>
                  Delete
                </button>
              </td>
            </tr>
          ))}
          {volumes?.data.length === 0 && (
            <tr>
              <td colSpan={3} className="muted">
                No volumes yet.
              </td>
            </tr>
          )}
        </tbody>
      </table>
      <div className="card" style={{ marginTop: 12, maxWidth: 480 }}>
        <h3>Create volume</h3>
        <form onSubmit={onCreateVolume}>
          <label>
            Name
            <input value={volumeName} onChange={(e) => setVolumeName(e.target.value)} required />
          </label>
          <label>
            Host path
            <input
              value={volumeHostPath}
              onChange={(e) => setVolumeHostPath(e.target.value)}
              placeholder="/data/fleet/my-volume"
              required
            />
          </label>
          <button type="submit" disabled={createVolume.isPending}>
            Create
          </button>
        </form>
      </div>
    </div>
  );
}
