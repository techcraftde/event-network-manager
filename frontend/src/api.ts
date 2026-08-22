import type {ConfigPlan,ConfigStatus,DanteHealth,Snapshot,Topology} from './types';
const base=import.meta.env.VITE_API_URL ?? 'http://127.0.0.1:8787';
export function getTopology():Promise<Topology>{return json('/api/topology')}
export async function scan():Promise<void>{await json('/api/discovery',{method:'POST'})}
export function getSnapshots(switchId=''):Promise<Snapshot[]>{return json(`/api/snapshots?switchId=${encodeURIComponent(switchId)}`)}
async function json<T>(path:string,init?:RequestInit):Promise<T>{const r=await fetch(`${base}${path}`,init);const body=await r.json();if(!r.ok)throw new Error(body.error??`HTTP ${r.status}`);return body}
export function getConfigStatus(switchId:string):Promise<ConfigStatus>{return json(`/api/config/status?switchId=${encodeURIComponent(switchId)}`)}
export function trustHostKey(switchId:string,fingerprint:string):Promise<ConfigStatus>{return json('/api/config/trust-host-key',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({switchId,fingerprint})})}
export function createDantePlan(switchId:string,vlanId:number,ports:number[]):Promise<ConfigPlan>{return json('/api/config/dante-plan',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({switchId,vlanId,ports})})}
export function createVLANPlan(switchId:string,vlanId:number,ports:number[],mode:'access'|'trunk'):Promise<ConfigPlan>{return json('/api/config/vlan-plan',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({switchId,vlanId,ports,mode})})}
export function getDanteHealth(switchId:string,vlanId:number,ports:number[]):Promise<DanteHealth>{return json('/api/config/dante-health',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({switchId,vlanId,ports})})}
export function applyPlan(plan:ConfigPlan):Promise<Snapshot>{return json('/api/config/apply',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({switchId:plan.switchId,description:plan.description,commands:plan.commands})})}
export function captureSnapshot(switchId:string):Promise<Snapshot>{return json('/api/snapshots',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({switchId})})}
export function rollbackSnapshot(snapshotId:string):Promise<{ok:boolean}>{return json(`/api/config/rollback/${encodeURIComponent(snapshotId)}`,{method:'POST'})}
