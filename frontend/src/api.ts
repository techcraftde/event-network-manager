import type {AlarmReport,ConfigPlan,ConfigStatus,DanteHealth,EventBaselineStatus,RolePortRequest,RoleProfile,Snapshot,Topology} from './types';
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
export function saveStartupConfig(switchId:string):Promise<{ok:boolean}>{return json('/api/config/save-startup',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({switchId})})}
export function captureSnapshot(switchId:string):Promise<Snapshot>{return json('/api/snapshots',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({switchId})})}
export function rollbackSnapshot(snapshotId:string):Promise<{ok:boolean}>{return json(`/api/config/rollback/${encodeURIComponent(snapshotId)}`,{method:'POST'})}
export function getRoles():Promise<RoleProfile[]>{return json('/api/roles')}
export function saveRoles(roles:RoleProfile[]):Promise<RoleProfile[]>{return json('/api/roles',{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify(roles)})}
export function createRolePlan(switchId:string,ports:RolePortRequest[]):Promise<ConfigPlan>{return json('/api/config/role-plan',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({switchId,ports})})}
export function applyRolePlan(plan:ConfigPlan,settings:RolePortRequest[]):Promise<Snapshot>{return json('/api/config/apply-role',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({plan,settings})})}
export function saveSwitchName(switchId:string,name:string):Promise<void>{return json('/api/switches/name',{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({switchId,name})})}
export function getAlarms():Promise<AlarmReport>{return json('/api/alarms')}
export function getEventBaseline(switchId:string):Promise<EventBaselineStatus>{return json(`/api/config/event-baseline?switchId=${encodeURIComponent(switchId)}`)}
export function createEventBaselinePlan(switchId:string):Promise<ConfigPlan>{return json('/api/config/event-baseline-plan',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({switchId})})}
export function applyEventBaseline(plan:ConfigPlan):Promise<Snapshot>{return json('/api/config/apply-event-baseline',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({plan})})}
export function applyReferenceReset(switchId:string):Promise<Snapshot>{return json('/api/config/apply-reference-reset',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({switchId})})}
