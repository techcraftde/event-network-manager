import type {Port,Switch,Topology} from './types';

function ports(seed:number):Port[]{
 const names=['Dante','Control','Lighting','Video','Unused'];
 return Array.from({length:28},(_,i)=>{const index=i+1,link=index<=10||index>=25,role=names[(index+seed)%names.length],vlan=10+(index%4)*10;return{index,name:`gi${index}`,link,speedMbps:1000,role,vlans:[vlan],pvid:vlan,taggedVlans:[],untaggedVlans:[vlan],vlanMode:'Access',rxMbps:((index*seed*7)%90)/10,txMbps:((index*seed*11)%70)/10,errors:0,poeWatts:role==='Dante'&&link?6.4:0,poeEnabled:index<=24}})
}
const switches:Switch[]=[
 {id:'foh',name:'FOH Core',model:'SG350-28P',address:'192.168.50.2',status:'online',firmwareVersion:'2.5.9.55',hardwareVersion:'V04',serialNumber:'demo-1',macAddress:'00:00:00:00:00:01',uptimeSeconds:3600,cpuPercent:18,temperatureC:42,poeBudgetWatts:185,poeUsageWatts:12.8,ports:ports(1)},
 {id:'stage',name:'Stage Left',model:'SG350-28',address:'192.168.50.3',status:'online',firmwareVersion:'2.5.9.55',hardwareVersion:'V04',serialNumber:'demo-2',macAddress:'00:00:00:00:00:02',uptimeSeconds:3600,cpuPercent:11,temperatureC:39,poeBudgetWatts:0,poeUsageWatts:0,ports:ports(2)},
 {id:'video',name:'Video Rack',model:'SG350-28P',address:'192.168.50.4',status:'warning',firmwareVersion:'2.5.9.55',hardwareVersion:'V04',serialNumber:'demo-3',macAddress:'00:00:00:00:00:03',uptimeSeconds:3600,cpuPercent:27,temperatureC:47,poeBudgetWatts:185,poeUsageWatts:8.2,ports:ports(3)}
];
export const mockTopology:Topology={switches,links:[
 {id:'foh-stage',sourceSwitchId:'foh',sourcePort:25,targetSwitchId:'stage',targetPort:25,protocol:'LLDP'},
 {id:'foh-video',sourceSwitchId:'foh',sourcePort:26,targetSwitchId:'video',targetPort:25,protocol:'CDP'}
],updatedAt:new Date().toISOString(),source:'Demo · Backend offline',vlans:[{id:10,name:'Dante'},{id:20,name:'Control'},{id:30,name:'Lighting'},{id:40,name:'Video'}]};
