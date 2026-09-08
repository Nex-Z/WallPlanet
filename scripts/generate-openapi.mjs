import fs from 'node:fs'
import path from 'node:path'
import {fileURLToPath} from 'node:url'
const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..')
const string={type:'string'},integer={type:'integer'},boolean={type:'boolean'},date={type:'string',format:'date-time'}
const ref=name=>({$ref:'#/components/schemas/'+name})
const array=items=>({type:'array',items})
const object=(properties,required=[])=>({type:'object',properties,...(required.length?{required}:{})})
const page=name=>object({items:array(ref(name)),nextCursor:{type:'string',description:'Opaque cursor; empty or omitted means end of list.'}},['items'])
const schemas={
 Error:object({error:string},['error']),OK:object({ok:boolean},['ok']),
 Credentials:object({username:{type:'string',minLength:3,maxLength:32},password:{type:'string',minLength:10,maxLength:128,writeOnly:true}},['username','password']),
 Profile:object({name:{type:'string',minLength:1,maxLength:40},bio:{type:'string',maxLength:200},avatar:{...string,description:'A local /assets/ or /media/ image path, or empty.'},cover:string},['name']),
 User:object({id:string,username:string,role:{type:'string',enum:['user','admin']},profile:ref('Profile'),stats:object({favorites:integer,downloads:integer,subscriptions:integer,history:integer})}),
 Session:object({user:{oneOf:[ref('User'),{type:'null'}]},csrf:string,demo:boolean}),
 Entity:object({id:string,kind:{type:'string',enum:['author','topic','channel']},name:string,description:string,avatar:string,cover:string,handle:string,featured:boolean,subscribed:boolean,subscribers:integer,count:integer,score:integer}),
 Image:object({id:string,url:string,thumbnail:string,width:integer,height:integer,bytes:integer,mime:string}),
 Wallpaper:object({id:string,title:string,description:string,tags:array(string),topicIds:array(string),sourceUrl:string,sourceMetrics:object({likes:integer,reposts:integer,replies:integer}),publishedAt:date,status:{type:'string',enum:['published','hidden']},featured:boolean,author:ref('Entity'),channel:ref('Entity'),images:array(ref('Image')),likes:integer,favorites:integer,downloads:integer,liked:boolean,favorited:boolean,score:integer}),
 WallpaperUpdate:object({title:{type:'string',minLength:1,maxLength:500},description:string,tags:array(string),topicIds:array(string),status:{type:'string',enum:['published','hidden']},featured:boolean},['title','description','tags','topicIds','status','featured']),
 Source:object({id:string,name:{type:'string',maxLength:100},kind:{type:'string',enum:['accounts','keyword']},query:{type:'string',maxLength:5000},enabled:{type:'boolean',default:false},interval_hours:{type:'integer',minimum:1,maximum:720,default:168},max_items:{type:'integer',minimum:1,maximum:1000,default:100},min_short:{type:'integer',minimum:1,default:720},min_long:{type:'integer',minimum:1,default:1280},topic_ids:array(string),tags:array(string),watermark:{oneOf:[date,{type:'null'}]},next_run:date},['name','kind','query']),
 Job:object({id:string,source_id:string,source_name:string,status:{type:'string',enum:['queued','starting','uncertain','running','importing','retrying','succeeded','partial','failed']},run_id:{type:['string','null']},dataset_id:{type:['string','null']},dataset_offset:integer,imported:integer,skipped:integer,failed_items:integer,error:string,started_at:{oneOf:[date,{type:'null'}]},finished_at:{oneOf:[date,{type:'null'}]},created_at:date,input:{type:['object','null']}}),
 JobItem:object({id:string,source_key:string,error:string,attempts:integer,resolved:boolean}),
 Banner:object({title:{type:'string',maxLength:100},subtitle:{type:'string',maxLength:200},image:{...string,description:'Local /media/ or /assets/ path'},href:{...string,description:'Same-site path, for example /discover?topic=nature'}},['title','image','href']),
 Homepage:object({banners:{type:'array',maxItems:6,items:ref('Banner')}},['banners']),
 Settings:object({tokenConfigured:boolean,tokenMask:string,updatedAt:{oneOf:[date,{type:'null'}]},actor:{type:'string',const:'xquik/x-tweet-scraper'},demo:boolean}),
 AdminUser:object({id:string,username:string,role:string,disabled:boolean,profile:ref('Profile'),created_at:date}),
}
for(const s of ['Wallpaper','Entity','Source','Job','JobItem','AdminUser'])schemas[s+'Page']=page(s)
const paths={}
const q=(name,schema=string,description='')=>({name,in:'query',required:false,schema,description})
function endpoint(url,method,summary,{tag='Catalog',body,response='OK',auth=false,queries=[],status=200,description='',binary=false}={}){
 const parameters=[...url.matchAll(/\{(\w+)\}/g)].map(m=>({name:m[1],in:'path',required:true,schema:string})).concat(queries)
 if(auth&&method!=='get')parameters.push({name:'X-CSRF-Token',in:'header',required:true,schema:string,description:'csrf returned from /auth/me or authentication response.'})
 const schema=typeof response==='string'?ref(response):response
 const op={operationId:method+'_'+url.replace(/[{}]/g,'').replaceAll('/','_'),tags:[tag],summary,description,parameters,security:auth?[{sessionCookie:[]}]:[],responses:{[status]:{description:status===202?'Queued':'Success',content:binary?{'image/jpeg':{schema:{type:'string',format:'binary'}},'image/png':{schema:{type:'string',format:'binary'}},'image/webp':{schema:{type:'string',format:'binary'}}}:{'application/json':{schema}}},'400':{description:'Invalid request',content:{'application/json':{schema:ref('Error')}}},'401':{description:'Login required',content:{'application/json':{schema:ref('Error')}}},'403':{description:'Role, Origin or CSRF rejected',content:{'application/json':{schema:ref('Error')}}},'404':{description:'Not found or hidden',content:{'application/json':{schema:ref('Error')}}},'409':{description:'Conflict with existing account/task or current state',content:{'application/json':{schema:ref('Error')}}},'429':{description:'Rate limit exceeded',content:{'application/json':{schema:ref('Error')}}},'500':{description:'Temporary server error',content:{'application/json':{schema:ref('Error')}}}}}
 if(body)op.requestBody={required:true,content:{'application/json':{schema:typeof body==='string'?ref(body):body}}}
 paths[url]??={};paths[url][method]=op
}
endpoint('/health','get','Database readiness',{tag:'System',response:object({status:string,demo:boolean})})
endpoint('/openapi.json','get','This OpenAPI document',{tag:'System',response:{type:'object'}})
endpoint('/auth/me','get','Current user and CSRF token',{tag:'Authentication',response:'Session',description:'Returns user:null for guests. Cookie-authenticated requests include current profile and live personal counts.'})
for(const action of ['register','login'])endpoint('/auth/'+action,'post',action==='register'?'Create user account':'Sign in',{tag:'Authentication',body:'Credentials',response:object({csrf:string}),description:'Sets HttpOnly session cookie. Rate limited to 15 authentication attempts per IP per 15 minutes. Username must be 3–32 characters and password 10–128 characters. If already authenticated, supply current X-CSRF-Token.'})
endpoint('/auth/logout','post','Sign out',{tag:'Authentication',auth:true})
endpoint('/me/profile','patch','Update nickname, bio, avatar and cover',{tag:'Account',auth:true,body:'Profile'})
endpoint('/me/password','put','Change password and revoke all sessions',{tag:'Account',auth:true,body:object({current:string,password:{type:'string',minLength:10,maxLength:128}},['current','password'])})
endpoint('/home','get','Homepage carousel configuration',{response:'Homepage'})
endpoint('/wallpapers','get','Search and browse wallpaper groups',{response:'WallpaperPage',queries:[q('q',string,'Search title, description, author and tags'),q('topic'),q('author'),q('channel'),q('tag'),q('orientation',{type:'string',enum:['landscape','portrait']}),q('scope',{type:'string',enum:['favorites','downloads','history','subscriptions']},'Requires login when present'),q('featured',{type:'string',enum:['true','prefer']},'prefer uses featured works if any exist, otherwise all works'),q('limit',{type:'integer',minimum:1,maximum:60,default:24}),q('cursor')],description:'Results contain only published groups with stored images. Cursor encodes timestamp and ID. Personal lists use the relevant interaction timestamp; ordinary lists use publication time.'})
endpoint('/wallpapers/{id}','get','Wallpaper group with all stored images',{response:'Wallpaper'})
for(const method of ['put','delete'])endpoint('/wallpapers/{id}/interactions/{kind}',method,method==='put'?'Like or favorite idempotently':'Remove like or favorite',{auth:true,description:'kind must be like or favorite. Existing state is not duplicated.'})
endpoint('/wallpapers/{id}/history','post','Record a view; keep latest 50 wallpapers',{auth:true})
endpoint('/wallpapers/{id}/download/{media}','post','Download selected original image',{auth:true,binary:true,description:'Attachment response. Counts a responded download request, not browser completion. Limited to 30 per user per minute.'})
endpoint('/entities','get','List authors, topics or channels',{response:'EntityPage',queries:[q('kind',{type:'string',enum:['author','topic','channel']}),q('subscribed',boolean),q('cursor')],description:'Up to 60 results per page. IDs are opaque strings. subscribed=true returns subscriptions of the current user.'})
endpoint('/entities/{id}','get','Entity details and subscription status',{response:'Entity'})
for(const method of ['put','delete'])endpoint('/entities/{id}/subscription',method,method==='put'?'Subscribe idempotently':'Unsubscribe',{auth:true})
endpoint('/ranks','get','Top 50 ranked wallpapers, authors or channels',{queries:[q('kind',{type:'string',enum:['wallpaper','author','channel'],default:'wallpaper'}),q('period',{type:'string',enum:['7d','30d','all'],default:'7d'})],response:object({items:array({oneOf:[ref('Wallpaper'),ref('Entity')]}),period:string}),description:'Hourly snapshots. Score = likes + favorites*4 + distinct(user,wallpaper,UTC date) downloads*2. X metrics are excluded. Scores aggregate for authors/channels; zero-score ties have deterministic ordering.'})
const admin=(url,method,summary,options={})=>endpoint('/admin'+url,method,summary,{...options,tag:'Administration',auth:true,description:'Administrator role required. '+(options.description||'')})
admin('/overview','get','Operational counts',{response:object({users:integer,wallpapers:integer,sources:integer,activeJobs:integer,failedItems:integer})})
admin('/settings','get','Apify configuration status (never returns token)',{response:'Settings'})
admin('/settings','put','Encrypt and replace Apify Token',{body:object({token:{type:'string',minLength:10,maxLength:512,writeOnly:true}},['token']),response:'Settings',description:'Disabled in demo mode.'})
admin('/sources','get','List collection sources',{response:'SourcePage'})
admin('/sources/preview','post','Preview batch input without calling Apify',{body:'Source',response:object({actor:string,input:{type:'object'},authorCount:{type:'integer'},queryCount:{type:'integer'},runs:{type:'integer'}})})
admin('/sources','post','Create a source (paused by default)',{body:'Source',response:'Source'})
admin('/sources/{id}','put','Update source configuration',{body:'Source',response:'Source'})
admin('/sources/{id}/run','post','Queue manual collection',{status:202,response:object({id:string}),description:'Requires configured token, disabled in demo. Rejects a duplicate active source task.'})
admin('/jobs','get','Collection runs and import status',{response:'JobPage',queries:[q('cursor')]})
admin('/jobs/{id}/items','get','Import item outcomes and failures',{response:'JobItemPage'})
admin('/jobs/{id}/retry','post','Retry failed items without creating another Actor run',{status:202})
admin('/jobs/{id}/items/{item}/retry','post','Retry only one failed dataset item',{status:202})
admin('/jobs/{id}/reconcile','post','Resolve an uncertain Actor start',{body:object({runId:string,confirmAbsent:boolean}),description:'Supply either a verified existing run ID, or confirmAbsent:true after inspecting the Apify console. Existing run input and start time are validated before association.'})
admin('/wallpapers','get','List published and hidden works',{response:'WallpaperPage',queries:[q('q'),q('cursor')]})
admin('/wallpapers/{id}','patch','Edit metadata, recommendation and publication status',{body:'WallpaperUpdate'})
const entityInput=object({kind:{type:'string',enum:['author','topic','channel']},name:{type:'string',minLength:1,maxLength:100},description:string,avatar:string,cover:string,featured:boolean},['kind','name'])
admin('/entities','post','Create a curated author or topic',{body:entityInput,response:object({id:string}),description:'Only the existing X channel is supported in v1; creating additional channels is rejected.'})
admin('/entities/{id}','put','Update entity display metadata',{body:entityInput,response:object({id:string})})
admin('/homepage','put','Replace homepage banners',{body:'Homepage'})
admin('/users','get','List user status without credentials',{response:'AdminUserPage',queries:[q('cursor')]})
admin('/users/{id}','patch','Enable or disable a regular account',{body:object({disabled:boolean},['disabled']),description:'Administrator accounts cannot be disabled by this endpoint.'})
const doc={openapi:'3.1.0',info:{title:'壁纸星球 / WallPlanet API',version:'1.0.0',description:'REST API for Web and future app clients. IDs are strings; database credentials and Apify secrets never appear in public payloads. Browser auth uses an HttpOnly session cookie; mutating authenticated requests require X-CSRF-Token. Demo mode uses a separate database and wallplanet_demo_session cookie.'},servers:[{url:'/api/v1'}],tags:[{name:'Catalog'},{name:'Authentication'},{name:'Account'},{name:'Administration'},{name:'System'}],paths,components:{securitySchemes:{sessionCookie:{type:'apiKey',in:'cookie',name:'wallplanet_session'}},schemas}}
fs.writeFileSync(path.join(root,'server/openapi.json'),JSON.stringify(doc,null,2)+'\n')
console.log(`Documented ${Object.values(paths).reduce((sum,x)=>sum+Object.keys(x).length,0)} operations`)
